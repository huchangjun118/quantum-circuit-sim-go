package diagram

import (
	"strings"
	"testing"

	"github.com/huchangjun118/quantum-circuit-sim-go/internal/circuit"
)

func TestRenderBellCircuit(t *testing.T) {
	c, err := circuit.Parse(strings.NewReader("qubits 2\nq0: H\nq0 q1: CX\nq0 -> c0\nq1 -> c1\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := Render(c)
	for _, want := range []string{"q0:", "q1:", "H", "●", "⊕", "M"} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagram missing %q:\n%s", want, got)
		}
	}
}

func TestRenderThreeQubitGate(t *testing.T) {
	c, err := circuit.Parse(strings.NewReader("qubits 3\nq0 q1 q2: CCX\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := Render(c)
	if !strings.Contains(got, "●") || !strings.Contains(got, "⊕") {
		t.Fatalf("CCX diagram missing controls/target:\n%s", got)
	}
}
