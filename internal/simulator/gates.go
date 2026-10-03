package simulator

import (
	"fmt"
	"math"

	"github.com/huchangjun118/quantum-circuit-sim-go/internal/circuit"
)

type matrix2 [2][2]complex128

func gateMatrix(g circuit.Gate) (matrix2, error) {
	switch g.Name {
	case "I", "ID":
		return matrix2{{1, 0}, {0, 1}}, nil
	case "H", "CH":
		v := 1 / math.Sqrt2
		return matrix2{{complex(v, 0), complex(v, 0)}, {complex(v, 0), complex(-v, 0)}}, nil
	case "X", "CX", "CNOT", "CCX", "CCNOT", "MCX":
		return matrix2{{0, 1}, {1, 0}}, nil
	case "Y", "CY":
		return matrix2{{0, -1i}, {1i, 0}}, nil
	case "Z", "CZ":
		return matrix2{{1, 0}, {0, -1}}, nil
	case "S":
		return matrix2{{1, 0}, {0, 1i}}, nil
	case "SDG":
		return matrix2{{1, 0}, {0, -1i}}, nil
	case "T":
		return matrix2{{1, 0}, {0, complex(math.Cos(math.Pi/4), math.Sin(math.Pi/4))}}, nil
	case "TDG":
		return matrix2{{1, 0}, {0, complex(math.Cos(-math.Pi/4), math.Sin(-math.Pi/4))}}, nil
	case "SX":
		return matrix2{{complex(0.5, 0.5), complex(0.5, -0.5)}, {complex(0.5, -0.5), complex(0.5, 0.5)}}, nil
	case "RX":
		theta := g.Params[0]
		c := math.Cos(theta / 2)
		s := math.Sin(theta / 2)
		return matrix2{{complex(c, 0), complex(0, -s)}, {complex(0, -s), complex(c, 0)}}, nil
	case "RY":
		theta := g.Params[0]
		c := math.Cos(theta / 2)
		s := math.Sin(theta / 2)
		return matrix2{{complex(c, 0), complex(-s, 0)}, {complex(s, 0), complex(c, 0)}}, nil
	case "RZ":
		theta := g.Params[0]
		return matrix2{{complex(math.Cos(-theta/2), math.Sin(-theta/2)), 0}, {0, complex(math.Cos(theta/2), math.Sin(theta/2))}}, nil
	case "P", "CP":
		phi := g.Params[0]
		return matrix2{{1, 0}, {0, complex(math.Cos(phi), math.Sin(phi))}}, nil
	case "CRX":
		return gateMatrix(circuit.Gate{Name: "RX", Params: g.Params})
	case "CRY":
		return gateMatrix(circuit.Gate{Name: "RY", Params: g.Params})
	case "CRZ":
		return gateMatrix(circuit.Gate{Name: "RZ", Params: g.Params})
	case "U":
		theta, phi, lambda := g.Params[0], g.Params[1], g.Params[2]
		c := math.Cos(theta / 2)
		s := math.Sin(theta / 2)
		return matrix2{
			{complex(c, 0), -complex(math.Cos(lambda), math.Sin(lambda)) * complex(s, 0)},
			{complex(math.Cos(phi), math.Sin(phi)) * complex(s, 0), complex(math.Cos(phi+lambda), math.Sin(phi+lambda)) * complex(c, 0)},
		}, nil
	default:
		return matrix2{}, fmt.Errorf("simulator: unsupported gate %q", g.Name)
	}
}

// applyGate applies one validated instruction. Gate matrices are applied with
// bit-mask loops so allocation is avoided and each amplitude is updated once.
func (s *Simulator) applyGate(ins circuit.Instruction) error {
	if len(ins.Controls) > 0 && len(ins.Targets) == 1 {
		m, err := gateMatrix(ins.Gate)
		if err != nil {
			return err
		}
		s.applyControlledSingleTarget(m, ins.Controls, ins.Targets[0])
		return nil
	}

	switch ins.Gate.Name {
	case "SWAP":
		s.applySwap(ins.Targets[0], ins.Targets[1])
	case "CSWAP":
		s.applyControlledSwap(ins.Controls[0], ins.Targets[0], ins.Targets[1])
	default:
		m, err := gateMatrix(ins.Gate)
		if err != nil {
			return err
		}
		s.applySingleQubit(m, ins.Targets[0])
	}
	return nil
}

func (s *Simulator) applySingleQubit(m matrix2, target int) {
	bit := 1 << target
	for base := 0; base < len(s.state); base++ {
		if base&bit != 0 {
			continue
		}
		a0 := s.state[base]
		a1 := s.state[base|bit]
		s.state[base] = m[0][0]*a0 + m[0][1]*a1
		s.state[base|bit] = m[1][0]*a0 + m[1][1]*a1
	}
}

func (s *Simulator) applyControlledSingleTarget(m matrix2, controls []int, target int) {
	targetBit := 1 << target
	controlMask := 0
	for _, q := range controls {
		controlMask |= 1 << q
	}
	for base := 0; base < len(s.state); base++ {
		if base&targetBit != 0 || base&controlMask != controlMask {
			continue
		}
		a0 := s.state[base]
		a1 := s.state[base|targetBit]
		s.state[base] = m[0][0]*a0 + m[0][1]*a1
		s.state[base|targetBit] = m[1][0]*a0 + m[1][1]*a1
	}
}

func (s *Simulator) applySwap(a, b int) {
	bitA := 1 << a
	bitB := 1 << b
	mask := bitA | bitB
	for base := 0; base < len(s.state); base++ {
		if base&mask != 0 {
			continue
		}
		swap := base | bitA
		other := base | bitB
		s.state[swap], s.state[other] = s.state[other], s.state[swap]
	}
}

func (s *Simulator) applyControlledSwap(control, a, b int) {
	controlBit := 1 << control
	bitA := 1 << a
	bitB := 1 << b
	targetMask := bitA | bitB
	for base := 0; base < len(s.state); base++ {
		if base&controlBit == 0 || base&targetMask != 0 {
			continue
		}
		swap := base | bitA
		other := base | bitB
		s.state[swap], s.state[other] = s.state[other], s.state[swap]
	}
}
