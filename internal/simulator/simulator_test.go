package simulator

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/huchangjun118/quantum-circuit-sim-go/internal/circuit"
)

func TestBellStateAndDeterminism(t *testing.T) {
	c := mustParse(t, "qubits 2\nq0: H\nq0 q1: CX\n")
	first, err := Run(c, RunOptions{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	second, err := Run(c, RunOptions{})
	if err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	want := 1 / math.Sqrt2
	assertAmplitude(t, first.State, 0, want, 0)
	assertAmplitude(t, first.State, 3, want, 0)
	for i := range first.State {
		if i != 0 && i != 3 {
			assertAmplitude(t, first.State, i, 0, 0)
		}
	}
	for i := range first.State {
		if first.State[i] != second.State[i] {
			t.Fatalf("state is not deterministic at index %d: %v vs %v", i, first.State[i], second.State[i])
		}
	}
}

func TestGHZState(t *testing.T) {
	c := mustParse(t, "qubits 3\nq0: H\nq0 q1: CX\nq1 q2: CX\n")
	result, err := Run(c, RunOptions{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	want := 1 / math.Sqrt2
	assertAmplitude(t, result.State, 0, want, 0)
	assertAmplitude(t, result.State, 7, want, 0)
	for i := range result.State {
		if i != 0 && i != 7 {
			assertAmplitude(t, result.State, i, 0, 0)
		}
	}
}

func TestPauliAndSwapGates(t *testing.T) {
	c := mustParse(t, "qubits 2\nq0: X\nq0 q1: SWAP\n")
	result, err := Run(c, RunOptions{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	// X|00> = |10> (q0=1), then SWAP q0,q1 = |01> (index 2).
	assertAmplitude(t, result.State, 2, 1, 0)
}

func TestMeasurementCollapseMatchesOutcome(t *testing.T) {
	seen0, seen1 := false, false
	for seed := uint64(0); seed < 1000 && !(seen0 && seen1); seed++ {
		sim, err := New(2)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		if err := sim.Apply(circuit.Instruction{Kind: circuit.InstructionGate, Gate: circuit.Gate{Name: "H"}, Targets: []int{0}}); err != nil {
			t.Fatalf("Apply H: %v", err)
		}
		rng := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
		outcome, probability, err := sim.Measure(0, rng)
		if err != nil {
			t.Fatalf("Measure: %v", err)
		}
		if math.Abs(probability-0.5) > 1e-12 {
			t.Fatalf("expected probability 0.5, got %.12f", probability)
		}
		if outcome == 0 {
			seen0 = true
			assertAmplitude(t, sim.State(), 0, 1, 0)
			assertAmplitude(t, sim.State(), 1, 0, 0)
		} else {
			seen1 = true
			assertAmplitude(t, sim.State(), 0, 0, 0)
			assertAmplitude(t, sim.State(), 1, 1, 0)
		}
		for i := range sim.State() {
			if i != 0 && i != 1 {
				assertAmplitude(t, sim.State(), i, 0, 0)
			}
		}
	}
	if !seen0 || !seen1 {
		t.Fatalf("did not observe both measurement outcomes")
	}
}

func TestSampleShotsAreReproducible(t *testing.T) {
	c := mustParse(t, "qubits 2\ncbits 2\nq0: H\nq0 q1: CX\nq0 -> c0\nq1 -> c1\n")
	first, err := SampleShots(c, 100, 42)
	if err != nil {
		t.Fatalf("SampleShots: %v", err)
	}
	second, err := SampleShots(c, 100, 42)
	if err != nil {
		t.Fatalf("second SampleShots: %v", err)
	}
	if len(first.Results) != 100 || len(second.Results) != 100 {
		t.Fatalf("expected 100 results")
	}
	for i := range first.Results {
		if first.Results[i].BitString != second.Results[i].BitString {
			t.Fatalf("sample sequence differs at shot %d", i)
		}
	}
	for bitString := range first.Counts {
		if bitString != "00" && bitString != "11" {
			t.Fatalf("unexpected Bell measurement outcome %q", bitString)
		}
	}
	if first.Counts["00"]+first.Counts["11"] != 100 {
		t.Fatalf("counts do not sum to shots: %+v", first.Counts)
	}
}

func TestStageSnapshotsPreserveOrderAndTargets(t *testing.T) {
	c := mustParse(t, "qubits 2\nq0: H\nq0 q1: CX\nq1: X\n")
	result, err := Run(c, RunOptions{CaptureStages: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Stages) != 3 {
		t.Fatalf("expected 3 stages, got %d", len(result.Stages))
	}
	if result.Stages[0].GateName != "H" || len(result.Stages[0].Targets) != 1 || result.Stages[0].Targets[0] != 0 {
		t.Fatalf("unexpected first stage: %+v", result.Stages[0])
	}
	if result.Stages[1].GateName != "CX" || result.Stages[1].Controls[0] != 0 || result.Stages[1].Targets[0] != 1 {
		t.Fatalf("unexpected second stage: %+v", result.Stages[1])
	}
	if result.Stages[2].GateName != "X" || result.Stages[2].Targets[0] != 1 {
		t.Fatalf("unexpected third stage: %+v", result.Stages[2])
	}
	assertAmplitude(t, result.Stages[0].State, 0, 1/math.Sqrt2, 0)
	assertAmplitude(t, result.Stages[1].State, 3, 1/math.Sqrt2, 0)
	assertAmplitude(t, result.Stages[2].State, 1, 1/math.Sqrt2, 0)
}

func TestTwentyQubitGHZSampling(t *testing.T) {
	var src strings.Builder
	src.WriteString("qubits 20\ncbits 20\n")
	src.WriteString("q0: H\n")
	for q := 0; q < 19; q++ {
		src.WriteString("q")
		src.WriteString(itoa(q))
		src.WriteString(" q")
		src.WriteString(itoa(q + 1))
		src.WriteString(": CX\n")
	}
	for q := 0; q < 20; q++ {
		src.WriteString("q")
		src.WriteString(itoa(q))
		src.WriteString(" -> c")
		src.WriteString(itoa(q))
		src.WriteByte('\n')
	}
	c := mustParse(t, src.String())
	samples, err := SampleShots(c, 20, 7)
	if err != nil {
		t.Fatalf("SampleShots: %v", err)
	}
	for bitString := range samples.Counts {
		if bitString != strings.Repeat("0", 20) && bitString != strings.Repeat("1", 20) {
			t.Fatalf("unexpected 20-qubit GHZ outcome %q", bitString)
		}
	}
}

func mustParse(t *testing.T, src string) *circuit.Circuit {
	t.Helper()
	c, err := circuit.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return c
}

func assertAmplitude(t *testing.T, state []complex128, index int, wantReal, wantImag float64) {
	t.Helper()
	if index >= len(state) {
		t.Fatalf("index %d out of range for state of length %d", index, len(state))
	}
	got := state[index]
	if math.Abs(real(got)-wantReal) > 1e-9 || math.Abs(imag(got)-wantImag) > 1e-9 {
		t.Fatalf("amplitude[%d] = %.9f%+.9fi, want %.9f%+.9fi", index, real(got), imag(got), wantReal, wantImag)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func TestParameterizedGates(t *testing.T) {
	c := mustParse(t, "qubits 1\nq0: RX(3.141592653589793)\n")
	result, err := Run(c, RunOptions{})
	if err != nil {
		t.Fatalf("Run RX(pi): %v", err)
	}
	assertAmplitude(t, result.State, 1, 0, -1)

	c = mustParse(t, "qubits 1\nq0: H\nq0: RZ(3.141592653589793)\n")
	result, err = Run(c, RunOptions{})
	if err != nil {
		t.Fatalf("Run H then RZ(pi): %v", err)
	}
	want := 1 / math.Sqrt2
	assertAmplitude(t, result.State, 0, 0, -want)
	assertAmplitude(t, result.State, 1, 0, want)
}
