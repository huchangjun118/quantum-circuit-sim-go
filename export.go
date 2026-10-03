package qsim

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// StepSnapshot 记录执行某条门指令之后的态矢量快照。
type StepSnapshot struct {
	Step      int          `json:"step"`
	Line      int          `json:"line"`
	GateName  string       `json:"gate"`
	Targets   []int        `json:"targets"`
	Controls  []int        `json:"controls,omitempty"`
	Params    []float64    `json:"params,omitempty"`
	IsMeasure bool         `json:"is_measure"`
	Outcome   int          `json:"outcome"` // 非测量步为 -1
	State     []complex128 `json:"-"`       // 文本导出使用；JSON 见 ExportJSON
}

// Snapshot 执行线路并逐步导出态矢量快照（包含初始态 step=0）。
// seed 为 nil 时使用全局随机源；非 nil 时测量序列可复现。
func Snapshot(c *Circuit, seed *int64) ([]StepSnapshot, error) {
	var sim *Simulator
	if seed == nil {
		sim = NewSimulator(c.NQubits)
	} else {
		sim = NewSeededSimulator(c.NQubits, *seed)
	}

	snaps := []StepSnapshot{{
		Step: 0, Line: 0, GateName: "INIT",
		Targets: nil, Outcome: -1, State: sim.StateVector(),
	}}

	for i, g := range c.Gates {
		if err := sim.ApplyGate(g); err != nil {
			return nil, err
		}
		snap := StepSnapshot{
			Step:      i + 1,
			Line:      g.Line,
			GateName:  g.Name,
			Targets:   append([]int{}, g.Targets...),
			Controls:  append([]int{}, g.Controls...),
			Params:    append([]float64{}, g.Params...),
			IsMeasure: g.IsMeasure,
			Outcome:   -1,
			State:     sim.StateVector(),
		}
		if g.IsMeasure {
			snap.GateName = "MEASURE"
			recs := sim.Measurements()
			snap.Outcome = recs[len(recs)-1].Outcome
		}
		snaps = append(snaps, snap)
	}
	return snaps, nil
}

// basisLabel 将基态下标格式化为 |q0q1...> 形式（比特 0 在最左）。
func basisLabel(idx, n int) string {
	b := make([]byte, n)
	for q := 0; q < n; q++ {
		if idx&(1<<q) != 0 {
			b[q] = '1'
		} else {
			b[q] = '0'
		}
	}
	return string(b)
}

func formatComplex(z complex128) string {
	re, im := real(z), imag(z)
	if absFloat(im) < 1e-12 {
		return strconv.FormatFloat(re, 'g', 6, 64)
	}
	if absFloat(re) < 1e-12 {
		return strconv.FormatFloat(im, 'g', 6, 64) + "i"
	}
	sign := "+"
	if im < 0 {
		sign = "-"
	}
	return fmt.Sprintf("%s%s%si",
		strconv.FormatFloat(re, 'g', 6, 64), sign,
		strconv.FormatFloat(absFloat(im), 'g', 6, 64))
}

func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func gateDescription(s StepSnapshot) string {
	var sb strings.Builder
	sb.WriteString(s.GateName)
	if len(s.Controls) > 0 {
		sb.WriteString(" controls=[")
		sb.WriteString(joinInts(s.Controls))
		sb.WriteString("]")
	}
	sb.WriteString(" targets=[")
	sb.WriteString(joinInts(s.Targets))
	sb.WriteString("]")
	if len(s.Params) > 0 {
		ps := make([]string, len(s.Params))
		for i, p := range s.Params {
			ps[i] = strconv.FormatFloat(p, 'g', -1, 64)
		}
		sb.WriteString(" params=[")
		sb.WriteString(strings.Join(ps, ","))
		sb.WriteString("]")
	}
	return sb.String()
}

func joinInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.Itoa(x)
	}
	return strings.Join(parts, ",")
}

// ExportText 将快照以易读文本写入 w，体现门的先后顺序、作用目标与每阶段态矢量。
func ExportText(c *Circuit, snapshots []StepSnapshot, w io.Writer) error {
	n := c.NQubits
	fmt.Fprintf(w, "# circuit: %d qubit(s), %d instruction(s)\n", n, c.GateCount())
	for _, s := range snapshots {
		if s.Step == 0 {
			fmt.Fprintln(w, "\n[step 0] initial state |0...0>")
		} else {
			fmt.Fprintf(w, "\n[step %d] line %d: %s", s.Step, s.Line, gateDescription(s))
			if s.IsMeasure {
				fmt.Fprintf(w, " -> outcome %d (state collapsed)", s.Outcome)
			}
			fmt.Fprintln(w)
		}
		for i, amp := range s.State {
			if isZero(amp) {
				continue
			}
			fmt.Fprintf(w, "  |%s> = %s\n", basisLabel(i, n), formatComplex(amp))
		}
	}
	return nil
}

func isZero(z complex128) bool {
	return absFloat(real(z)) < 1e-12 && absFloat(imag(z)) < 1e-12
}

// ampJSON 是复数振幅的 JSON 表示。
type ampJSON struct {
	Basis string  `json:"basis"`
	Re    float64 `json:"re"`
	Im    float64 `json:"im"`
}

type stepJSON struct {
	Step      int       `json:"step"`
	Line      int       `json:"line"`
	Gate      string    `json:"gate"`
	Targets   []int     `json:"targets"`
	Controls  []int     `json:"controls,omitempty"`
	Params    []float64 `json:"params,omitempty"`
	IsMeasure bool      `json:"is_measure"`
	Outcome   int       `json:"outcome"`
	Amps      []ampJSON `json:"amplitudes"`
}

type exportDoc struct {
	NQubits   int        `json:"n_qubits"`
	Gates     int        `json:"gate_count"`
	Snapshots []stepJSON `json:"snapshots"`
}

// ExportJSON 将快照导出为 JSON 写入 w（仅导出非零振幅）。
func ExportJSON(c *Circuit, snapshots []StepSnapshot, w io.Writer) error {
	n := c.NQubits
	doc := exportDoc{NQubits: n, Gates: c.GateCount()}
	for _, s := range snapshots {
		sj := stepJSON{
			Step: s.Step, Line: s.Line, Gate: s.GateName,
			Targets:   append([]int{}, s.Targets...),
			Controls:  append([]int{}, s.Controls...),
			Params:    append([]float64{}, s.Params...),
			IsMeasure: s.IsMeasure, Outcome: s.Outcome,
		}
		for i, amp := range s.State {
			if isZero(amp) {
				continue
			}
			sj.Amps = append(sj.Amps, ampJSON{
				Basis: basisLabel(i, n), Re: real(amp), Im: imag(amp),
			})
		}
		doc.Snapshots = append(doc.Snapshots, sj)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
