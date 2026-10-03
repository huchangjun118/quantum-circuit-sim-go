package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/bits"
	"os"
	"strings"

	"github.com/huchangjun118/quantum-circuit-sim-go/internal/circuit"
	"github.com/huchangjun118/quantum-circuit-sim-go/internal/diagram"
	"github.com/huchangjun118/quantum-circuit-sim-go/internal/simulator"
)

type options struct {
	inputPath  string
	shots      int
	seed       uint64
	diagram    bool
	state      bool
	jsonOutput bool
	stagesPath string
}

// Run executes the command-line interface.
func Run(args []string) error {
	opts, err := parseFlags(args)
	if err != nil {
		return err
	}
	input, closeInput, err := openInput(opts.inputPath)
	if err != nil {
		return err
	}
	defer closeInput()

	c, err := circuit.Parse(input)
	if err != nil {
		return err
	}

	if opts.shots > 0 {
		samples, err := simulator.SampleShots(c, opts.shots, opts.seed)
		if err != nil {
			return err
		}
		if opts.stagesPath != "" {
			stageResult, err := simulator.Run(c, simulator.RunOptions{Seed: opts.seed, CaptureStages: true})
			if err != nil {
				return err
			}
			if err := writeStageFile(opts.stagesPath, c, stageResult); err != nil {
				return err
			}
		}
		if opts.jsonOutput {
			return writeJSON(os.Stdout, sampleOutput(c, samples))
		}
		if opts.diagram {
			fmt.Print(diagram.Render(c))
		}
		printSamples(samples)
		if opts.stagesPath != "" {
			fmt.Fprintf(os.Stderr, "wrote stage states to %s\n", opts.stagesPath)
		}
		return nil
	}

	result, err := simulator.Run(c, simulator.RunOptions{Seed: opts.seed, CaptureStages: opts.stagesPath != ""})
	if err != nil {
		return err
	}
	if opts.stagesPath != "" {
		if err := writeStageFile(opts.stagesPath, c, result); err != nil {
			return err
		}
	}
	if opts.jsonOutput {
		return writeJSON(os.Stdout, resultOutput(c, result))
	}
	if opts.diagram {
		fmt.Print(diagram.Render(c))
	}
	if opts.state {
		printState(result.State)
	}
	if c.ClassicalBits > 0 {
		printClassical(result.ClassicalBits)
	}
	if opts.stagesPath != "" {
		fmt.Fprintf(os.Stderr, "wrote stage states to %s\n", opts.stagesPath)
	}
	return nil
}

func parseFlags(args []string) (options, error) {
	var opts options
	fs := flag.NewFlagSet("qcsim", flag.ContinueOnError)
	fs.StringVar(&opts.inputPath, "file", "", "circuit description file (default: first positional argument or stdin)")
	fs.IntVar(&opts.shots, "shots", 0, "number of measurement samples")
	fs.Uint64Var(&opts.seed, "seed", 0, "random seed for reproducible measurement sampling")
	fs.BoolVar(&opts.diagram, "diagram", true, "print an ASCII circuit diagram")
	fs.BoolVar(&opts.state, "state", true, "print the final state vector")
	fs.BoolVar(&opts.jsonOutput, "json", false, "print machine-readable JSON")
	fs.StringVar(&opts.stagesPath, "stages", "", "write state vectors after each instruction to a JSON file")

	flagArgs, positionalArgs := splitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return options{}, err
	}
	if opts.inputPath == "" && len(positionalArgs) > 0 {
		opts.inputPath = positionalArgs[0]
	}
	if len(positionalArgs) > 1 {
		return options{}, fmt.Errorf("only one input file may be specified")
	}
	if opts.shots < 0 {
		return options{}, fmt.Errorf("shots must be non-negative")
	}
	return opts, nil
}

// splitArgs allows flags to appear after the positional input file. The
// standard flag package stops parsing at the first non-flag argument.
func splitArgs(args []string) ([]string, []string) {
	var flagArgs, positionalArgs []string
	valueFlags := map[string]bool{
		"-file": true, "--file": true,
		"-shots": true, "--shots": true,
		"-seed": true, "--seed": true,
		"-stages": true, "--stages": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionalArgs = append(positionalArgs, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			flagArgs = append(flagArgs, arg)
			flagName := arg
			if equals := strings.IndexByte(arg, '='); equals >= 0 {
				flagName = arg[:equals]
			} else if valueFlags[flagName] && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
		} else {
			positionalArgs = append(positionalArgs, arg)
		}
	}
	return flagArgs, positionalArgs
}

func openInput(path string) (io.Reader, func() error, error) {
	if path == "" || path == "-" {
		return os.Stdin, func() error { return nil }, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}

func printState(state []complex128) {
	qubits := bits.Len(uint(len(state))) - 1
	fmt.Println("final state:")
	for i, amplitude := range state {
		probability := real(amplitude)*real(amplitude) + imag(amplitude)*imag(amplitude)
		if probability < 1e-12 {
			continue
		}
		fmt.Printf("  %s%+.6fi |%s>  p=%.6f\n", formatReal(real(amplitude)), imag(amplitude), basisString(i, qubits), probability)
	}
}

func formatReal(v float64) string {
	if math.Abs(v) < 1e-12 {
		return "0.000000"
	}
	return fmt.Sprintf("%.6f", v)
}

func basisString(index, qubits int) string {
	bits := make([]byte, qubits)
	for i := 0; i < qubits; i++ {
		if index&(1<<i) != 0 {
			bits[i] = '1'
		} else {
			bits[i] = '0'
		}
	}
	return string(bits)
}

func printClassical(bits []int) {
	parts := make([]string, len(bits))
	for i, bit := range bits {
		parts[i] = fmt.Sprintf("c%d=%d", i, bit)
	}
	fmt.Println("classical bits:", strings.Join(parts, " "))
}

func printSamples(samples *simulator.SampleSet) {
	fmt.Printf("measurement samples (shots=%d seed=%d):\n", samples.Shots, samples.Seed)
	for _, shot := range samples.Results {
		fmt.Printf("  shot %3d: %s\n", shot.Index, shot.BitString)
	}
	fmt.Println("counts:")
	for bitString, count := range samples.Counts {
		fmt.Printf("  %s: %d\n", bitString, count)
	}
}

type amplitudeJSON struct {
	Index       int     `json:"index"`
	Basis       string  `json:"basis"`
	Real        float64 `json:"real"`
	Imag        float64 `json:"imag"`
	Probability float64 `json:"probability"`
}

type stageJSON struct {
	Index            int             `json:"index"`
	Kind             string          `json:"kind"`
	Gate             string          `json:"gate,omitempty"`
	Controls         []int           `json:"controls"`
	Targets          []int           `json:"targets"`
	ClassicalTargets []int           `json:"classical_targets"`
	Outcomes         []int           `json:"outcomes"`
	State            []amplitudeJSON `json:"state"`
}

type resultJSON struct {
	Qubits        int             `json:"qubits"`
	ClassicalBits []int           `json:"classical_bits"`
	State         []amplitudeJSON `json:"state"`
	Stages        []stageJSON     `json:"stages,omitempty"`
}

type sampleJSON struct {
	Shot      int    `json:"shot"`
	BitString string `json:"bit_string"`
	Outcomes  []int  `json:"outcomes"`
}

type samplesJSON struct {
	Shots   int            `json:"shots"`
	Seed    uint64         `json:"seed"`
	Counts  map[string]int `json:"counts"`
	Results []sampleJSON   `json:"results"`
}

func resultOutput(c *circuit.Circuit, result *simulator.Result) resultJSON {
	out := resultJSON{Qubits: c.Qubits, ClassicalBits: append([]int(nil), result.ClassicalBits...)}
	out.State = amplitudes(result.State)
	for _, stage := range result.Stages {
		kind := "gate"
		if stage.Kind == circuit.InstructionMeasure {
			kind = "measurement"
		}
		out.Stages = append(out.Stages, stageJSON{
			Index:            stage.Index,
			Kind:             kind,
			Gate:             stage.GateName,
			Controls:         append([]int(nil), stage.Controls...),
			Targets:          append([]int(nil), stage.Targets...),
			ClassicalTargets: append([]int(nil), stage.ClassicalTargets...),
			Outcomes:         append([]int(nil), stage.Outcomes...),
			State:            amplitudes(stage.State),
		})
	}
	return out
}

func sampleOutput(c *circuit.Circuit, samples *simulator.SampleSet) samplesJSON {
	out := samplesJSON{Shots: samples.Shots, Seed: samples.Seed, Counts: samples.Counts, Results: make([]sampleJSON, 0, len(samples.Results))}
	for _, shot := range samples.Results {
		out.Results = append(out.Results, sampleJSON{Shot: shot.Index, BitString: shot.BitString, Outcomes: append([]int(nil), shot.Outcomes...)})
	}
	return out
}

func amplitudes(state []complex128) []amplitudeJSON {
	qubits := bits.Len(uint(len(state))) - 1
	out := make([]amplitudeJSON, 0, len(state))
	for i, value := range state {
		probability := real(value)*real(value) + imag(value)*imag(value)
		out = append(out, amplitudeJSON{
			Index:       i,
			Basis:       basisString(i, qubits),
			Real:        real(value),
			Imag:        imag(value),
			Probability: probability,
		})
	}
	return out
}

func writeStageFile(path string, c *circuit.Circuit, result *simulator.Result) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return writeJSON(f, resultOutput(c, result))
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
