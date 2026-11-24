package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	resumeBuild "github.com/jlpdeveloper/resume-builder/internal/build"
	"github.com/jlpdeveloper/resume-builder/internal/validate"
)

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

func TestExecute_DependenciesJSON(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "profile.yml")
	if err := os.WriteFile(profilePath, []byte("schema: resume/v1\nkind: profile\nid: p1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"resume", "dependencies", profilePath, "--json"}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if code := execute(stdout, stderr); code != 0 {
		t.Fatalf("expected successful dependencies command, got %d: %s", code, stderr.String())
	}
	var result struct {
		Nodes []struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"nodes"`
	}
	if err := json.NewDecoder(stdout).Decode(&result); err != nil {
		t.Fatalf("decode dependency graph: %v", err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].ID != "p1" || result.Nodes[0].Kind != "profile" {
		t.Fatalf("unexpected dependency graph: %+v", result)
	}
}

func TestExecute_BuildJSON(t *testing.T) {
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
	os.Args = []string{"resume", "build", resumePath, "--timestamp", "2024-01-02T03:04:05Z", "--json"}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if code := execute(stdout, stderr); code != 0 {
		t.Fatalf("expected successful build, got %d: %s", code, stderr.String())
	}
	var result resumeBuild.BuildResult
	if err := json.NewDecoder(stdout).Decode(&result); err != nil {
		t.Fatalf("decode build result: %v", err)
	}
	if !result.Success || result.PageCount < 1 || result.PDFPath == "" || result.ManifestPath == "" {
		t.Fatalf("unexpected build result: %+v", result)
	}
	manifestData, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest resumeBuild.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	wantTimestamp := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	if !manifest.GeneratedAt.Equal(wantTimestamp) {
		t.Errorf("manifest timestamp = %s, want %s", manifest.GeneratedAt, wantTimestamp)
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
