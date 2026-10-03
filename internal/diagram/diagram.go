package diagram

import (
	"strings"

	"github.com/huchangjun118/quantum-circuit-sim-go/internal/circuit"
)

const cellWidth = 7

// Render returns an ASCII circuit diagram. Gates are placed in declaration
// order; controls and targets in the same instruction occupy the same column.
func Render(c *circuit.Circuit) string {
	rows := make([]strings.Builder, c.Qubits)
	for q := 0; q < c.Qubits; q++ {
		rows[q].WriteString("q")
		rows[q].WriteString(itoa(q))
		rows[q].WriteString(": ")
	}

	for _, ins := range c.Instructions {
		tokens := instructionTokens(ins, c.Qubits)
		for q := 0; q < c.Qubits; q++ {
			rows[q].WriteString(tokens[q])
		}
	}

	var out strings.Builder
	for q := 0; q < c.Qubits; q++ {
		out.WriteString(rows[q].String())
		out.WriteByte('\n')
	}
	return out.String()
}

func instructionTokens(ins circuit.Instruction, qubits int) []string {
	tokens := make([]string, qubits)
	for i := range tokens {
		tokens[i] = strings.Repeat("─", cellWidth)
	}

	switch ins.Kind {
	case circuit.InstructionMeasure:
		for _, q := range ins.Targets {
			tokens[q] = center("M")
		}
		connect(tokens, ins.Targets)
	case circuit.InstructionGate:
		controls := ins.Controls
		targets := ins.Targets
		all := append(append([]int{}, controls...), targets...)
		for _, q := range controls {
			tokens[q] = center("●")
		}
		for _, q := range targets {
			tokens[q] = targetToken(ins.Gate.Name)
		}
		connect(tokens, all)
	}
	return tokens
}

func connect(tokens []string, involved []int) {
	if len(involved) < 2 {
		return
	}
	min, max := involved[0], involved[0]
	for _, q := range involved {
		if q < min {
			min = q
		}
		if q > max {
			max = q
		}
	}
	for q := min + 1; q < max; q++ {
		if tokens[q] == strings.Repeat("─", cellWidth) {
			tokens[q] = center("│")
		}
	}
}

func targetToken(name string) string {
	switch name {
	case "CX", "CNOT", "CCX", "CCNOT", "MCX":
		return center("⊕")
	case "SWAP", "CSWAP":
		return center("×")
	case "CH":
		return center("H")
	case "CP":
		return center("P")
	case "CRX":
		return center("RX")
	case "CRY":
		return center("RY")
	case "CRZ":
		return center("RZ")
	case "CZ":
		return center("Z")
	case "CY":
		return center("Y")
	default:
		return center(name)
	}
}

func center(s string) string {
	runes := []rune(s)
	if len(runes) >= cellWidth {
		return s
	}
	total := cellWidth - len(runes)
	left := total / 2
	right := total - left
	return strings.Repeat("─", left) + s + strings.Repeat("─", right)
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
