package validate

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeYAML(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	data := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidate_ValidResumeGraph(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "profile.yml",
		"schema: resume/v1", "kind: profile", "id: p1",
		"header:", "  name: Alex Example",
		"summaries:", "  - id: summary-1", "    text: Experienced engineer.",
	)
	resume := writeYAML(t, dir, "resume.yml",
		"schema: resume/v1", "kind: resume", "id: r1", "profile: profile.yml",
		"target:", "  company: Acme", "  role: Engineer",
		"summary:", "  id: summary-1",
	)
	result := Validate(resume)
	if !result.Valid || len(result.Errors) != 0 {
		t.Fatalf("expected valid resume, got %+v", result)
	}
}

func TestValidate_InvalidYAMLIncludesLocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resume.yml")
	if err := os.WriteFile(path, []byte("schema: resume/v1\nkind: [invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	result := Validate(path)
	if result.Valid || len(result.Errors) != 1 {
		t.Fatalf("expected one syntax error, got %+v", result)
	}
	if result.Errors[0].Path != path || result.Errors[0].Line < 1 {
		t.Fatalf("expected path and line in diagnostic, got %+v", result.Errors[0])
	}
}

func TestValidate_RejectsUnknownField(t *testing.T) {
	path := writeYAML(t, t.TempDir(), "profile.yml",
		"schema: resume/v1", "kind: profile", "id: p1",
		"header:", "  name: Alex Example", "unknownField: true",
	)
	result := Validate(path)
	if result.Valid || len(result.Errors) == 0 {
		t.Fatalf("expected unknown field error, got %+v", result)
	}
	if !strings.Contains(result.Errors[0].Message, "unknownField") || result.Errors[0].Line < 1 {
		t.Errorf("expected field and line in diagnostic, got %+v", result.Errors[0])
	}
}

func TestValidate_UnknownContentReference(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "profile.yml",
		"schema: resume/v1", "kind: profile", "id: p1",
		"header:", "  name: Alex Example",
		"summaries:", "  - id: known", "    text: A summary.",
	)
	resume := writeYAML(t, dir, "resume.yml",
		"schema: resume/v1", "kind: resume", "id: r1", "profile: profile.yml",
		"target:", "  company: Acme", "  role: Engineer",
		"summary:", "  id: missing",
	)
	result := Validate(resume)
	if result.Valid || len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "unknown summary id") {
		t.Fatalf("expected unknown summary diagnostic, got %+v", result)
	}
}

func TestValidate_WarningsAreSeparateFromErrors(t *testing.T) {
	path := writeYAML(t, t.TempDir(), "profile.yml",
		"schema: resume/v1", "kind: profile", "id: p1",
		"header:", "  name: Alex Example",
		"employment:", "  - id: acme", "    name: Acme", "    roles:",
		"      - id: role-1", "        title: Engineer", "        start: '2020'", "        accomplishments: [Same outcome]",
		"      - id: role-2", "        title: Lead Engineer", "        start: '2022'", "        accomplishments: [Same outcome]",
	)
	result := Validate(path)
	if !result.Valid || len(result.Errors) != 0 || len(result.Warnings) == 0 {
		t.Fatalf("expected warning-only result, got %+v", result)
	}
}

func TestValidate_RejectsInvalidDateAndURL(t *testing.T) {
	path := writeYAML(t, t.TempDir(), "profile.yml",
		"schema: resume/v1", "kind: profile", "id: p1",
		"header:", "  name: Alex Example", "  github: github.com/example",
		"employment:", "  - id: acme", "    name: Acme", "    roles:",
		"      - id: role-1", "        title: Engineer", "        start: '2020-13'",
	)
	result := Validate(path)
	if result.Valid || len(result.Errors) != 2 {
		t.Fatalf("expected date and URL errors, got %+v", result)
	}
}

func TestValidate_RejectsInvalidTemplateColor(t *testing.T) {
	path := writeYAML(t, t.TempDir(), "template.yml",
		"schema: resume/v1", "kind: template", "id: standard",
		"page:", "  width: 216", "  height: 279",
		"fonts:", "  regular: ResumeSans", "  bold: ResumeSans",
		"colors:", "  heading: blue",
	)
	result := Validate(path)
	if result.Valid || len(result.Errors) == 0 || !strings.Contains(result.Errors[0].Message, "invalid heading color") {
		t.Fatalf("expected template color error, got %+v", result)
	}
}

func TestRun_JSONOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yml")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := Run([]string{path}, stdout, stderr, true)
	if code == 0 || !strings.Contains(stdout.String(), `"valid":false`) {
		t.Fatalf("expected JSON validation failure, code %d, output %q", code, stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("expected empty stderr, got %q", stderr.String())
	}
}
