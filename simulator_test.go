package qsim

import (
	"math"
	"testing"
	"time"
)

const eps = 1e-9

func capprox(a, b complex128) bool {
	d := a - b
	return math.Abs(real(d)) < eps && math.Abs(imag(d)) < eps
}

func stateApprox(a, b []complex128) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !capprox(a[i], b[i]) {
			return false
		}
	}
	return true
}

func TestDeterminismRepeatRun(t *testing.T) {
	src := "qubits 4\nh 0\ncx 0,1\nt 2\nrx(33.3) 3\nccx 0,1,2\nswap 2,3\nsdg 1\n"
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSeededSimulator(4, 7)
	if err := s.Run(c); err != nil {
		t.Fatal(err)
	}
	first := s.StateVector()

	// 同一份输入重复仿真，态矢量逐位一致。
	for rep := 0; rep < 3; rep++ {
		if err := s.Run(c); err != nil {
			t.Fatal(err)
		}
		again := s.StateVector()
		if !stateApprox(first, again) {
			t.Fatalf("run %d differs from first run", rep+1)
		}
	}

	// 两个独立但同种子的仿真器也一致。
	s2 := NewSeededSimulator(4, 7)
	if err := s2.Run(c); err != nil {
		t.Fatal(err)
	}
	if !stateApprox(first, s2.StateVector()) {
		t.Fatal("independent simulators with same seed differ")
	}
}

func TestKnownStates(t *testing.T) {
	inv := complex(1/math.Sqrt2, 0)

	// H|0> on qubit 1 -> (|00> + |01>)/sqrt2，比特 0 在标签最左。
	c, _ := Parse("qubits 2\nh 1\n")
	s := NewSimulator(2)
	if err := s.Run(c); err != nil {
		t.Fatal(err)
	}
	st := s.StateVector()
	// 下标 0=00, 1=10(q0=1), 2=01(q1=1), 3=11
	if !capprox(st[0], inv) || !capprox(st[2], inv) || st[1] != 0 || st[3] != 0 {
		t.Fatalf("H on q1 wrong: %v", st)
	}

	// RZ(180) = Z（只差全局相位约定），P(180) = Z。
	c2, _ := Parse("qubits 1\np(180) 0\n")
	s2 := NewSimulator(1)
	_ = s2.Run(c2)
	// |0> 不被 Z 改变。
	if !capprox(s2.StateVector()[0], 1) {
		t.Fatalf("P(180)|0> wrong: %v", s2.StateVector())
	}

	// X 后 Z：X|0>=|1>, Z|1>=-|1>。
	c3, _ := Parse("qubits 1\nx 0\nz 0\n")
	s3 := NewSimulator(1)
	_ = s3.Run(c3)
	if !capprox(s3.StateVector()[1], -1) {
		t.Fatalf("ZX|0> wrong: %v", s3.StateVector())
	}

	// RY(90)|0> = (|0>+|1>)/sqrt2。
	c4, _ := Parse("qubits 1\nry(90) 0\n")
	s4 := NewSimulator(1)
	_ = s4.Run(c4)
	if !capprox(s4.StateVector()[0], inv) || !capprox(s4.StateVector()[1], inv) {
		t.Fatalf("RY(90)|0> wrong: %v", s4.StateVector())
	}
}

func TestNormPreserved(t *testing.T) {
	src := "qubits 3\nh 0\nh 1\nrx(70) 2\ncx 0,2\nccx 0,1,2\nt 0\n"
	c, _ := Parse(src)
	s := NewSimulator(3)
	if err := s.Run(c); err != nil {
		t.Fatal(err)
	}
	norm := 0.0
	for _, p := range s.Probs() {
		norm += p
	}
	if math.Abs(norm-1) > 1e-9 {
		t.Fatalf("norm = %v, want 1", norm)
	}
}

func TestMeasurementCollapse(t *testing.T) {
	c, _ := Parse("qubits 2\nh 0\ncx 0,1\nm 0\nm 1\n")
	s := NewSeededSimulator(2, 123)
	if err := s.Run(c); err != nil {
		t.Fatal(err)
	}
	st := s.StateVector()
	recs := s.Measurements()
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	if recs[0].Outcome != recs[1].Outcome {
		t.Fatalf("Bell measurement outcomes must agree, got %+v", recs)
	}
	out := recs[0].Outcome
	// 坍缩后态矢量必须与采样结果严格对应：只有 |00> 或 |11> 为 1。
	if out == 0 {
		if !capprox(st[0], 1) {
			t.Fatalf("outcome 0 but state[00]=%v, full=%v", st[0], st)
		}
		for i := 1; i < 4; i++ {
			if st[i] != 0 {
				t.Fatalf("state not collapsed: %v", st)
			}
		}
	} else {
		if !capprox(st[3], 1) {
			t.Fatalf("outcome 1 but state[11]=%v, full=%v", st[3], st)
		}
		for i := 0; i < 3; i++ {
			if st[i] != 0 {
				t.Fatalf("state not collapsed: %v", st)
			}
		}
	}
}

func TestMeasurementOrderCorrespondence(t *testing.T) {
	// 逐个测量：坍缩到 0 的比特后续概率必须为 0。
	c, _ := Parse("qubits 2\nh 0\ncx 0,1\nm 0\nm 1\n")
	for seed := int64(0); seed < 20; seed++ {
		s := NewSeededSimulator(2, seed)
		if err := s.Run(c); err != nil {
			t.Fatal(err)
		}
		recs := s.Measurements()
		for _, r := range recs {
			for i, p := range s.Probs() {
				bit := (i >> r.Qubit) & 1
				if bit != r.Outcome && p > 1e-12 {
					t.Fatalf("seed %d: qubit %d measured %d but p(%d)=%v",
						seed, r.Qubit, r.Outcome, i, p)
				}
			}
		}
	}
}

func TestSampleReproducible(t *testing.T) {
	c, _ := Parse("qubits 2\nh 0\ncx 0,1\n")
	run := func(seed int64) map[string]int {
		s := NewSeededSimulator(2, seed)
		counts, err := s.Sample(c, 500)
		if err != nil {
			t.Fatal(err)
		}
		return counts
	}
	a, b := run(99), run(99)
	if len(a) != len(b) {
		t.Fatal("same-seed sample maps differ in keys")
	}
	for k, v := range a {
		if b[k] != v {
			t.Fatalf("same-seed sample mismatch on %s: %d vs %d", k, v, b[k])
		}
	}
	// Bell 态只能采到 00 与 11。
	total := 0
	for k, v := range a {
		if k != "00" && k != "11" {
			t.Fatalf("impossible sample %q", k)
		}
		total += v
	}
	if total != 500 {
		t.Fatalf("counts sum = %d, want 500", total)
	}
}

func TestMeasureAllImplicit(t *testing.T) {
	// 无 measure 指令时，采样对全部比特测量，key 长度必须为 n。
	c, _ := Parse("qubits 3\nh 0\nh 1\nh 2\n")
	s := NewSeededSimulator(3, 5)
	counts, err := s.Sample(c, 100)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for k, v := range counts {
		if len(k) != 3 {
			t.Fatalf("key %q length != 3", k)
		}
		total += v
	}
	if total != 100 {
		t.Fatalf("total = %d", total)
	}
}

func TestExplicitMeasureSampleOrder(t *testing.T) {
	// m 1 然后 m 0：结果串第 0 位是 q1，第 1 位是 q0。
	c, _ := Parse("qubits 2\nh 0\nm 1\nm 0\n")
	s := NewSeededSimulator(2, 1)
	counts, err := s.Sample(c, 50)
	if err != nil {
		t.Fatal(err)
	}
	for k := range counts {
		if len(k) != 2 {
			t.Fatalf("key length %d", len(k))
		}
		// q1 未加门，始终为 0，所以第一位恒为 '0'。
		if k[0] != '0' {
			t.Fatalf("unmeasured q1 should be 0, key=%s", k)
		}
	}
}

func TestLargeScale(t *testing.T) {
	// 16 比特、浅层线路必须快速完成且概率归一。
	const n = 16
	var sb []byte
	sb = append(sb, []byte("qubits 16\n")...)
	for q := 0; q < n; q++ {
		sb = append(sb, []byte("h "+itoa(q)+"\n")...)
	}
	for q := 0; q+1 < n; q++ {
		sb = append(sb, []byte("cx "+itoa(q)+","+itoa(q+1)+"\n")...)
	}
	c, err := Parse(string(sb))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSimulator(n)
	if err := s.Run(c); err != nil {
		t.Fatal(err)
	}
	if len(s.StateVector()) != 1<<n {
		t.Fatalf("state size = %d", len(s.StateVector()))
	}
	norm := 0.0
	for _, p := range s.Probs() {
		norm += p
	}
	if math.Abs(norm-1) > 1e-6 {
		t.Fatalf("norm = %v", norm)
	}

	// 16 比特全均匀叠加 + 末端采样也必须完成且 shots 总数正确。
	c2, _ := Parse("qubits 16\n" + func() string {
		var x []byte
		for q := 0; q < n; q++ {
			x = append(x, []byte("h "+itoa(q)+"\n")...)
		}
		return string(x)
	}())
	s2 := NewSeededSimulator(n, 42)
	counts, err := s2.Sample(c2, 64)
	if err != nil {
		t.Fatal(err)
	}
	sum := 0
	for _, v := range counts {
		sum += v
	}
	if sum != 64 {
		t.Fatalf("sample sum = %d", sum)
	}
}

func TestLargeScaleBoundedTime(t *testing.T) {
	// 18 比特、35 扇门（约 26 万个振幅），单次演化必须在 2 秒内完成，
	// 以保证比特数增大时性能不会出现不可控退化。
	const n = 18
	var src []byte
	src = append(src, []byte("qubits 18\n")...)
	for q := 0; q < n; q++ {
		src = append(src, []byte("h "+itoa(q)+"\n")...)
	}
	for q := 0; q+1 < n; q++ {
		src = append(src, []byte("cx "+itoa(q)+","+itoa(q+1)+"\n")...)
	}
	c, err := Parse(string(src))
	if err != nil {
		t.Fatal(err)
	}
	s := NewSimulator(n)
	start := time.Now()
	if err := s.Run(c); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("18-qubit run took %v, exceeds 2s budget", d)
	}
	norm := 0.0
	for _, p := range s.Probs() {
		norm += p
	}
	if math.Abs(norm-1) > 1e-6 {
		t.Fatalf("norm = %v", norm)
	}
}

func itoa(x int) string {
	if x == 0 {
		return "0"
	}
	var b []byte
	for x > 0 {
		b = append([]byte{byte('0' + x%10)}, b...)
		x /= 10
	}
	return string(b)
}

func TestSampleRejectsBadShots(t *testing.T) {
	c, _ := Parse("qubits 1\nh 0\n")
	s := NewSimulator(1)
	if _, err := s.Sample(c, 0); err == nil {
		t.Fatal("expected error for shots=0")
	}
}

func TestQubitMismatchRejected(t *testing.T) {
	c, _ := Parse("qubits 3\nh 0\n")
	s := NewSimulator(2)
	if err := s.Run(c); err == nil {
		t.Fatal("expected qubit-count mismatch error")
	}
}
