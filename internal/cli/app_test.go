package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/huchangjun118/quantum-circuit-sim-go/internal/circuit"
	"github.com/huchangjun118/quantum-circuit-sim-go/internal/simulator"
)

func TestSplitArgsAllowsFileAfterFlags(t *testing.T) {
	flagArgs, positionalArgs := splitArgs([]string{"--seed", "7", "bell.qc", "--shots", "3", "--stages", "stages.json"})
	if len(positionalArgs) != 1 || positionalArgs[0] != "bell.qc" {
		t.Fatalf("unexpected positional args: %v", positionalArgs)
	}
	if len(flagArgs) != 6 {
		t.Fatalf("unexpected flag args: %v", flagArgs)
	}
}

func TestWriteStageFileContainsGateOrderAndTargets(t *testing.T) {
	c, err := circuit.Parse(strings.NewReader("qubits 2\nq0: H\nq0 q1: CX\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	result, err := simulator.Run(c, simulator.RunOptions{CaptureStages: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	path := filepath.Join(t.TempDir(), "stages.json")
	if err := writeStageFile(path, c, result); err != nil {
		t.Fatalf("writeStageFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	text := string(data)
	for _, want := range []string{`"gate": "H"`, `"gate": "CX"`, `"controls": [`, `"targets": [`} {
		if !strings.Contains(text, want) {
			t.Fatalf("stage file missing %q:\n%s", want, text)
		}
	}
}

func TestRunPrintsSamplesAndState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bell.qc")
	if err := os.WriteFile(path, []byte("qubits 2\ncbits 2\nq0: H\nq0 q1: CX\nq0 -> c0\nq1 -> c1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	oldStdout := os.Stdout
	t.Cleanup(func() { os.Stdout = oldStdout })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	if err := Run([]string{"--file", path, "--shots", "5", "--seed", "1", "--diagram=false", "--state=false"}); err != nil {
		t.Fatalf("Run samples: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(data), "measurement samples") || !strings.Contains(string(data), "counts:") {
		t.Fatalf("sample output missing:\n%s", data)
	}

	r, w, err = os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	if err := Run([]string{"--file", path, "--diagram=false"}); err != nil {
		t.Fatalf("Run state: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data, err = io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(data), "final state:") || !strings.Contains(string(data), "classical bits:") {
		t.Fatalf("state output missing:\n%s", data)
	}
}
