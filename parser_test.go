package qsim

import (
	"errors"
	"strings"
	"testing"
)

func TestParseBasic(t *testing.T) {
	src := `
# a comment
qubits 3
h 0
cx 0,1
ccx 0,1,2
rx(45) 2
swap 1,2
m 0,1
`
	c, err := Parse(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.NumQubits() != 3 {
		t.Fatalf("n qubits = %d, want 3", c.NumQubits())
	}
	// m 0,1 展开成两条测量，所以门数 = 3 单/受控/多比特 + rx + swap + 2 measure
	if c.GateCount() != 7 {
		t.Fatalf("gate count = %d, want 7", c.GateCount())
	}
	// 门顺序：h(0) cx(1) ccx(2) rx(3) swap(4) m0(5) m1(6)
	cx := c.Gates[1]
	if cx.Name != "X" || len(cx.Controls) != 1 || cx.Controls[0] != 0 ||
		len(cx.Targets) != 1 || cx.Targets[0] != 1 {
		t.Fatalf("cx parsed wrong: %+v", cx)
	}
	rx := c.Gates[3]
	if rx.Name != "RX" || len(rx.Params) != 1 || rx.Params[0] != 45 {
		t.Fatalf("rx parsed wrong: %+v", rx)
	}
}

func TestParseMeasureAll(t *testing.T) {
	c, err := Parse("qubits 3\nmeasure\n")
	if err != nil {
		t.Fatal(err)
	}
	if c.GateCount() != 3 {
		t.Fatalf("want 3 expanded measures, got %d", c.GateCount())
	}
	for i, g := range c.Gates {
		if !g.IsMeasure || g.Targets[0] != i {
			t.Fatalf("measure-all expansion wrong at %d: %+v", i, g)
		}
	}
}

func TestParseUnrecognizedInstruction(t *testing.T) {
	src := "qubits 2\nh 0\nfoobar 1\n"
	_, err := Parse(src)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Line != 3 {
		t.Fatalf("error line = %d, want 3", pe.Line)
	}
	if pe.Op != "foobar" {
		t.Fatalf("error op = %q, want foobar", pe.Op)
	}
	if !strings.Contains(pe.Error(), "line 3") {
		t.Fatalf("error message must locate line: %q", pe.Error())
	}
}

func TestParseGarbageHasLocation(t *testing.T) {
	cases := []struct {
		src      string
		wantLine int
	}{
		{"qubits 2\nh 0\ncx 0\n", 3}, // wrong arity
		{"qubits 2\nh 0\nx a\n", 3},  // non-int operand
		{"qubits 2\nrx 0\n", 2},      // missing param
		{"qubits 2\nrx(90 0\n", 2},   // unterminated param
		{"qubits 2\nh(1) 0\n", 2},    // param on fixed gate
		{"h 0\nqubits 2\n", 1},       // gate before declaration
		{"qubits 2\nqubits 3\n", 2},  // duplicate declaration
		{"h 0\n", 1},                 // no declaration at all
		{"qubits 0\n", 1},            // non-positive count
		{"qubits 2\nh 2\n", 2},       // out of range
		{"qubits 2\ncx 0,0\n", 2},    // duplicate target
	}
	for i, tc := range cases {
		_, err := Parse(tc.src)
		if err == nil {
			t.Errorf("case %d: expected error for %q, got nil", i, tc.src)
			continue
		}
		// 每种拒绝原因必须能按类型区分。
		var pe *ParseError
		var ce *ConflictError
		var tor *TargetOutOfRangeError
		var mpe *MissingParamError
		var dte *DuplicateTargetError
		typed := errors.As(err, &pe) || errors.As(err, &ce) ||
			errors.As(err, &tor) || errors.As(err, &mpe) || errors.As(err, &dte)
		if !typed {
			t.Errorf("case %d: error is not one of the typed rejections: %T", i, err)
		}
		// 行号可定位。
		switch e := err.(type) {
		case *ParseError:
			if e.Line != tc.wantLine {
				t.Errorf("case %d: ParseError line = %d, want %d", i, e.Line, tc.wantLine)
			}
		case *ConflictError:
			if e.Line != tc.wantLine {
				t.Errorf("case %d: ConflictError line = %d, want %d", i, e.Line, tc.wantLine)
			}
		case *TargetOutOfRangeError:
			if e.Line != tc.wantLine {
				t.Errorf("case %d: TargetOutOfRangeError line = %d", i, e.Line)
			}
		case *MissingParamError:
			if e.Line != tc.wantLine {
				t.Errorf("case %d: MissingParamError line = %d", i, e.Line)
			}
		case *DuplicateTargetError:
			if e.Line != tc.wantLine {
				t.Errorf("case %d: DuplicateTargetError line = %d", i, e.Line)
			}
		}
	}
}

func TestParseRejectDistinct(t *testing.T) {
	_, err := Parse("qubits 2\nh 5\n")
	var tor *TargetOutOfRangeError
	if !errors.As(err, &tor) || tor.Target != 5 || tor.NQubits != 2 {
		t.Fatalf("want TargetOutOfRangeError target=5 n=2, got %v", err)
	}

	_, err = Parse("qubits 2\nrx 0\n")
	var mpe *MissingParamError
	if !errors.As(err, &mpe) {
		t.Fatalf("want MissingParamError, got %v", err)
	}

	_, err = Parse("qubits 2\nqubits 4\nh 0\n")
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("want ConflictError, got %v", err)
	}

	_, err = Parse("qubits 3\nccx 0,0,2\n")
	var dte *DuplicateTargetError
	if !errors.As(err, &dte) || dte.Target != 0 {
		t.Fatalf("want DuplicateTargetError target 0, got %v", err)
	}
}

func TestParseNoHalfState(t *testing.T) {
	// 解析失败必须返回 nil 线路，调用方拿不到半成品。
	c, err := Parse("qubits 2\nh 0\nbadgate 1\nh 1\n")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if c != nil {
		t.Fatalf("circuit must be nil on rejection, got %+v", c)
	}
}

func TestParseColumnLocation(t *testing.T) {
	// 缩进的非法指令，列号必须指向指令开头。
	_, err := Parse("qubits 2\n    nope 0\n")
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want ParseError, got %v", err)
	}
	if pe.Column != 5 {
		t.Fatalf("column = %d, want 5", pe.Column)
	}
}
