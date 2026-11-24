package resolve

import (
	"os"
	"path/filepath"
	"testing"
)

func writeYAMLContent(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveContent_EmptyGraph(t *testing.T) {
	_, err := ResolveContent(nil)
	if err == nil {
		t.Error("expected error for nil graph")
	}

	_, err = ResolveContent(&Graph{})
	if err == nil {
		t.Error("expected error for empty graph")
	}
}

func TestResolveContent_MissingProfile(t *testing.T) {
	dir := t.TempDir()
	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = ResolveContent(graph)
	if err == nil {
		t.Error("expected error for missing profile")
	}
}

func TestResolveContent_MinimalResume(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
  email: john@example.com
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if resolved.Header.Name != "John Doe" {
		t.Errorf("expected name 'John Doe', got %q", resolved.Header.Name)
	}
	if resolved.Target.Company != "Acme" {
		t.Errorf("expected company 'Acme', got %q", resolved.Target.Company)
	}
	if resolved.Summary != nil {
		t.Error("expected nil summary")
	}
	if len(resolved.Sections) == 0 {
		t.Error("expected default sections")
	}
}

func TestResolveContent_SummaryFromProfile(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
summaries:
  - id: general
    text: Senior engineer with 10 years of experience.
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
summary:
  id: general
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if resolved.Summary == nil {
		t.Fatal("expected summary")
	}
	if resolved.Summary.Text != "Senior engineer with 10 years of experience." {
		t.Errorf("unexpected summary text: %q", resolved.Summary.Text)
	}
	if resolved.Summary.Source != "profile" {
		t.Errorf("expected source 'profile', got %q", resolved.Summary.Source)
	}
}

func TestResolveContent_SummaryInline(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
summary:
  text: Target-specific summary.
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if resolved.Summary == nil {
		t.Fatal("expected summary")
	}
	if resolved.Summary.Text != "Target-specific summary." {
		t.Errorf("unexpected summary text: %q", resolved.Summary.Text)
	}
	if resolved.Summary.Source != "resume" {
		t.Errorf("expected source 'resume', got %q", resolved.Summary.Source)
	}
}

func TestResolveContent_SummaryNotFound(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
summary:
  id: nonexistent
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = ResolveContent(graph)
	if err == nil {
		t.Error("expected error for missing summary id")
	}
}

func TestResolveContent_ImpactStatements(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
impact:
  - id: i1
    text: Led team of 5 engineers.
  - id: i2
    text: Reduced latency by 40%.
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
impact:
  - id: i1
  - text: Custom impact for this role.
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Impact) != 2 {
		t.Fatalf("expected 2 impact statements, got %d", len(resolved.Impact))
	}
	if resolved.Impact[0].Text != "Led team of 5 engineers." {
		t.Errorf("unexpected impact[0]: %q", resolved.Impact[0].Text)
	}
	if resolved.Impact[0].Source != "profile" {
		t.Errorf("expected source 'profile', got %q", resolved.Impact[0].Source)
	}
	if resolved.Impact[1].Text != "Custom impact for this role." {
		t.Errorf("unexpected impact[1]: %q", resolved.Impact[1].Text)
	}
	if resolved.Impact[1].Source != "resume" {
		t.Errorf("expected source 'resume', got %q", resolved.Impact[1].Source)
	}
}

func TestResolveContent_SkillsFromBase(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAMLContent(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
skills:
  - id: lang
    name: Languages
    skills:
      - Go
      - Python
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
base: base.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Skills) != 1 {
		t.Fatalf("expected 1 skill group, got %d", len(resolved.Skills))
	}
	if resolved.Skills[0].Name != "Languages" {
		t.Errorf("expected 'Languages', got %q", resolved.Skills[0].Name)
	}
	if resolved.Skills[0].Source != "base" {
		t.Errorf("expected source 'base', got %q", resolved.Skills[0].Source)
	}
}

func TestResolveContent_SkillsFromResume(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
skills:
  - id: lang
    name: Languages
    skills:
      - Go
      - Python
      - Rust
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
skills:
  - group: lang
    skills:
      - Go
      - Rust
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Skills) != 1 {
		t.Fatalf("expected 1 skill group, got %d", len(resolved.Skills))
	}
	if len(resolved.Skills[0].Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(resolved.Skills[0].Skills))
	}
	if resolved.Skills[0].Skills[0] != "Go" {
		t.Errorf("expected 'Go', got %q", resolved.Skills[0].Skills[0])
	}
	if resolved.Skills[0].Source != "resume" {
		t.Errorf("expected source 'resume', got %q", resolved.Skills[0].Source)
	}
}

func TestResolveContent_ExperienceFromBase(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAMLContent(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
employment:
  - id: acme
    name: Acme Corp
    location: San Francisco
    roles:
      - id: r1
        title: Senior Engineer
        start: 2020-01
        accomplishments:
          - Led team of 5
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
base: base.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Experience) != 1 {
		t.Fatalf("expected 1 employer, got %d", len(resolved.Experience))
	}
	if resolved.Experience[0].Name != "Acme Corp" {
		t.Errorf("expected 'Acme Corp', got %q", resolved.Experience[0].Name)
	}
	if resolved.Experience[0].Source != "base" {
		t.Errorf("expected source 'base', got %q", resolved.Experience[0].Source)
	}
	if len(resolved.Experience[0].Roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(resolved.Experience[0].Roles))
	}
}

func TestResolveContent_ExperienceFromResume(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
employment:
  - id: acme
    name: Acme Corp
    location: San Francisco
    roles:
      - id: r1
        title: Senior Engineer
        start: 2020-01
        accomplishments:
          - Led team of 5
          - Reduced latency by 40%
      - id: r2
        title: Staff Engineer
        start: 2023-01
        accomplishments:
          - Architected new platform
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
experience:
  - company: acme
    roles:
      - r1
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Experience) != 1 {
		t.Fatalf("expected 1 employer, got %d", len(resolved.Experience))
	}
	if len(resolved.Experience[0].Roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(resolved.Experience[0].Roles))
	}
	if resolved.Experience[0].Roles[0].Title != "Senior Engineer" {
		t.Errorf("expected 'Senior Engineer', got %q", resolved.Experience[0].Roles[0].Title)
	}
	// Unselected role should not appear.
	if resolved.Experience[0].Roles[0].ID != "r1" {
		t.Errorf("expected role r1, got %q", resolved.Experience[0].Roles[0].ID)
	}
}

func TestResolveContent_ExperienceBulletFilter(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
employment:
  - id: acme
    name: Acme Corp
    roles:
      - id: r1
        title: Senior Engineer
        start: 2020-01
        accomplishments:
          - Led team of 5
          - Reduced latency by 40%
          - Mentored junior engineers
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
experience:
  - company: acme
    roles:
      - r1
    bullets:
      - Led team of 5
      - Reduced latency by 40%
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Experience[0].Roles[0].Accomplishments) != 2 {
		t.Fatalf("expected 2 accomplishments, got %d", len(resolved.Experience[0].Roles[0].Accomplishments))
	}
}

func TestResolveContent_EducationFromBase(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
education:
  - id: cs
    institution: MIT
    degree: BS
    field: Computer Science
    graduation: 2015
`)

	writeYAMLContent(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
education:
  - id: cs
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
base: base.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Education) != 1 {
		t.Fatalf("expected 1 education entry, got %d", len(resolved.Education))
	}
	if resolved.Education[0].Institution != "MIT" {
		t.Errorf("expected 'MIT', got %q", resolved.Education[0].Institution)
	}
}

func TestResolveContent_EducationFromResume(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
education:
  - id: cs
    institution: MIT
    degree: BS
    field: Computer Science
  - id: mba
    institution: Stanford
    degree: MBA
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
education:
  - id: cs
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Education) != 1 {
		t.Fatalf("expected 1 education entry, got %d", len(resolved.Education))
	}
	if resolved.Education[0].Institution != "MIT" {
		t.Errorf("expected 'MIT', got %q", resolved.Education[0].Institution)
	}
}

func TestResolveContent_PageLimitInheritance(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAMLContent(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
pageLimit: 2
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
base: base.yaml
target:
  company: Acme
  role: Engineer
pageLimit: 1
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if resolved.PageLimit != 1 {
		t.Errorf("expected page limit 1 (resume override), got %d", resolved.PageLimit)
	}
}

func TestResolveContent_PageLimitFromBase(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAMLContent(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
pageLimit: 2
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
base: base.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if resolved.PageLimit != 2 {
		t.Errorf("expected page limit 2 from base, got %d", resolved.PageLimit)
	}
}

func TestResolveContent_SectionsFromBase(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAMLContent(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
sections:
  - summary
  - experience
  - skills
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
base: base.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(resolved.Sections))
	}
	if resolved.Sections[0] != "summary" {
		t.Errorf("expected 'summary', got %q", resolved.Sections[0])
	}
}

func TestResolveContent_SectionsResumeOverride(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAMLContent(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
sections:
  - summary
  - experience
  - skills
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
base: base.yaml
target:
  company: Acme
  role: Engineer
sections:
  - summary
  - experience
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(resolved.Sections))
	}
}

func TestResolveContent_UnselectedContentExcluded(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
employment:
  - id: acme
    name: Acme Corp
    roles:
      - id: r1
        title: Senior Engineer
        start: 2020-01
        accomplishments:
          - Led team of 5
  - id: beta
    name: Beta Inc
    roles:
      - id: r2
        title: Engineer
        start: 2018-01
        accomplishments:
          - Built API service
education:
  - id: cs
    institution: MIT
    degree: BS
    field: Computer Science
  - id: hs
    institution: Lincoln High
    degree: Diploma
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
experience:
  - company: acme
    roles:
      - r1
education:
  - id: cs
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	// Only Acme should appear, not Beta.
	if len(resolved.Experience) != 1 {
		t.Fatalf("expected 1 employer, got %d", len(resolved.Experience))
	}
	if resolved.Experience[0].Name != "Acme Corp" {
		t.Errorf("expected 'Acme Corp', got %q", resolved.Experience[0].Name)
	}

	// Only CS education should appear, not HS.
	if len(resolved.Education) != 1 {
		t.Fatalf("expected 1 education entry, got %d", len(resolved.Education))
	}
	if resolved.Education[0].Institution != "MIT" {
		t.Errorf("expected 'MIT', got %q", resolved.Education[0].Institution)
	}
}

func TestResolveContent_TemplateNotAdded(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAMLContent(t, dir, "template.yaml", `
schema: resume/v1
kind: template
id: standard
page:
  width: 8.5
  height: 11
margins:
  top: 0.5
  bottom: 0.5
  left: 0.75
  right: 0.75
fonts:
  regular: Arial
  bold: Arial Bold
colors:
  text: "#333333"
  heading: "#0066cc"
sizes:
  name: 18
  contact: 10
  heading: 12
  body: 10
  footer: 8
spacing:
  line: 1.2
  section: 6
  roleGap: 4
  bulletGap: 2
headings:
  uppercase: true
  rule: true
indent:
  bullet: 6
  role: 4
  accomplishment: 8
headerLayout:
  align: center
  compact: true
footerLayout:
  name: true
  page: true
pageBreaks:
  keepHeadingWithContent: true
  keepRoleWithAccomplishment: true
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
template: template.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	// Template should be available but should not add content.
	if resolved.Template == nil {
		t.Error("expected template to be set")
	}
	if resolved.TemplatePath == "" {
		t.Error("expected template path to be set")
	}
}

func TestResolveContent_FullChain(t *testing.T) {
	dir := t.TempDir()

	writeYAMLContent(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
  email: john@example.com
  phone: "555-1234"
employment:
  - id: acme
    name: Acme Corp
    location: San Francisco
    roles:
      - id: r1
        title: Senior Engineer
        start: 2020-01
        accomplishments:
          - Led team of 5
          - Reduced latency by 40%
      - id: r2
        title: Staff Engineer
        start: 2023-01
        accomplishments:
          - Architected new platform
skills:
  - id: lang
    name: Languages
    skills:
      - Go
      - Python
      - Rust
  - id: tools
    name: Tools
    skills:
      - Docker
      - Kubernetes
      - Terraform
education:
  - id: cs
    institution: MIT
    degree: BS
    field: Computer Science
    graduation: 2015
certifications:
  - id: aws
    name: AWS Solutions Architect
    issuer: Amazon
    dateEarned: 2022-06
`)

	writeYAMLContent(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
profile: profile.yaml
pageLimit: 2
sections:
  - summary
  - experience
  - skills
  - education
  - certifications
employment:
  - id: acme
    name: Acme Corp
    location: San Francisco
    roles:
      - id: r1
        title: Senior Engineer
        start: 2020-01
        accomplishments:
          - Led team of 5
          - Reduced latency by 40%
education:
  - id: cs
certifications:
  - id: aws
`)

	writeYAMLContent(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
base: base.yaml
target:
  company: Acme
  role: Staff Engineer
headline: Staff Software Engineer
summary:
  text: Senior engineer specializing in distributed systems.
experience:
  - company: acme
    roles:
      - r2
      - r1
skills:
  - group: lang
    skills:
      - Go
      - Rust
  - group: tools
education:
  - id: cs
certifications:
  - id: aws
`)

	r := &Resolver{}
	graph, err := r.Resolve(filepath.Join(dir, "resume.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveContent(graph)
	if err != nil {
		t.Fatal(err)
	}

	// Header from profile.
	if resolved.Header.Name != "John Doe" {
		t.Errorf("expected 'John Doe', got %q", resolved.Header.Name)
	}

	// Target from resume.
	if resolved.Target.Company != "Acme" {
		t.Errorf("expected 'Acme', got %q", resolved.Target.Company)
	}

	// Headline from resume.
	if resolved.Headline != "Staff Software Engineer" {
		t.Errorf("expected headline, got %q", resolved.Headline)
	}

	// Summary from resume (inline).
	if resolved.Summary == nil || resolved.Summary.Source != "resume" {
		t.Error("expected inline summary from resume")
	}

	// Experience: resume overrides base with different role order.
	if len(resolved.Experience) != 1 {
		t.Fatalf("expected 1 employer, got %d", len(resolved.Experience))
	}
	if len(resolved.Experience[0].Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(resolved.Experience[0].Roles))
	}
	// Resume specified r2 first, then r1.
	if resolved.Experience[0].Roles[0].ID != "r2" {
		t.Errorf("expected first role r2, got %q", resolved.Experience[0].Roles[0].ID)
	}
	if resolved.Experience[0].Roles[1].ID != "r1" {
		t.Errorf("expected second role r1, got %q", resolved.Experience[0].Roles[1].ID)
	}

	// Skills: resume overrides base with filtered skills.
	if len(resolved.Skills) != 2 {
		t.Fatalf("expected 2 skill groups, got %d", len(resolved.Skills))
	}
	if len(resolved.Skills[0].Skills) != 2 {
		t.Errorf("expected 2 filtered skills, got %d", len(resolved.Skills[0].Skills))
	}

	// Page limit from base (resume doesn't override).
	if resolved.PageLimit != 2 {
		t.Errorf("expected page limit 2, got %d", resolved.PageLimit)
	}

	// Sections from base (resume doesn't override).
	if len(resolved.Sections) != 5 {
		t.Errorf("expected 5 sections from base, got %d", len(resolved.Sections))
	}
}
