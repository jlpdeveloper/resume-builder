package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestExecute_StubCommands(t *testing.T) {
	stubs := []string{"validate", "inspect", "diff", "build", "schema", "dependencies"}

	for _, cmd := range stubs {
		t.Run(cmd, func(t *testing.T) {
			oldArgs := os.Args
			defer func() { os.Args = oldArgs }()

			os.Args = []string{"resume", cmd}
			stderr := &bytes.Buffer{}

			got := execute(nil, stderr)
			if got != 1 {
				t.Errorf("expected exit code 1, got %d", got)
			}
			if !strings.Contains(stderr.String(), "not yet implemented") {
				t.Errorf("expected 'not yet implemented' in stderr, got: %s", stderr.String())
			}
		})
	}
}

func TestExecute_Version(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"resume", "version"}
	stdout := &bytes.Buffer{}

	got := execute(stdout, nil)
	if got != 0 {
		t.Errorf("expected exit code 0, got %d", got)
	}
	if !strings.Contains(stdout.String(), version) {
		t.Errorf("expected version %q in stdout, got: %s", version, stdout.String())
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"resume", "unknown"}
	stderr := &bytes.Buffer{}

	got := execute(nil, stderr)
	if got != 1 {
		t.Errorf("expected exit code 1, got %d", got)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("expected 'unknown command' in stderr, got: %s", stderr.String())
	}
}

func TestExecute_NoArgs(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"resume"}
	stdout := &bytes.Buffer{}

	got := execute(stdout, nil)
	if got != 1 {
		t.Errorf("expected exit code 1, got %d", got)
	}
	if !strings.Contains(stdout.String(), "Usage") {
		t.Errorf("expected usage in stdout, got: %s", stdout.String())
	}
}

func TestExecute_JsonFlag(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"resume", "validate", "--json"}
	stderr := &bytes.Buffer{}

	got := execute(nil, stderr)
	if got != 1 {
		t.Errorf("expected exit code 1, got %d", got)
	}

	var result map[string]string
	if err := json.NewDecoder(stderr).Decode(&result); err != nil {
		t.Fatalf("expected valid JSON on stderr, got decode error: %v", err)
	}
	if _, ok := result["error"]; !ok {
		t.Error("expected 'error' key in JSON output")
	}
}

func TestExecute_Help(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"resume", "help"}
	stdout := &bytes.Buffer{}

	got := execute(stdout, nil)
	if got != 0 {
		t.Errorf("expected exit code 0, got %d", got)
	}
	if !strings.Contains(stdout.String(), "Usage") {
		t.Errorf("expected usage in stdout, got: %s", stdout.String())
	}
}
