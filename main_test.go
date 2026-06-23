package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func TestPrintHelp(t *testing.T) {
	got := printHelp()
	goldenPath := filepath.Join("testdata", "help.golden")

	if *updateGolden {
		os.MkdirAll("testdata", 0755)
		if err := os.WriteFile(goldenPath, []byte(got), 0644); err != nil {
			t.Fatalf("failed to update golden file: %v", err)
		}
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file: %v", err)
	}

	if got != string(want) {
		t.Errorf("help text mismatch. Run with -update to regenerate")
	}
}

func TestVersionDefault(t *testing.T) {
	if version != "dev" {
		t.Errorf("expected version to be \"dev\", got %q", version)
	}
}

func TestRunCLIVersion(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCLI([]string{"version"}, nil, nil, nil)

	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("runCLI(\"version\") returned error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "aimux") {
		t.Errorf("expected output to contain \"aimux\", got: %q", output)
	}
	if !strings.Contains(output, version) {
		t.Errorf("expected output to contain version %q, got: %q", version, output)
	}
}
