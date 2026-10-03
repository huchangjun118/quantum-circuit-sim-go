package qsim

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotOrderAndTargets(t *testing.T) {
	src := "qubits 2\nh 0\ncx 0,1\nm 0\n"
	c, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	seed := int64(7)
	snaps, err := Snapshot(c, &seed)
	if err != nil {
		t.Fatal(err)
	}
	// INIT + 3 条门（m 0 只有一条）。
	if len(snaps) != 4 {
		t.Fatalf("want 4 snapshots, got %d", len(snaps))
	}
	if snaps[0].GateName != "INIT" {
		t.Fatalf("snap0 = %v", snaps[0])
	}
	if snaps[1].GateName != "H" || snaps[1].Targets[0] != 0 || snaps[1].Step != 1 {
		t.Fatalf("snap1 wrong: %+v", snaps[1])
	}
	cx := snaps[2]
	if cx.GateName != "X" || len(cx.Controls) != 1 || cx.Controls[0] != 0 || cx.Targets[0] != 1 {
		t.Fatalf("cx snapshot wrong: %+v", cx)
	}
	ms := snaps[3]
	if !ms.IsMeasure || ms.Targets[0] != 0 || (ms.Outcome != 0 && ms.Outcome != 1) {
		t.Fatalf("measure snapshot wrong: %+v", ms)
	}
	// 测量后态矢量必须与记录的 outcome 严格对应。
	if ms.Outcome == 0 && ms.State[0] != 1 {
		t.Fatalf("outcome 0 but state[0]=%v", ms.State[0])
	}
	if ms.Outcome == 1 && ms.State[3] != 1 {
		t.Fatalf("outcome 1 but state for |11>=%v", ms.State[3])
	}
}

func TestSnapshotReproducible(t *testing.T) {
	src := "qubits 3\nh 0\ncx 0,1\nh 2\nm 0,1,2\n"
	c, _ := Parse(src)
	seed := int64(1234)
	a, err := Snapshot(c, &seed)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Snapshot(c, &seed)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a {
		if a[i].Outcome != b[i].Outcome {
			t.Fatalf("snapshot step %d outcome differs: %d vs %d", i, a[i].Outcome, b[i].Outcome)
		}
		if !stateApprox(a[i].State, b[i].State) {
			t.Fatalf("snapshot step %d state differs", i)
		}
	}
}

func TestExportTextContainsSequence(t *testing.T) {
	src := "qubits 2\nh 0\ncx 0,1\n"
	c, _ := Parse(src)
	snaps, _ := Snapshot(c, nil)
	var buf bytes.Buffer
	if err := ExportText(c, snaps, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"step 0", "step 1", "step 2", "H targets=[0]",
		"X controls=[0] targets=[1]", "|00>", "|11>"} {
		if !strings.Contains(out, want) {
			t.Errorf("export text missing %q", want)
		}
	}
}

func TestExportJSONValid(t *testing.T) {
	src := "qubits 2\nh 0\nrx(90) 1\nm 0\n"
	c, _ := Parse(src)
	seed := int64(3)
	snaps, _ := Snapshot(c, &seed)
	var buf bytes.Buffer
	if err := ExportJSON(c, snaps, &buf); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("exported JSON invalid: %v\n%s", err, buf.String())
	}
	if doc["n_qubits"].(float64) != 2 {
		t.Fatalf("n_qubits wrong: %v", doc["n_qubits"])
	}
	snapList := doc["snapshots"].([]any)
	if len(snapList) != 4 {
		t.Fatalf("want 4 snapshot entries, got %d", len(snapList))
	}
}

func TestDrawContainsTargets(t *testing.T) {
	src := "qubits 3\nh 0\ncx 0,1\nccx 0,1,2\nswap 0,2\nm 1\n"
	c, _ := Parse(src)
	out, err := RenderASCII(c, DefaultDrawConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"q0:", "q1:", "q2:", "[H]", "[X]", "[M]"} {
		if !strings.Contains(out, want) {
			t.Errorf("diagram missing %q\n%s", want, out)
		}
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 { // 1 行步骤号 + 3 根线
		t.Fatalf("diagram line count = %d, want 4", len(lines))
	}
}

func TestDrawEmptyCircuit(t *testing.T) {
	c, _ := Parse("qubits 2\n# nothing but a declaration\n")
	out, err := RenderASCII(c, DefaultDrawConfig())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "q0: --") || !strings.Contains(out, "q1: --") {
		t.Fatalf("empty circuit diagram wrong:\n%s", out)
	}
}
