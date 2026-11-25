package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jlpdeveloper/resume-builder/internal/validate"
)

func TestExecute_StubCommands(t *testing.T) {
	stubs := []string{"diff", "build", "dependencies"}

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

	os.Args = []string{"resume", "validate", filepath.Join(t.TempDir(), "missing.yml"), "--json"}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	got := execute(stdout, stderr)
	if got != 1 {
		t.Errorf("expected exit code 1, got %d", got)
	}

	var result validate.Result
	if err := json.NewDecoder(stdout).Decode(&result); err != nil {
		t.Fatalf("expected structured JSON on stdout, got decode error: %v", err)
	}
	if result.Valid || len(result.Errors) == 0 {
		t.Error("expected invalid result with at least one error")
	}
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got %q", stderr.String())
	}
}

func TestExecute_InspectJSON(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yml")
	resumePath := filepath.Join(dir, "resume.yml")
	if err := os.WriteFile(profilePath, []byte("schema: resume/v1\nkind: profile\nid: p1\nheader:\n  name: Alex Example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resumePath, []byte("schema: resume/v1\nkind: resume\nid: r1\nprofile: profile.yml\ntarget:\n  company: Acme\n  role: Engineer\n"), 0600); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"resume", "inspect", resumePath, "--json"}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if code := execute(stdout, stderr); code != 0 {
		t.Fatalf("expected successful inspect, got %d: %s", code, stderr.String())
	}
	var report struct {
		Valid  bool `json:"valid"`
		Target struct {
			Company string `json:"company"`
		} `json:"target"`
	}
	if err := json.NewDecoder(stdout).Decode(&report); err != nil {
		t.Fatalf("decode inspect result: %v", err)
	}
	if !report.Valid || report.Target.Company != "Acme" {
		t.Fatalf("unexpected inspect result: %+v", report)
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
