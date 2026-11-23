package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestRealMain_StubCommands(t *testing.T) {
	stubs := []string{"validate", "inspect", "diff", "build", "schema", "dependencies"}

	for _, cmd := range stubs {
		t.Run(cmd, func(t *testing.T) {
			oldArgs := os.Args
			oldStderr := os.Stderr
			defer func() {
				os.Args = oldArgs
				os.Stderr = oldStderr
			}()

			os.Args = []string{"resume", cmd}
			b := &bytes.Buffer{}
			os.Stderr = b

			got := realMain()
			if got != 1 {
				t.Errorf("expected exit code 1, got %d", got)
			}
			if !strings.Contains(b.String(), "not yet implemented") {
				t.Errorf("expected 'not yet implemented' in stderr, got: %s", b.String())
			}
		})
	}
}

func TestRealMain_Version(t *testing.T) {
	oldArgs := os.Args
	oldStdout := os.Stdout
	defer func() {
		os.Args = oldArgs
		os.Stdout = oldStdout
	}()

	os.Args = []string{"resume", "version"}
	b := &bytes.Buffer{}
	os.Stdout = b

	got := realMain()
	if got != 0 {
		t.Errorf("expected exit code 0, got %d", got)
	}
	if !strings.Contains(b.String(), version) {
		t.Errorf("expected version %q in stdout, got: %s", version, b.String())
	}
}

func TestRealMain_UnknownCommand(t *testing.T) {
	oldArgs := os.Args
	oldStderr := os.Stderr
	defer func() {
		os.Args = oldArgs
		os.Stderr = oldStderr
	}()

	os.Args = []string{"resume", "unknown"}
	b := &bytes.Buffer{}
	os.Stderr = b

	got := realMain()
	if got != 1 {
		t.Errorf("expected exit code 1, got %d", got)
	}
	if !strings.Contains(b.String(), "unknown command") {
		t.Errorf("expected 'unknown command' in stderr, got: %s", b.String())
	}
}

func TestRealMain_NoArgs(t *testing.T) {
	oldArgs := os.Args
	oldStdout := os.Stdout
	defer func() {
		os.Args = oldArgs
		os.Stdout = oldStdout
	}()

	os.Args = []string{"resume"}
	b := &bytes.Buffer{}
	os.Stdout = b

	got := realMain()
	if got != 1 {
		t.Errorf("expected exit code 1, got %d", got)
	}
	if !strings.Contains(b.String(), "Usage") {
		t.Errorf("expected usage in stdout, got: %s", b.String())
	}
}

func TestRealMain_JsonFlag(t *testing.T) {
	oldArgs := os.Args
	oldStderr := os.Stderr
	defer func() {
		os.Args = oldArgs
		os.Stderr = oldStderr
	}()

	os.Args = []string{"resume", "validate", "--json"}
	b := &bytes.Buffer{}
	os.Stderr = b

	got := realMain()
	if got != 1 {
		t.Errorf("expected exit code 1, got %d", got)
	}

	var result map[string]string
	if err := json.NewDecoder(b).Decode(&result); err != nil {
		t.Fatalf("expected valid JSON on stderr, got decode error: %v", err)
	}
	if _, ok := result["error"]; !ok {
		t.Error("expected 'error' key in JSON output")
	}
}

func TestRealMain_Help(t *testing.T) {
	oldArgs := os.Args
	oldStdout := os.Stdout
	defer func() {
		os.Args = oldArgs
		os.Stdout = oldStdout
	}()

	os.Args = []string{"resume", "help"}
	b := &bytes.Buffer{}
	os.Stdout = b

	got := realMain()
	if got != 0 {
		t.Errorf("expected exit code 0, got %d", got)
	}
	if !strings.Contains(b.String(), "Usage") {
		t.Errorf("expected usage in stdout, got: %s", b.String())
	}
}
