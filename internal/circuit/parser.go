package circuit

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Parse reads and validates a line-oriented circuit description.
//
// Example:
//
//	qubits 2
//	cbits 2
//	q0: H
//	q1: X
//	q0 q1: CX
//	q0 -> c0
//	q1 -> c1
//
// The returned circuit is safe to simulate: all semantic checks are completed
// before Parse returns, so callers cannot observe a partially built circuit.
func Parse(r io.Reader) (*Circuit, error) {
	c := &Circuit{}
	seenQubits := false
	seenCbits := false
	instructionsStarted := false
	seenClassicalWrite := map[int]bool{}

	scanner := bufio.NewScanner(r)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := stripComment(raw)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "qubits"):
			if instructionsStarted {
				return nil, &ValidationError{Kind: ValidationConflictingDeclaration, Line: lineNo, Column: 1, Message: "qubits declaration must precede all instructions"}
			}
			if seenQubits {
				return nil, &ValidationError{Kind: ValidationConflictingDeclaration, Line: lineNo, Column: 1, Message: "duplicate qubits declaration"}
			}
			n, col, err := parseHeaderInt(line, "qubits")
			if err != nil {
				return nil, &ParseError{Line: lineNo, Column: col, Message: err.Error()}
			}
			if n <= 0 {
				return nil, &ValidationError{Kind: ValidationInvalidParameter, Line: lineNo, Column: col, Message: "qubit count must be positive"}
			}
			c.Qubits = n
			seenQubits = true

		case strings.HasPrefix(trimmed, "cbits"):
			if instructionsStarted {
				return nil, &ValidationError{Kind: ValidationConflictingDeclaration, Line: lineNo, Column: 1, Message: "cbits declaration must precede all instructions"}
			}
			if seenCbits {
				return nil, &ValidationError{Kind: ValidationConflictingDeclaration, Line: lineNo, Column: 1, Message: "duplicate cbits declaration"}
			}
			n, col, err := parseHeaderInt(line, "cbits")
			if err != nil {
				return nil, &ParseError{Line: lineNo, Column: col, Message: err.Error()}
			}
			if n < 0 {
				return nil, &ValidationError{Kind: ValidationInvalidParameter, Line: lineNo, Column: col, Message: "classical bit count must be non-negative"}
			}
			c.ClassicalBits = n
			seenCbits = true

		case strings.Contains(line, "->"):
			instructionsStarted = true
			ins, err := parseMeasurement(line, lineNo)
			if err != nil {
				return nil, err
			}
			if c.Qubits > 0 {
				for _, q := range ins.Targets {
					if q < 0 || q >= c.Qubits {
						return nil, &ValidationError{Kind: ValidationQubitOutOfRange, Line: lineNo, Column: 1, Message: fmt.Sprintf("measurement target q%d is out of range for %d qubits", q, c.Qubits)}
					}
				}
			}
			for _, b := range ins.ClassicalTargets {
				if seenClassicalWrite[b] {
					return nil, &ValidationError{Kind: ValidationConflictingDeclaration, Line: lineNo, Column: 1, Message: fmt.Sprintf("classical bit c%d is assigned more than once", b)}
				}
				seenClassicalWrite[b] = true
			}
			c.Instructions = append(c.Instructions, ins)

		case strings.Contains(line, ":"):
			instructionsStarted = true
			ins, err := parseGate(line, lineNo)
			if err != nil {
				return nil, err
			}
			if err := validateGateArity(ins.Gate.Name, len(ins.Controls)+len(ins.Targets), lineNo); err != nil {
				return nil, err
			}
			if c.Qubits > 0 {
				for _, q := range append(append([]int{}, ins.Controls...), ins.Targets...) {
					if q < 0 || q >= c.Qubits {
						return nil, &ValidationError{Kind: ValidationQubitOutOfRange, Line: lineNo, Column: 1, Message: fmt.Sprintf("gate operand q%d is out of range for %d qubits", q, c.Qubits)}
					}
				}
			}
			if err := validateNoDuplicateOperands(ins, lineNo); err != nil {
				return nil, err
			}
			c.Instructions = append(c.Instructions, ins)

		default:
			return nil, &ParseError{Line: lineNo, Column: 1, Message: "expected a header, gate instruction, or measurement"}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, &ParseError{Line: lineNo, Column: 1, Message: err.Error()}
	}
	if !seenQubits {
		return nil, &ValidationError{Kind: ValidationMissingHeader, Line: 1, Column: 1, Message: "missing required qubits declaration"}
	}

	// Infer a classical register when measurements are present but cbits is
	// omitted. An explicit cbits declaration still enables range checks.
	if !seenCbits {
		for _, ins := range c.Instructions {
			if ins.Kind == InstructionMeasure {
				for _, b := range ins.ClassicalTargets {
					if b+1 > c.ClassicalBits {
						c.ClassicalBits = b + 1
					}
				}
			}
		}
	}
	for _, ins := range c.Instructions {
		if ins.Kind == InstructionMeasure {
			for _, b := range ins.ClassicalTargets {
				if b < 0 || b >= c.ClassicalBits {
					return nil, &ValidationError{Kind: ValidationClassicalOutOfRange, Line: ins.Line, Column: 1, Message: fmt.Sprintf("classical bit c%d is out of range for %d classical bits", b, c.ClassicalBits)}
				}
			}
		}
	}
	return c, nil
}

func stripComment(line string) string {
	if i := strings.Index(line, "#"); i >= 0 {
		return line[:i]
	}
	return line
}

func parseHeaderInt(line, keyword string) (int, int, error) {
	idx := strings.Index(line, keyword)
	rest := line[idx+len(keyword):]
	fields := strings.Fields(rest)
	if len(fields) != 1 {
		return 0, idx + len(keyword) + 1, fmt.Errorf("expected %s <non-negative integer>", keyword)
	}
	value, err := strconv.Atoi(fields[0])
	col := idx + len(keyword) + strings.Index(rest, fields[0]) + 1
	if err != nil {
		return 0, col, fmt.Errorf("invalid integer %q", fields[0])
	}
	return value, col, nil
}

func parseMeasurement(line string, lineNo int) (Instruction, error) {
	parts := strings.SplitN(line, "->", 2)
	if len(parts) != 2 {
		return Instruction{}, &ParseError{Line: lineNo, Column: 1, Message: "measurement must use q... -> c..."}
	}
	qubits, err := parseQubitRefs(parts[0], lineNo, 0)
	if err != nil {
		return Instruction{}, err
	}
	arrowAt := strings.Index(line, "->")
	bits, err := parseClassicalRefs(parts[1], lineNo, arrowAt+len("->"))
	if err != nil {
		return Instruction{}, err
	}
	if len(qubits) == 0 || len(bits) == 0 {
		return Instruction{}, &ParseError{Line: lineNo, Column: 1, Message: "measurement requires at least one qubit and one classical bit"}
	}
	if duplicate := firstDuplicate(qubits); duplicate >= 0 {
		return Instruction{}, &ValidationError{Kind: ValidationConflictingDeclaration, Line: lineNo, Column: 1, Message: fmt.Sprintf("duplicate qubit q%d in measurement", duplicate)}
	}
	if len(qubits) != len(bits) {
		return Instruction{}, &ValidationError{Kind: ValidationConflictingDeclaration, Line: lineNo, Column: 1, Message: "measurement qubit and classical bit counts differ"}
	}
	return Instruction{Kind: InstructionMeasure, Targets: qubits, ClassicalTargets: bits, Line: lineNo}, nil
}

func firstDuplicate(values []int) int {
	seen := map[int]bool{}
	for _, value := range values {
		if seen[value] {
			return value
		}
		seen[value] = true
	}
	return -1
}

func parseGate(line string, lineNo int) (Instruction, error) {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return Instruction{}, &ParseError{Line: lineNo, Column: 1, Message: "gate instruction must use <qubits>: <gate>"}
	}
	refs, err := parseQubitRefs(line[:colon], lineNo, 0)
	if err != nil {
		return Instruction{}, err
	}
	if len(refs) == 0 {
		return Instruction{}, &ParseError{Line: lineNo, Column: 1, Message: "gate instruction requires at least one qubit operand"}
	}
	gate, err := parseGateName(line[colon+1:], lineNo, colon+2)
	if err != nil {
		return Instruction{}, err
	}
	controls, targets, err := splitOperands(gate.Name, refs, lineNo)
	if err != nil {
		return Instruction{}, err
	}
	return Instruction{Kind: InstructionGate, Gate: gate, Controls: controls, Targets: targets, Line: lineNo}, nil
}

func parseQubitRefs(s string, lineNo, baseColumn int) ([]int, error) {
	var refs []int
	for _, field := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		col := baseColumn + strings.Index(s, field) + 1
		if len(field) < 2 || field[0] != 'q' {
			return nil, &ParseError{Line: lineNo, Column: col, Message: fmt.Sprintf("invalid qubit reference %q; expected q<index>", field)}
		}
		n, err := strconv.Atoi(field[1:])
		if err != nil || n < 0 {
			return nil, &ParseError{Line: lineNo, Column: col, Message: fmt.Sprintf("invalid qubit index %q", field)}
		}
		refs = append(refs, n)
	}
	return refs, nil
}

func parseClassicalRefs(s string, lineNo, baseColumn int) ([]int, error) {
	var refs []int
	for _, field := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		col := baseColumn + strings.Index(s, field) + 1
		if len(field) < 2 || field[0] != 'c' {
			return nil, &ParseError{Line: lineNo, Column: col, Message: fmt.Sprintf("invalid classical bit reference %q; expected c<index>", field)}
		}
		n, err := strconv.Atoi(field[1:])
		if err != nil || n < 0 {
			return nil, &ParseError{Line: lineNo, Column: col, Message: fmt.Sprintf("invalid classical bit index %q", field)}
		}
		refs = append(refs, n)
	}
	return refs, nil
}

func parseGateName(s string, lineNo, baseColumn int) (Gate, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return Gate{}, &ParseError{Line: lineNo, Column: baseColumn, Message: "missing gate name"}
	}
	name := trimmed
	params := []float64{}
	if open := strings.Index(trimmed, "("); open >= 0 {
		closeParen := strings.LastIndex(trimmed, ")")
		if closeParen < open || closeParen != len(trimmed)-1 {
			return Gate{}, &ParseError{Line: lineNo, Column: baseColumn + open + 1, Message: "malformed gate parameter list"}
		}
		name = strings.TrimSpace(trimmed[:open])
		raw := strings.TrimSpace(trimmed[open+1 : closeParen])
		if raw == "" {
			return Gate{}, &ValidationError{Kind: ValidationMissingParameter, Line: lineNo, Column: baseColumn + open + 1, Message: fmt.Sprintf("gate %s requires a parameter", name)}
		}
		for _, part := range strings.Split(raw, ",") {
			value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
			if err != nil {
				return Gate{}, &ValidationError{Kind: ValidationInvalidParameter, Line: lineNo, Column: baseColumn + open + 1, Message: fmt.Sprintf("invalid parameter %q for gate %s", part, name)}
			}
			params = append(params, value)
		}
	}
	name = strings.ToUpper(name)
	if !knownGate(name) {
		return Gate{}, &ValidationError{Kind: ValidationUnknownGate, Line: lineNo, Column: baseColumn, Message: fmt.Sprintf("unknown gate %q", name)}
	}
	if err := validateParameterCount(name, len(params), lineNo, baseColumn); err != nil {
		return Gate{}, err
	}
	return Gate{Name: name, Params: params}, nil
}

func splitOperands(name string, operands []int, lineNo int) ([]int, []int, error) {
	switch name {
	case "CX", "CNOT", "CY", "CZ", "CH", "CP", "CRX", "CRY", "CRZ":
		return operands[:len(operands)-1], operands[len(operands)-1:], nil
	case "CCX", "CCNOT":
		return operands[:len(operands)-1], operands[len(operands)-1:], nil
	case "CSWAP":
		return operands[:1], operands[1:], nil
	case "MCX":
		if len(operands) < 2 {
			return nil, nil, &ValidationError{Kind: ValidationInvalidArity, Line: lineNo, Column: 1, Message: "MCX requires at least one control and one target"}
		}
		return operands[:len(operands)-1], operands[len(operands)-1:], nil
	default:
		return nil, operands, nil
	}
}

func validateGateArity(name string, operandCount, lineNo int) error {
	want, ok := gateArities[name]
	if !ok {
		return &ValidationError{Kind: ValidationUnknownGate, Line: lineNo, Column: 1, Message: fmt.Sprintf("unknown gate %q", name)}
	}
	if want < 0 {
		if operandCount < 2 {
			return &ValidationError{Kind: ValidationInvalidArity, Line: lineNo, Column: 1, Message: "MCX expects at least one control and one target"}
		}
		return nil
	}
	if operandCount != want {
		return &ValidationError{Kind: ValidationInvalidArity, Line: lineNo, Column: 1, Message: fmt.Sprintf("gate %s expects %d qubit operands, got %d", name, want, operandCount)}
	}
	return nil
}

func validateParameterCount(name string, count, lineNo, col int) error {
	want, ok := parameterCounts[name]
	if !ok {
		if count > 0 {
			return &ValidationError{Kind: ValidationInvalidParameter, Line: lineNo, Column: col, Message: fmt.Sprintf("gate %s does not accept parameters, got %d", name, count)}
		}
		return nil
	}
	if count != want {
		return &ValidationError{Kind: ValidationMissingParameter, Line: lineNo, Column: col, Message: fmt.Sprintf("gate %s expects %d parameter(s), got %d", name, want, count)}
	}
	return nil
}

func validateNoDuplicateOperands(ins Instruction, lineNo int) error {
	seen := map[int]bool{}
	for _, q := range ins.Controls {
		if seen[q] {
			return &ValidationError{Kind: ValidationConflictingDeclaration, Line: lineNo, Column: 1, Message: fmt.Sprintf("duplicate qubit q%d in gate operands", q)}
		}
		seen[q] = true
	}
	for _, q := range ins.Targets {
		if seen[q] {
			return &ValidationError{Kind: ValidationConflictingDeclaration, Line: lineNo, Column: 1, Message: fmt.Sprintf("duplicate qubit q%d in gate operands", q)}
		}
		seen[q] = true
	}
	return nil
}

func knownGate(name string) bool {
	_, ok := gateArities[name]
	return ok
}

var gateArities = map[string]int{
	"I": 1, "ID": 1, "H": 1, "X": 1, "Y": 1, "Z": 1, "S": 1, "SDG": 1,
	"T": 1, "TDG": 1, "SX": 1, "RX": 1, "RY": 1, "RZ": 1, "P": 1,
	"U":  1,
	"CX": 2, "CNOT": 2, "CY": 2, "CZ": 2, "CH": 2, "CP": 2,
	"CRX": 2, "CRY": 2, "CRZ": 2, "SWAP": 2,
	"CCX": 3, "CCNOT": 3, "CSWAP": 3,
	"MCX": -1,
}

var parameterCounts = map[string]int{
	"RX": 1, "RY": 1, "RZ": 1, "P": 1, "CP": 1,
	"CRX": 1, "CRY": 1, "CRZ": 1, "U": 3,
}
