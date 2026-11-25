package diff

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeDiffYAML(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func diffFixture(t *testing.T) (string, func(string, string, string, int, string, string, []string) string) {
	t.Helper()
	dir := t.TempDir()
	writeDiffYAML(t, dir, "profile.yml",
		"schema: resume/v1", "kind: profile", "id: p1",
		"header:", "  name: Alex Example",
		"impact:", "  - id: impact-a", "    text: Improved reliability.", "  - id: impact-b", "    text: Reduced operating cost.",
		"skills:", "  - id: tech", "    name: Technologies", "    skills: [Go, SQL]",
		"employment:", "  - id: acme", "    name: Acme", "    roles:",
		"      - id: role-1", "        title: Engineer", "        start: '2020'",
		"        accomplishments: [Built a service, Launched a product]",
	)
	writeResume := func(name, company, headline string, pageLimit int, summary, bullets string, sections []string) string {
		lines := []string{
			"schema: resume/v1", "kind: resume", "id: r1", "profile: profile.yml",
			"target:", "  company: " + company, "  role: Engineer",
		}
		if headline != "" {
			lines = append(lines, "headline: "+headline)
		}
		lines = append(lines, "summary:", "  text: "+summary)
		if pageLimit > 0 {
			lines = append(lines, "pageLimit: "+strconv.Itoa(pageLimit))
		}
		if bullets != "" {
			lines = append(lines, "experience:", "  - company: acme", "    roles: [role-1]", "    bullets: ["+bullets+"]")
		}
		if sections != nil {
			if hasSection(sections, "skills") {
				lines = append(lines, "skills:", "  - group: tech", "    skills: [Go]")
			}
			lines = append(lines, "sections: ["+strings.Join(sections, ", ")+"]")
		}
		return writeDiffYAML(t, dir, name, lines...)
	}
	return dir, writeResume
}

func TestCompare_IdenticalResumes(t *testing.T) {
	_, writeResume := diffFixture(t)
	left := writeResume("left.yml", "Acme", "", 1, "A useful summary.", "Built a service", []string{"summary", "experience", "skills"})
	right := writeResume("right.yml", "Acme", "", 1, "A useful summary.", "Built a service", []string{"summary", "experience", "skills"})
	result, err := Compare(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Identical || len(result.Changes) != 0 {
		t.Fatalf("expected identical resolved resumes, got %+v", result)
	}
}

func TestCompare_ReportsMetadataContentAndOrdering(t *testing.T) {
	_, writeResume := diffFixture(t)
	left := writeResume("left.yml", "Acme", "", 0, "A useful summary.", "Built a service", []string{"summary", "experience", "skills"})
	right := writeResume("right.yml", "Beta", "Lead Engineer", 2, "A changed summary.", "Launched a product", []string{"skills", "summary", "experience"})
	result, err := Compare(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if result.Identical {
		t.Fatal("expected differences")
	}
	for _, want := range []struct{ kind, section, field string }{
		{"changed", "metadata", "company"},
		{"changed", "metadata", "headline"},
		{"changed", "metadata", "pageLimit"},
		{"changed", "summary", "text"},
		{"changed", "experience", "text"},
		{"reordered", "sections", "order"},
	} {
		if !hasChange(result, want.kind, want.section, want.field) {
			t.Errorf("missing %s %s/%s in %+v", want.kind, want.section, want.field, result.Changes)
		}
	}
}

func TestCompare_ReportsAddedAndRemovedSkills(t *testing.T) {
	_, writeResume := diffFixture(t)
	left := writeResume("left.yml", "Acme", "", 0, "A useful summary.", "", []string{"summary", "skills"})
	right := writeResume("right.yml", "Acme", "", 0, "A useful summary.", "", []string{"summary"})
	result, err := Compare(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(result, "removed", "skills", "name") {
		t.Fatalf("expected removed skill entry, got %+v", result.Changes)
	}
}

func TestCompare_ReportsReorderedAccomplishments(t *testing.T) {
	_, writeResume := diffFixture(t)
	left := writeResume("left.yml", "Acme", "", 0, "A useful summary.", "Built a service, Launched a product", []string{"summary", "experience"})
	right := writeResume("right.yml", "Acme", "", 0, "A useful summary.", "Launched a product, Built a service", []string{"summary", "experience"})
	result, err := Compare(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(result, "reordered", "experience/acme/role-1/accomplishments", "order") {
		t.Fatalf("expected reordered accomplishments, got %+v", result.Changes)
	}
}

func TestCompare_ReportsReorderedImpact(t *testing.T) {
	dir, _ := diffFixture(t)
	left := writeDiffYAML(t, dir, "left.yml",
		"schema: resume/v1", "kind: resume", "id: r1", "profile: profile.yml",
		"target:", "  company: Acme", "  role: Engineer",
		"impact:", "  - id: impact-a", "  - id: impact-b", "sections: [impact]",
	)
	right := writeDiffYAML(t, dir, "right.yml",
		"schema: resume/v1", "kind: resume", "id: r1", "profile: profile.yml",
		"target:", "  company: Acme", "  role: Engineer",
		"impact:", "  - id: impact-b", "  - id: impact-a", "sections: [impact]",
	)
	result, err := Compare(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(result, "reordered", "impact", "order") {
		t.Fatalf("expected reordered impact statements, got %+v", result.Changes)
	}
}

func TestCompare_ReportsTemplateSettings(t *testing.T) {
	dir, _ := diffFixture(t)
	writeDiffYAML(t, dir, "template-left.yml",
		"schema: resume/v1", "kind: template", "id: standard",
		"page:", "  width: 612", "  height: 792",
		"fonts:", "  regular: Arial", "  bold: Arial",
		"colors:", "  heading: '#123456'",
	)
	writeDiffYAML(t, dir, "template-right.yml",
		"schema: resume/v1", "kind: template", "id: standard",
		"page:", "  width: 612", "  height: 792",
		"fonts:", "  regular: Arial", "  bold: Arial",
		"colors:", "  heading: '#654321'",
	)
	left := writeDiffYAML(t, dir, "left.yml",
		"schema: resume/v1", "kind: resume", "id: r1", "profile: profile.yml", "template: template-left.yml",
		"target:", "  company: Acme", "  role: Engineer", "summary:", "  text: A useful summary.", "sections: [summary]",
	)
	right := writeDiffYAML(t, dir, "right.yml",
		"schema: resume/v1", "kind: resume", "id: r1", "profile: profile.yml", "template: template-right.yml",
		"target:", "  company: Acme", "  role: Engineer", "summary:", "  text: A useful summary.", "sections: [summary]",
	)
	result, err := Compare(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if !hasChange(result, "changed", "metadata", "settings") {
		t.Fatalf("expected template settings change, got %+v", result.Changes)
	}
}

func TestRun_JSONAndExitCodes(t *testing.T) {
	_, writeResume := diffFixture(t)
	left := writeResume("left.yml", "Acme", "", 0, "A useful summary.", "", []string{"summary"})
	right := writeResume("right.yml", "Acme", "", 0, "A useful summary.", "", []string{"summary"})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if code := Run([]string{left, right}, stdout, stderr, true); code != 0 {
		t.Fatalf("expected identical resumes to return zero, got %d: %s", code, stderr.String())
	}
	var result Result
	if err := json.NewDecoder(stdout).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if !result.Identical {
		t.Fatalf("expected identical JSON result, got %+v", result)
	}

	right = writeResume("different.yml", "Beta", "", 0, "A useful summary.", "", []string{"summary"})
	stdout.Reset()
	if code := Run([]string{left, right}, stdout, stderr, true); code == 0 {
		t.Fatal("expected differences to return non-zero")
	}
}

func hasChange(result Result, changeType, section, field string) bool {
	for _, change := range result.Changes {
		if change.Type == changeType && change.Section == section && change.Field == field {
			return true
		}
	}
	return false
}

func hasSection(sections []string, name string) bool {
	for _, section := range sections {
		if section == name {
			return true
		}
	}
	return false
}
