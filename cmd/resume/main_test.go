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
	resumeInspect "github.com/jlpdeveloper/resume-builder/internal/inspect"
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

func TestSampleResumeValidateAndInspect(t *testing.T) {
	samplePath := filepath.Join("..", "..", "resumes", "example.yml")
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"resume", "validate", samplePath, "--json"}
	validationOutput := &bytes.Buffer{}
	validationError := &bytes.Buffer{}
	if code := execute(validationOutput, validationError); code != 0 {
		t.Fatalf("sample validation failed: %s", validationError.String())
	}
	var validationResult validate.Result
	if err := json.Unmarshal(validationOutput.Bytes(), &validationResult); err != nil {
		t.Fatalf("decode sample validation: %v", err)
	}
	if !validationResult.Valid {
		t.Fatalf("sample validation returned errors: %+v", validationResult.Errors)
	}

	os.Args = []string{"resume", "inspect", samplePath, "--json"}
	inspectOutput := &bytes.Buffer{}
	inspectError := &bytes.Buffer{}
	if code := execute(inspectOutput, inspectError); code != 0 {
		t.Fatalf("sample inspection failed: %s", inspectError.String())
	}
	var report resumeInspect.Report
	if err := json.Unmarshal(inspectOutput.Bytes(), &report); err != nil {
		t.Fatalf("decode sample inspection: %v", err)
	}
	if !report.Valid || len(report.Sources) != 4 {
		t.Fatalf("sample did not resolve all four source files: %+v", report.Sources)
	}
	wantSections := []string{"summary", "experience", "skills", "impact", "projects", "education", "certifications"}
	if len(report.Sections) != len(wantSections) {
		t.Fatalf("resolved sections = %d, want %d", len(report.Sections), len(wantSections))
	}
	var summary, experience, skills, impact, projects resumeInspect.Section
	for i, section := range report.Sections {
		if section.Name != wantSections[i] {
			t.Errorf("section %d = %q, want %q", i, section.Name, wantSections[i])
		}
		switch section.Name {
		case "summary":
			summary = section
		case "experience":
			experience = section
		case "skills":
			skills = section
		case "impact":
			impact = section
		case "projects":
			projects = section
		}
	}
	if report.PageLimit != 2 {
		t.Errorf("inherited page limit = %d, want 2", report.PageLimit)
	}
	if len(summary.Items) != 1 || summary.Items[0].Source.Kind != "resume" {
		t.Errorf("inline summary was not selected from the resume: %+v", summary.Items)
	}
	if len(impact.Items) != 2 || impact.Items[0].ID != "deployment-reliability" || impact.Items[1].ID != "service-availability" {
		t.Errorf("profile impact selections were not resolved: %+v", impact.Items)
	}
	if len(experience.Items) != 1 || experience.Items[0].ID != "northstar" {
		t.Fatalf("resume should replace base experience defaults with Northstar only: %+v", experience.Items)
	}
	roles := experience.Items[0].Roles
	if len(roles) != 2 || roles[0].ID != "senior-platform" || roles[1].ID != "staff-platform" {
		t.Fatalf("resume role order was not applied: %+v", roles)
	}
	if len(skills.Items) != 2 || skills.Items[0].ID != "platform" || skills.Items[1].ID != "reliability" {
		t.Fatalf("resume skill group selection was not applied: %+v", skills.Items)
	}
	if len(projects.Items) != 1 || projects.Items[0].ID != "delivery-platform" {
		t.Fatalf("resume should replace base project defaults with one project: %+v", projects.Items)
	}

	outputDir := t.TempDir()
	pdfPath := filepath.Join(outputDir, "example.pdf")
	manifestPath := filepath.Join(outputDir, "example.manifest.json")
	os.Args = []string{"resume", "build", samplePath, "--output", pdfPath, "--manifest", manifestPath, "--timestamp", "2024-01-02T03:04:05Z", "--json"}
	buildOutput := &bytes.Buffer{}
	buildError := &bytes.Buffer{}
	if code := execute(buildOutput, buildError); code != 0 {
		t.Fatalf("sample build failed: %s", buildError.String())
	}
	var buildResult resumeBuild.BuildResult
	if err := json.Unmarshal(buildOutput.Bytes(), &buildResult); err != nil {
		t.Fatalf("decode sample build result: %v", err)
	}
	if !buildResult.Success || buildResult.Fingerprints.InputSHA256 == "" || buildResult.Fingerprints.OutputSHA256 == "" {
		t.Fatalf("sample build did not produce fingerprints: %+v", buildResult)
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
