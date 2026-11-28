package inspect

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeInspectYAML(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func makeInspectFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeInspectYAML(t, dir, "profile.yml",
		"schema: resume/v1", "kind: profile", "id: p1",
		"header:", "  name: Alex Example", "  email: alex@example.com", "  phone: 555-0100",
		"summaries:", "  - id: professional", "    text: Experienced engineer.",
		"impact:", "  - id: impact-1", "    text: Improved service reliability.",
		"skills:", "  - id: tech", "    name: Programming", "    skills: [Go, SQL]",
		"employment:", "  - id: acme", "    name: Acme", "    roles:",
		"      - id: role-1", "        title: Engineer", "        start: '2020-01'",
		"        scope: Led engineering delivery and production ownership",
		"        accomplishments: [Improved uptime]",
		"projects:", "  - id: project-1", "    name: Resume API", "    role: Maintainer", "    start: '2022'",
		"    link: https://example.com", "    accomplishments: [Built a service]",
		"education:", "  - id: education-1", "    institution: Example University", "    degree: BS",
		"certifications:", "  - id: cert-1", "    name: Go Developer", "    issuer: Example Org", "    dateEarned: '2023-01'",
	)
	return writeInspectYAML(t, dir, "resume.yml",
		"schema: resume/v1", "kind: resume", "id: r1", "profile: profile.yml",
		"target:", "  company: Acme", "  role: Platform Engineer", "  url: https://acme.example", "  notes: Sample target",
		"summary:", "  id: professional",
		"impact:", "  - id: impact-1",
		"skills:", "  - group: tech", "    skills: [Go]",
		"experience:", "  - company: acme", "    roles: [role-1]",
		"projects:", "  - id: project-1", "education:", "  - id: education-1",
		"certifications:", "  - id: cert-1", "metadata:", "  title: Alex Example — Platform Engineer",
		"sections: [skills, summary, impact, experience, projects, education, certifications]",
	)
}

func TestInspect_OrderedSectionsAndSources(t *testing.T) {
	resumePath := makeInspectFixture(t)
	report, err := Inspect(resumePath)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid || report.Header.Name != "Alex Example" {
		t.Fatalf("unexpected report header: %+v", report)
	}
	if len(report.Sections) != 7 || report.Sections[0].Name != "skills" || report.Sections[1].Name != "summary" {
		t.Fatalf("section order was not preserved: %+v", report.Sections)
	}
	if len(report.Sections[0].Items) != 1 {
		t.Fatalf("expected one skill group, got %+v", report.Sections[0].Items)
	}
	if report.Sections[1].Items[0].ID != "professional" {
		t.Errorf("expected resolved summary ID, got %+v", report.Sections[1].Items[0])
	}
	skill := report.Sections[0].Items[0]
	if skill.Source.Kind != "profile" || skill.Source.File == "" || skill.Source.SelectedBy != "resume" || !skill.Source.Inherited {
		t.Errorf("unexpected skill source: %+v", skill.Source)
	}
	project := report.Sections[4].Items[0]
	if project.Source.Kind != "profile" || project.Source.SelectedBy != "resume" || !project.Source.Inherited {
		t.Errorf("unexpected project source: %+v", project.Source)
	}
	experience := report.Sections[3].Items[0]
	if len(experience.Roles) != 1 || experience.Roles[0].Source.File == "" || experience.Roles[0].Scope != "Led engineering delivery and production ownership" {
		t.Errorf("expected role source attribution, got %+v", experience)
	}
}

func TestRun_HumanAndJSONOutput(t *testing.T) {
	resumePath := makeInspectFixture(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if code := Run([]string{resumePath}, stdout, stderr, false); code != 0 {
		t.Fatalf("expected human inspect success, got %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1. Skills") || !strings.Contains(stdout.String(), "inherited") || !strings.Contains(stdout.String(), "profile.yml") || !strings.Contains(stdout.String(), "Phone: 555-0100") || !strings.Contains(stdout.String(), "date_earned: 2023-01") || !strings.Contains(stdout.String(), "Led engineering delivery and production ownership") {
		t.Errorf("human output omitted order or provenance: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run([]string{resumePath}, stdout, stderr, true); code != 0 {
		t.Fatalf("expected JSON inspect success, got %d: %s", code, stderr.String())
	}
	var report Report
	if err := json.NewDecoder(stdout).Decode(&report); err != nil {
		t.Fatalf("decode inspect JSON: %v", err)
	}
	if report.Target.Company != "Acme" || report.Header.Name != "Alex Example" {
		t.Fatalf("unexpected JSON report: %+v", report)
	}
	if report.Sections[3].Items[0].Roles[0].Scope != "Led engineering delivery and production ownership" {
		t.Errorf("JSON report omitted role scope: %+v", report.Sections[3].Items[0].Roles[0])
	}
}

func TestRun_InvalidResumeReturnsJSONDiagnostics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resume.yml")
	stdout := &bytes.Buffer{}
	if code := Run([]string{path}, stdout, &bytes.Buffer{}, true); code == 0 {
		t.Fatal("expected invalid resume to fail")
	}
	var result map[string]any
	if err := json.NewDecoder(stdout).Decode(&result); err != nil {
		t.Fatalf("decode validation diagnostics: %v", err)
	}
	if result["valid"] != false {
		t.Fatalf("expected invalid result, got %v", result["valid"])
	}
}
