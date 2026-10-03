package circuit

// Circuit is a fully parsed and validated quantum circuit.
type Circuit struct {
	Qubits        int
	ClassicalBits int
	Instructions  []Instruction
}

// InstructionKind identifies the kind of circuit instruction.
type InstructionKind int

const (
	InstructionGate InstructionKind = iota
	InstructionMeasure
)

// Instruction is one ordered operation in a circuit.
type Instruction struct {
	Kind             InstructionKind
	Gate             Gate
	Controls         []int
	Targets          []int
	ClassicalTargets []int
	Line             int
}

// Gate describes a quantum gate and its optional parameters.
type Gate struct {
	Name   string
	Params []float64
}
