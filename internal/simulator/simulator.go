package simulator

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/huchangjun118/quantum-circuit-sim-go/internal/circuit"
)

// Simulator owns the state vector for one circuit execution.
type Simulator struct {
	qubits int
	state  []complex128
}

// New creates a simulator in the all-zero computational-basis state.
func New(qubits int) (*Simulator, error) {
	if qubits <= 0 {
		return nil, fmt.Errorf("simulator: qubit count must be positive")
	}
	if qubits > 30 {
		return nil, fmt.Errorf("simulator: %d qubits requires more than the supported state-vector limit", qubits)
	}
	size := 1 << qubits
	state := make([]complex128, size)
	state[0] = 1
	return &Simulator{qubits: qubits, state: state}, nil
}

// Qubits returns the number of qubits represented by the state vector.
func (s *Simulator) Qubits() int { return s.qubits }

// State returns a copy of the current state vector.
func (s *Simulator) State() []complex128 {
	out := make([]complex128, len(s.state))
	copy(out, s.state)
	return out
}

// Apply evolves the simulator by one validated gate instruction.
func (s *Simulator) Apply(ins circuit.Instruction) error {
	if ins.Kind != circuit.InstructionGate {
		return fmt.Errorf("simulator: instruction is not a gate")
	}
	return s.applyGate(ins)
}

// Measure performs a projective measurement of one qubit and collapses the
// state. The returned outcome is 0 or 1.
func (s *Simulator) Measure(qubit int, rng *rand.Rand) (int, float64, error) {
	if qubit < 0 || qubit >= s.qubits {
		return 0, 0, fmt.Errorf("simulator: qubit q%d is out of range", qubit)
	}
	bit := 1 << qubit
	probabilityOne := 0.0
	for i, amplitude := range s.state {
		if i&bit != 0 {
			probabilityOne += real(amplitude)*real(amplitude) + imag(amplitude)*imag(amplitude)
		}
	}
	if probabilityOne < 0 || probabilityOne > 1+1e-12 {
		return 0, 0, fmt.Errorf("simulator: invalid probability %.12f", probabilityOne)
	}
	if probabilityOne < 0 {
		probabilityOne = 0
	}
	if probabilityOne > 1 {
		probabilityOne = 1
	}

	outcome := 0
	if rng.Float64() < probabilityOne {
		outcome = 1
	}
	keepProbability := probabilityOne
	if outcome == 0 {
		keepProbability = 1 - probabilityOne
	}
	if keepProbability <= 0 {
		return 0, 0, fmt.Errorf("simulator: cannot normalize measurement outcome %d", outcome)
	}
	norm := math.Sqrt(keepProbability)
	for i := range s.state {
		if (i & bit) == 0 {
			if outcome == 0 {
				s.state[i] /= complex(norm, 0)
			} else {
				s.state[i] = 0
			}
		} else {
			if outcome == 1 {
				s.state[i] /= complex(norm, 0)
			} else {
				s.state[i] = 0
			}
		}
	}
	return outcome, probabilityOne, nil
}

// Stage captures the state immediately after one instruction.
type Stage struct {
	Index            int
	Kind             circuit.InstructionKind
	GateName         string
	Controls         []int
	Targets          []int
	ClassicalTargets []int
	Outcomes         []int
	State            []complex128
}

// Result contains one complete trajectory through a circuit.
type Result struct {
	Qubits        int
	State         []complex128
	ClassicalBits []int
	Stages        []Stage
}

// RunOptions controls deterministic execution and optional stage snapshots.
type RunOptions struct {
	Seed          uint64
	CaptureStages bool
}

// Run executes a circuit once. Measurement outcomes use a deterministic PCG
// random source seeded from opts.Seed.
func Run(c *circuit.Circuit, opts RunOptions) (*Result, error) {
	sim, err := New(c.Qubits)
	if err != nil {
		return nil, err
	}
	sim.state[0] = 1
	rng := rand.New(rand.NewPCG(opts.Seed, 0x9e3779b97f4a7c15))
	classical := make([]int, c.ClassicalBits)
	var stages []Stage
	if opts.CaptureStages {
		stages = make([]Stage, 0, len(c.Instructions))
	}

	for idx, ins := range c.Instructions {
		outcomes := []int{}
		switch ins.Kind {
		case circuit.InstructionGate:
			if err := sim.Apply(ins); err != nil {
				return nil, fmt.Errorf("simulator: instruction %d: %w", idx, err)
			}
		case circuit.InstructionMeasure:
			outcomes = make([]int, len(ins.Targets))
			for j, q := range ins.Targets {
				outcome, _, err := sim.Measure(q, rng)
				if err != nil {
					return nil, fmt.Errorf("simulator: instruction %d: %w", idx, err)
				}
				outcomes[j] = outcome
				classical[ins.ClassicalTargets[j]] = outcome
			}
		default:
			return nil, fmt.Errorf("simulator: instruction %d: unknown kind", idx)
		}
		if opts.CaptureStages {
			stages = append(stages, Stage{
				Index:            idx,
				Kind:             ins.Kind,
				GateName:         ins.Gate.Name,
				Controls:         append([]int(nil), ins.Controls...),
				Targets:          append([]int(nil), ins.Targets...),
				ClassicalTargets: append([]int(nil), ins.ClassicalTargets...),
				Outcomes:         append([]int(nil), outcomes...),
				State:            sim.State(),
			})
		}
	}
	return &Result{Qubits: c.Qubits, State: sim.State(), ClassicalBits: classical, Stages: stages}, nil
}

// Shot is one sampled measurement trajectory.
type Shot struct {
	Index     int
	Outcomes  []int
	BitString string
}

// SampleSet contains deterministic repeated measurement samples.
type SampleSet struct {
	Shots   int
	Seed    uint64
	Counts  map[string]int
	Results []Shot
}

// SampleShots executes the circuit from a fresh initial state for each shot.
// A single random stream is shared across shots, so the same seed produces the
// complete sample sequence reproducibly.
func SampleShots(c *circuit.Circuit, shots int, seed uint64) (*SampleSet, error) {
	if shots <= 0 {
		return nil, fmt.Errorf("simulator: shot count must be positive")
	}
	if c.ClassicalBits == 0 || !hasMeasurement(c) {
		return nil, fmt.Errorf("simulator: circuit contains no measurements to sample")
	}
	rng := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
	set := &SampleSet{Shots: shots, Seed: seed, Counts: map[string]int{}, Results: make([]Shot, 0, shots)}
	for shot := 0; shot < shots; shot++ {
		sim, err := New(c.Qubits)
		if err != nil {
			return nil, err
		}
		sim.state[0] = 1
		classical := make([]int, c.ClassicalBits)
		for idx, ins := range c.Instructions {
			if ins.Kind != circuit.InstructionMeasure {
				if err := sim.Apply(ins); err != nil {
					return nil, fmt.Errorf("simulator: shot %d instruction %d: %w", shot, idx, err)
				}
				continue
			}
			for j, q := range ins.Targets {
				outcome, _, err := sim.Measure(q, rng)
				if err != nil {
					return nil, fmt.Errorf("simulator: shot %d instruction %d: %w", shot, idx, err)
				}
				classical[ins.ClassicalTargets[j]] = outcome
			}
		}
		outcomes := append([]int(nil), classical...)
		bitString := bitString(classical)
		set.Counts[bitString]++
		set.Results = append(set.Results, Shot{Index: shot, Outcomes: outcomes, BitString: bitString})
	}
	return set, nil
}

func hasMeasurement(c *circuit.Circuit) bool {
	for _, ins := range c.Instructions {
		if ins.Kind == circuit.InstructionMeasure {
			return true
		}
	}
	return false
}

func bitString(bits []int) string {
	out := make([]byte, len(bits))
	for i, bit := range bits {
		out[i] = byte('0' + bit)
	}
	return string(out)
}
