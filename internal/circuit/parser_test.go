package circuit

import (
	"errors"
	"strings"
	"testing"
)

func TestParseValidCircuit(t *testing.T) {
	src := `
# a comment
qubits 2
cbits 2
q0: H
q1: X
q0 q1: CX
q0: RX(1.25)
q0 -> c0
q1 -> c1
`
	c, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if c.Qubits != 2 || c.ClassicalBits != 2 {
		t.Fatalf("unexpected register sizes: %+v", c)
	}
	if len(c.Instructions) != 6 {
		t.Fatalf("expected 6 instructions, got %d", len(c.Instructions))
	}
	gate := c.Instructions[2]
	if gate.Gate.Name != "CX" || len(gate.Controls) != 1 || gate.Controls[0] != 0 || gate.Targets[0] != 1 {
		t.Fatalf("unexpected CX instruction: %+v", gate)
	}
	if c.Instructions[4].Kind != InstructionMeasure || c.Instructions[4].Targets[0] != 0 || c.Instructions[4].ClassicalTargets[0] != 0 {
		t.Fatalf("unexpected measurement: %+v", c.Instructions[4])
	}
}

func TestParseUnknownInstruction(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 1\nq0: H\nnot an instruction\n"))
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected ParseError, got %v", err)
	}
	if parseErr.Line != 3 {
		t.Fatalf("expected line 3, got %d", parseErr.Line)
	}
}

func TestParseUnknownGate(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 1\nq0: FOO\n"))
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if validationErr.Kind != ValidationUnknownGate || !errors.Is(err, ErrUnknownGate) {
		t.Fatalf("unexpected error category: %v", err)
	}
}

func TestRejectQubitOutOfRange(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 2\nq2: H\n"))
	assertValidationKind(t, err, ValidationQubitOutOfRange)
	if !errors.Is(err, ErrQubitOutOfRange) {
		t.Fatalf("error does not wrap ErrQubitOutOfRange: %v", err)
	}
}

func TestRejectMissingParameter(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 1\nq0: RX()\n"))
	assertValidationKind(t, err, ValidationMissingParameter)
	if !errors.Is(err, ErrMissingParameter) {
		t.Fatalf("error does not wrap ErrMissingParameter: %v", err)
	}
}

func TestRejectInvalidParameter(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 1\nq0: RX(abc)\n"))
	assertValidationKind(t, err, ValidationInvalidParameter)
}

func TestRejectInvalidArity(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 2\nq0 q1: H\n"))
	assertValidationKind(t, err, ValidationInvalidArity)
	if !errors.Is(err, ErrInvalidArity) {
		t.Fatalf("error does not wrap ErrInvalidArity: %v", err)
	}
}

func TestRejectDuplicateOperands(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 2\nq0 q0: CX\n"))
	assertValidationKind(t, err, ValidationConflictingDeclaration)
	if !errors.Is(err, ErrConflictingDeclaration) {
		t.Fatalf("error does not wrap ErrConflictingDeclaration: %v", err)
	}
}

func TestRejectDuplicateHeader(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 2\nqubits 3\n"))
	assertValidationKind(t, err, ValidationConflictingDeclaration)
}

func TestRejectMissingHeader(t *testing.T) {
	_, err := Parse(strings.NewReader("q0: H\n"))
	assertValidationKind(t, err, ValidationMissingHeader)
}

func TestRejectMeasurementCountMismatch(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 2\nq0 q1 -> c0\n"))
	assertValidationKind(t, err, ValidationConflictingDeclaration)
}

func TestRejectClassicalBitOutOfRange(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 1\ncbits 1\nq0 -> c1\n"))
	assertValidationKind(t, err, ValidationClassicalOutOfRange)
	if !errors.Is(err, ErrClassicalBitOutOfRange) {
		t.Fatalf("error does not wrap ErrClassicalBitOutOfRange: %v", err)
	}
}

func TestRejectDuplicateClassicalWrite(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 1\ncbits 2\nq0 -> c0\nq0 -> c0\n"))
	assertValidationKind(t, err, ValidationConflictingDeclaration)
}

func TestInferClassicalRegister(t *testing.T) {
	c, err := Parse(strings.NewReader("qubits 1\nq0 -> c3\n"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if c.ClassicalBits != 4 {
		t.Fatalf("expected inferred 4 classical bits, got %d", c.ClassicalBits)
	}
}

func assertValidationKind(t *testing.T, err error, kind ValidationKind) {
	t.Helper()
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if validationErr.Kind != kind {
		t.Fatalf("expected kind %s, got %s (line %d)", kind, validationErr.Kind, validationErr.Line)
	}
}

func TestRejectDuplicateMeasurementQubit(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 2\ncbits 2\nq0 q0 -> c0 c1\n"))
	assertValidationKind(t, err, ValidationConflictingDeclaration)
}

func TestParseErrorColumns(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits abc\n"))
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected ParseError, got %v", err)
	}
	if parseErr.Line != 1 || parseErr.Column != 8 {
		t.Fatalf("expected line 1 column 8, got line %d column %d", parseErr.Line, parseErr.Column)
	}

	_, err = Parse(strings.NewReader("qubits 1\nq0 -> cx\n"))
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected ParseError, got %v", err)
	}
	if parseErr.Line != 2 || parseErr.Column != 7 {
		t.Fatalf("expected line 2 column 7, got line %d column %d", parseErr.Line, parseErr.Column)
	}
}

func TestRejectUnexpectedGateParameter(t *testing.T) {
	_, err := Parse(strings.NewReader("qubits 1\nq0: H(1)\n"))
	assertValidationKind(t, err, ValidationInvalidParameter)
}
