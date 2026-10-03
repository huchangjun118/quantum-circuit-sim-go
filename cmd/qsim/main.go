package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	qs "github.com/huchangjun118/quantum-circuit-sim-go"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		var pe *qs.ParseError
		var tor *qs.TargetOutOfRangeError
		var mpe *qs.MissingParamError
		var ce *qs.ConflictError
		var dte *qs.DuplicateTargetError
		switch {
		case errors.As(err, &pe):
			fmt.Fprintln(os.Stderr, "rejected: unrecognized instruction (parse error)")
		case errors.As(err, &tor):
			fmt.Fprintln(os.Stderr, "rejected: target qubit out of range")
		case errors.As(err, &mpe):
			fmt.Fprintln(os.Stderr, "rejected: missing or invalid parameter")
		case errors.As(err, &ce):
			fmt.Fprintln(os.Stderr, "rejected: conflicting declaration")
		case errors.As(err, &dte):
			fmt.Fprintln(os.Stderr, "rejected: duplicate target in one instruction")
		}
		os.Exit(2)
	}
}

type options struct {
	seed    *int64
	shots   int
	asJSON  bool
	noSteps bool
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return fmt.Errorf("no subcommand given")
	}
	cmd := args[0]
	rest := args[1:]
	switch cmd {
	case "run", "draw", "snapshot", "sample":
		return doCommand(cmd, rest)
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown subcommand %q", cmd)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `qsim - local quantum circuit simulator

usage:
  qsim run      <file> [--seed N] [--shots K]   evolve; with --shots print sample counts
  qsim draw     <file>                          print ASCII circuit diagram
  qsim snapshot <file> [--seed N] [--json]      export statevector after every gate
  qsim sample   <file> --shots K [--seed N]     repeated sampling with counts

<file> may be "-" to read the circuit from stdin.
Angles in parametrized gates are in degrees.`)
}

func parseFlags(args []string) (positional []string, opt options, err error) {
	opt.shots = 1
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--seed":
			if i+1 >= len(args) {
				return nil, opt, fmt.Errorf("--seed requires a value")
			}
			v, perr := strconv.ParseInt(args[i+1], 10, 64)
			if perr != nil {
				return nil, opt, fmt.Errorf("--seed value %q is not an integer", args[i+1])
			}
			opt.seed = &v
			i++
		case a == "--shots":
			if i+1 >= len(args) {
				return nil, opt, fmt.Errorf("--shots requires a value")
			}
			v, perr := strconv.Atoi(args[i+1])
			if perr != nil || v < 1 {
				return nil, opt, fmt.Errorf("--shots value %q must be a positive integer", args[i+1])
			}
			opt.shots = v
			i++
		case a == "--json":
			opt.asJSON = true
		case strings.HasPrefix(a, "--"):
			return nil, opt, fmt.Errorf("unknown flag %q", a)
		default:
			positional = append(positional, a)
		}
	}
	return positional, opt, nil
}

func readCircuit(path string) (*qs.Circuit, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	// Parse 内部先构造临时结构，全部校验通过才返回，拒绝时不会留下半成品。
	return qs.Parse(string(data))
}

func doCommand(cmd string, args []string) error {
	pos, opt, err := parseFlags(args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("%s expects exactly one circuit file", cmd)
	}
	c, err := readCircuit(pos[0])
	if err != nil {
		return err
	}

	switch cmd {
	case "draw":
		return qs.DrawASCII(c, os.Stdout, qs.DefaultDrawConfig())

	case "snapshot":
		snaps, serr := qs.Snapshot(c, opt.seed)
		if serr != nil {
			return serr
		}
		if opt.asJSON {
			return qs.ExportJSON(c, snaps, os.Stdout)
		}
		return qs.ExportText(c, snaps, os.Stdout)

	case "run":
		var sim *qs.Simulator
		if opt.seed != nil {
			sim = qs.NewSeededSimulator(c.NumQubits(), *opt.seed)
		} else {
			sim = qs.NewSimulator(c.NumQubits())
		}
		if opt.shots > 1 {
			counts, serr := sim.Sample(c, opt.shots)
			if serr != nil {
				return serr
			}
			printCounts(counts, opt.shots)
			return nil
		}
		if err := sim.Run(c); err != nil {
			return err
		}
		printState(sim, c.NumQubits())
		for _, rec := range sim.Measurements() {
			fmt.Printf("measure q%d (line %d) -> %d\n", rec.Qubit, rec.Line, rec.Outcome)
		}
		return nil

	case "sample":
		if opt.shots <= 1 {
			return fmt.Errorf("sample requires --shots >= 2")
		}
		var sim *qs.Simulator
		if opt.seed != nil {
			sim = qs.NewSeededSimulator(c.NumQubits(), *opt.seed)
		} else {
			sim = qs.NewSimulator(c.NumQubits())
		}
		counts, serr := sim.Sample(c, opt.shots)
		if serr != nil {
			return serr
		}
		printCounts(counts, opt.shots)
		return nil
	}
	return nil
}

func printState(sim *qs.Simulator, n int) {
	st := sim.StateVector()
	probs := sim.Probs()
	fmt.Println("final state vector:")
	for i, a := range st {
		basis := make([]byte, n)
		for q := 0; q < n; q++ {
			if i&(1<<q) != 0 {
				basis[q] = '1'
			} else {
				basis[q] = '0'
			}
		}
		fmt.Printf("  |%s>: % .6f%+.6fi  p=%.6f\n", basis, real(a), imag(a), probs[i])
	}
}

func printCounts(counts map[string]int, shots int) {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("sampling results (%d shots):\n", shots)
	for _, k := range keys {
		fmt.Printf("  %s: %d (%.4f)\n", k, counts[k], float64(counts[k])/float64(shots))
	}
}
