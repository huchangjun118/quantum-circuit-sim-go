package circuit

import (
	"errors"
	"fmt"
)

// ValidationKind classifies semantic errors so callers can distinguish
// syntax problems from invalid circuit declarations.
type ValidationKind string

const (
	ValidationQubitOutOfRange        ValidationKind = "qubit_out_of_range"
	ValidationClassicalOutOfRange    ValidationKind = "classical_bit_out_of_range"
	ValidationMissingParameter       ValidationKind = "missing_parameter"
	ValidationInvalidParameter       ValidationKind = "invalid_parameter"
	ValidationUnknownGate            ValidationKind = "unknown_gate"
	ValidationInvalidArity           ValidationKind = "invalid_gate_arity"
	ValidationConflictingDeclaration ValidationKind = "conflicting_declaration"
	ValidationMissingHeader          ValidationKind = "missing_qubits_header"
)

// ValidationError describes a semantic error at a source location.
type ValidationError struct {
	Kind    ValidationKind
	Line    int
	Column  int
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s at line %d, column %d: %s", e.Kind, e.Line, e.Column, e.Message)
}

func (e *ValidationError) Unwrap() error {
	switch e.Kind {
	case ValidationQubitOutOfRange:
		return ErrQubitOutOfRange
	case ValidationClassicalOutOfRange:
		return ErrClassicalBitOutOfRange
	case ValidationMissingParameter:
		return ErrMissingParameter
	case ValidationInvalidParameter:
		return ErrInvalidParameter
	case ValidationUnknownGate:
		return ErrUnknownGate
	case ValidationInvalidArity:
		return ErrInvalidArity
	case ValidationConflictingDeclaration:
		return ErrConflictingDeclaration
	case ValidationMissingHeader:
		return ErrMissingHeader
	default:
		return nil
	}
}

// Sentinel errors make errors.Is possible for broad error categories.
var (
	ErrQubitOutOfRange        = errors.New("qubit index out of range")
	ErrClassicalBitOutOfRange = errors.New("classical bit index out of range")
	ErrMissingParameter       = errors.New("missing gate parameter")
	ErrInvalidParameter       = errors.New("invalid gate parameter")
	ErrUnknownGate            = errors.New("unknown gate")
	ErrInvalidArity           = errors.New("invalid gate arity")
	ErrConflictingDeclaration = errors.New("conflicting circuit declaration")
	ErrMissingHeader          = errors.New("missing qubits header")
)

// ParseError carries a source location for a malformed circuit description.
type ParseError struct {
	Line    int
	Column  int
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error at line %d, column %d: %s", e.Line, e.Column, e.Message)
}
