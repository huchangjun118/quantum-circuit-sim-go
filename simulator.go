package qsim

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
)

// Simulator 在态矢量上执行线路。
//
// 态矢量使用长度 2^n 的 []complex128 稠密表示；每扇门的代价为 O(2^n)，
// 内存占用为 O(2^n)，不随门数或控制位数量产生额外的指数爆炸。
type Simulator struct {
	n       int
	state   []complex128
	rng     *rand.Rand // 非 nil 时使用独立种子源（可复现）
	records []MeasurementRecord
}

// NewSimulator 创建 n 比特仿真器，初态为 |0...0>，测量使用全局随机源。
func NewSimulator(n int) *Simulator {
	return &Simulator{n: n, state: zeroState(n)}
}

// NewSeededSimulator 创建带固定随机种子的仿真器：
// 同一种子、同一线路的测量/采样序列逐位一致。
func NewSeededSimulator(n int, seed int64) *Simulator {
	return &Simulator{
		n:     n,
		state: zeroState(n),
		rng:   rand.New(rand.NewSource(seed)),
	}
}

func zeroState(n int) []complex128 {
	st := make([]complex128, 1<<n)
	st[0] = 1
	return st
}

func (s *Simulator) randFloat() float64 {
	if s.rng != nil {
		return s.rng.Float64()
	}
	return rand.Float64()
}

// N 返回量子比特数。
func (s *Simulator) N() int { return s.n }

// Run 重置为 |0...0> 后按声明顺序执行整条线路。
// 对同一个 Simulator 重复调用结果一致（确定性演化 + 确定性种子）。
func (s *Simulator) Run(c *Circuit) error {
	if c.NQubits != s.n {
		return fmt.Errorf("simulator has %d qubits but circuit declares %d", s.n, c.NQubits)
	}
	s.state = zeroState(s.n)
	s.records = s.records[:0]
	for _, g := range c.Gates {
		if err := s.ApplyGate(g); err != nil {
			return err
		}
	}
	return nil
}

// StateVector 返回当前态矢量的防御性拷贝。
func (s *Simulator) StateVector() []complex128 {
	out := make([]complex128, len(s.state))
	copy(out, s.state)
	return out
}

// Probs 返回各计算基态的测量概率 |amplitude|^2（拷贝）。
func (s *Simulator) Probs() []float64 {
	out := make([]float64, len(s.state))
	for i, a := range s.state {
		re := real(a)
		im := imag(a)
		out[i] = re*re + im*im
	}
	return out
}

// Measurements 返回截至目前发生的测量记录（拷贝）。
func (s *Simulator) Measurements() []MeasurementRecord {
	out := make([]MeasurementRecord, len(s.records))
	copy(out, s.records)
	return out
}

// ApplyGate 将单个门作用于当前态矢量；测量门立即坍缩。
func (s *Simulator) ApplyGate(g *Gate) error {
	for _, q := range g.Operands() {
		if q < 0 || q >= s.n {
			return &TargetOutOfRangeError{
				Op: g.Name, Line: g.Line, Target: q, NQubits: s.n,
			}
		}
	}
	if g.IsMeasure {
		if len(g.Targets) != 1 {
			return fmt.Errorf("internal error: measurement gate must have exactly one target")
		}
		outcome := s.measure(g.Targets[0])
		s.records = append(s.records, MeasurementRecord{
			Order:   len(s.records),
			Qubit:   g.Targets[0],
			Outcome: outcome,
			Line:    g.Line,
		})
		return nil
	}

	u := g.Mat()
	if u == nil {
		return fmt.Errorf("gate %s has no matrix", g.Name)
	}
	s.applyUnitary(u, g.Targets, g.Controls)
	return nil
}

// applyUnitary 将维度 2^k 的矩阵 u 作用到 targets 列出的 k 个比特上。
// targets 按高位优先解释（targets[0] 为矩阵指标最高位）。
// 仅当所有 controls 比特都处于 |1> 时该门才生效，否则该块保持不变。
func (s *Simulator) applyUnitary(u [][]complex128, targets, controls []int) {
	k := len(targets)
	used := append(append([]int{}, targets...), controls...)
	free := freeMask(used, s.n)

	// freeBits 升序枚举自由比特的位置。
	var freePos []int
	for b := 0; b < s.n; b++ {
		if free&(1<<b) != 0 {
			freePos = append(freePos, b)
		}
	}

	buf := make([]complex128, 1<<k)
	idx := make([]int, 1<<k)

	// 预计算每个矩阵数字位对应的地址增量（与块无关）。
	digitAddr := make([]int, 1<<k)
	for digit := 0; digit < (1 << k); digit++ {
		addr := 0
		for j := 0; j < k; j++ {
			if digit&(1<<(k-1-j)) != 0 {
				addr |= 1 << targets[j]
			}
		}
		digitAddr[digit] = addr
	}

	// 枚举自由比特赋值（共 2^nFree 个活动块）。
	nFree := len(freePos)
	// 控制位在活动块中固定为 |1>。
	var controlMask int
	for _, cq := range controls {
		controlMask |= 1 << cq
	}
	nBlocks := 1 << nFree
	base := controlMask
	for block := 0; block < nBlocks; block++ {
		// 该块内 2^k 个基态下标与对应振幅。
		for digit := 0; digit < (1 << k); digit++ {
			addr := base + digitAddr[digit]
			idx[digit] = addr
			buf[digit] = s.state[addr]
		}
		for row := 0; row < (1 << k); row++ {
			var acc complex128
			ur := u[row]
			for col := 0; col < (1 << k); col++ {
				acc += ur[col] * buf[col]
			}
			s.state[idx[row]] = acc
		}

		// 增量推进自由比特赋值（摊还 O(1)）：
		// 找到最低的需要进位的自由位并翻转。
		if block+1 < nBlocks {
			carry := block + 1
			for j := 0; j < nFree; j++ {
				bit := carry & (1 << j)
				wasSet := base&(1<<freePos[j]) != 0
				if bit != 0 && !wasSet {
					base |= 1 << freePos[j]
					break
				}
				if bit == 0 && wasSet {
					base &^= 1 << freePos[j]
				}
			}
		}
	}
}

func freeMask(qubits []int, n int) uint {
	var used uint
	for _, q := range qubits {
		used |= 1 << q
	}
	var all uint = (1 << n) - 1
	return all ^ used
}

// measure 对比特 q 做计算基测量：按概率坍缩并返回 0/1。
func (s *Simulator) measure(q int) int {
	p1 := 0.0
	mask := 1 << q
	for i, a := range s.state {
		if i&mask != 0 {
			re := real(a)
			im := imag(a)
			p1 += re*re + im*im
		}
	}
	outcome := 0
	if s.randFloat() < p1 {
		outcome = 1
	}
	norm := 0.0
	for i := range s.state {
		bit := (i >> q) & 1
		if bit != outcome {
			s.state[i] = 0
		} else {
			re := real(s.state[i])
			im := imag(s.state[i])
			norm += re*re + im*im
		}
	}
	if norm > 0 {
		scale := complex(1.0/math.Sqrt(norm), 0)
		for i := range s.state {
			s.state[i] *= scale
		}
	}
	return outcome
}

// MeasurementRecord 记录一次测量的结果。
type MeasurementRecord struct {
	Order   int // 线路中测量指令的序号
	Qubit   int // 被测比特
	Outcome int // 0 或 1
	Line    int // 源文件行号
}

// Sample 对线路执行 shots 次独立采样（每次从 |0...0> 重新演化）。
// 返回的 key 为经典比特串：
//   - 线路含 measure 指令时，按测量声明顺序排列被测比特；
//   - 不含 measure 时，对全部 n 个比特测量，比特 0 在字符串最左端。
//
// 仿真器带种子时，shots 序列完全可复现。
func (s *Simulator) Sample(c *Circuit, shots int) (map[string]int, error) {
	if shots < 1 {
		return nil, fmt.Errorf("shots must be >= 1, got %d", shots)
	}
	if c.NQubits != s.n {
		return nil, fmt.Errorf("simulator has %d qubits but circuit declares %d", s.n, c.NQubits)
	}

	measured := measuredQubits(c)
	implicitAll := len(measured) == 0
	if implicitAll {
		measured = make([]int, s.n)
		for q := range measured {
			measured[q] = q
		}
	}

	counts := make(map[string]int)
	for i := 0; i < shots; i++ {
		if err := s.Run(c); err != nil {
			return nil, err
		}
		var bits []int
		if implicitAll {
			bits = s.measureAllForSample()
		} else {
			recs := s.records
			bits = make([]int, len(measured))
			for j := range bits {
				bits[j] = recs[j].Outcome
			}
		}
		var sb strings.Builder
		for _, b := range bits {
			sb.WriteByte(byte('0' + b))
		}
		counts[sb.String()]++
	}
	return counts, nil
}

// measuredQubits 按线路中 measure 指令的声明顺序收集被测比特。
func measuredQubits(c *Circuit) []int {
	var qs []int
	for _, g := range c.Gates {
		if g.IsMeasure {
			qs = append(qs, g.Targets[0])
		}
	}
	return qs
}

// measureAllForSample 在采样末尾对全部比特做坍缩测量，返回比特 0..n-1 的结果。
func (s *Simulator) measureAllForSample() []int {
	bits := make([]int, s.n)
	for q := 0; q < s.n; q++ {
		bits[q] = s.measure(q)
	}
	return bits
}
