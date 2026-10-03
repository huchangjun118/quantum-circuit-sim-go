package qsim

import (
	"math"
	"math/cmplx"
)

// ci 是虚数单位 i。
const ci = complex(0.0, 1.0)

// Gate 表示一个量子门的声明（或一次测量）。
//
// Name 保存“基门”名称（受控门的 C/CC 前缀被剥离，控制位放在 Controls），
// 例如 CX 0,1 解析为 Name="X", Controls=[0], Targets=[1]。
type Gate struct {
	Name      string    // 基门名称（大写）：I X Y Z H S SDG T TDG RX RY RZ P SWAP
	Targets   []int     // 非控制目标比特（SWAP 有 2 个，其余 1 个）
	Controls  []int     // 控制比特（C 前缀 1 个，CC 前缀 2 个）
	Params    []float64 // 旋转门参数，单位：度
	IsMeasure bool      // 是否为测量指令
	Line      int       // 源文件行号（从 1 开始）
	Column    int       // 指令起始列（从 1 开始）
	Raw       string    // 原始文本（去注释、去首尾空白后）
}

// Operands 按声明顺序返回全部操作数（控制位在前，目标位在后）。
func (g *Gate) Operands() []int {
	out := make([]int, 0, len(g.Controls)+len(g.Targets))
	out = append(out, g.Controls...)
	out = append(out, g.Targets...)
	return out
}

// IsParametrized 判断门是否需要角度参数。
func (g *Gate) IsParametrized() bool {
	switch g.Name {
	case "RX", "RY", "RZ", "P":
		return true
	default:
		return false
	}
}

// 无参数单比特酉门集合。
var unitarySingleGates = map[string]bool{
	"I": true, "X": true, "Y": true, "Z": true, "H": true,
	"S": true, "SDG": true, "T": true, "TDG": true,
}

// 参数化单比特门集合。
var parametricSingleGates = map[string]bool{
	"RX": true, "RY": true, "RZ": true, "P": true,
}

// GateInfo 解析（可能受控的）门名，返回：
//   - base: 基门名称
//   - nControls: 控制位数量（0/1/2）
//   - ok: 名称是否被识别
func GateInfo(name string) (base string, nControls int, ok bool) {
	upper := name
	if len(upper) >= 3 && upper[:2] == "CC" {
		b := upper[2:]
		if unitarySingleGates[b] || parametricSingleGates[b] {
			return b, 2, true
		}
		return "", 0, false
	}
	if len(upper) >= 2 && upper[0] == 'C' {
		b := upper[1:]
		if unitarySingleGates[b] || parametricSingleGates[b] {
			return b, 1, true
		}
		return "", 0, false
	}
	if unitarySingleGates[upper] || parametricSingleGates[upper] || upper == "SWAP" {
		return upper, 0, true
	}
	return "", 0, false
}

// TargetArity 返回基门需要的非控制目标数。
func TargetArity(base string) int {
	if base == "SWAP" {
		return 2
	}
	return 1
}

// ParamCount 返回基门需要的参数个数。
func ParamCount(base string) int {
	if parametricSingleGates[base] {
		return 1
	}
	return 0
}

// Mat 返回基门作用在目标比特上的稠密酉矩阵：
// 单比特门为 2x2，SWAP 为 4x4。受控逻辑由模拟器处理，不在此矩阵内。
// 测量指令返回 nil。
func (g *Gate) Mat() [][]complex128 {
	if g.IsMeasure {
		return nil
	}
	switch g.Name {
	case "I":
		return matI()
	case "X":
		return matX()
	case "Y":
		return matY()
	case "Z":
		return matZ()
	case "H":
		return matH()
	case "S":
		return matS()
	case "SDG":
		return matSDG()
	case "T":
		return matT()
	case "TDG":
		return matTDG()
	case "SWAP":
		return matSWAP()
	case "RX":
		return matRX(deg2rad(g.Params[0]))
	case "RY":
		return matRY(deg2rad(g.Params[0]))
	case "RZ":
		return matRZ(deg2rad(g.Params[0]))
	case "P":
		return matP(deg2rad(g.Params[0]))
	default:
		return nil
	}
}

func deg2rad(d float64) float64 { return d * math.Pi / 180.0 }

// 确保 math/cmplx 被引用（矩阵构造的辅助）。
var _ = cmplx.Abs

func matI() [][]complex128 {
	return [][]complex128{
		{1, 0},
		{0, 1},
	}
}

func matX() [][]complex128 {
	return [][]complex128{
		{0, 1},
		{1, 0},
	}
}

func matY() [][]complex128 {
	return [][]complex128{
		{0, -ci},
		{ci, 0},
	}
}

func matZ() [][]complex128 {
	return [][]complex128{
		{1, 0},
		{0, -1},
	}
}

func matH() [][]complex128 {
	s := complex(1.0/math.Sqrt2, 0)
	return [][]complex128{
		{s, s},
		{s, -s},
	}
}

func matS() [][]complex128 {
	return [][]complex128{
		{1, 0},
		{0, ci},
	}
}

func matSDG() [][]complex128 {
	return [][]complex128{
		{1, 0},
		{0, -ci},
	}
}

func matT() [][]complex128 {
	return [][]complex128{
		{1, 0},
		{0, complex(math.Cos(math.Pi/4), math.Sin(math.Pi/4))},
	}
}

func matTDG() [][]complex128 {
	return [][]complex128{
		{1, 0},
		{0, complex(math.Cos(math.Pi/4), -math.Sin(math.Pi/4))},
	}
}

func matSWAP() [][]complex128 {
	return [][]complex128{
		{1, 0, 0, 0},
		{0, 0, 1, 0},
		{0, 1, 0, 0},
		{0, 0, 0, 1},
	}
}

// RX(θ) = exp(-i θ X / 2)，θ 为弧度。
func matRX(theta float64) [][]complex128 {
	c := complex(math.Cos(theta/2), 0)
	s := complex(math.Sin(theta/2), 0)
	return [][]complex128{
		{c, -ci * s},
		{-ci * s, c},
	}
}

// RY(θ) = exp(-i θ Y / 2)。
func matRY(theta float64) [][]complex128 {
	c := complex(math.Cos(theta/2), 0)
	s := complex(math.Sin(theta/2), 0)
	return [][]complex128{
		{c, -s},
		{s, c},
	}
}

// RZ(θ) = exp(-i θ Z / 2)。
func matRZ(theta float64) [][]complex128 {
	em := cmplx.Exp(complex(0, -theta/2))
	ep := cmplx.Exp(complex(0, theta/2))
	return [][]complex128{
		{em, 0},
		{0, ep},
	}
}

// P(θ) = diag(1, e^{iθ})。
func matP(theta float64) [][]complex128 {
	return [][]complex128{
		{1, 0},
		{0, cmplx.Exp(complex(0, theta))},
	}
}
