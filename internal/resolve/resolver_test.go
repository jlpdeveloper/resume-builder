package resolve

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jlpdeveloper/resume-builder/internal/schema"
)

func TestResolve_SingleFile(t *testing.T) {
	dir := t.TempDir()

	profile := writeYAML(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: my-profile
header:
  name: John Doe
`)

	r := &Resolver{}
	graph, err := r.Resolve(profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(graph.Documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(graph.Documents))
	}

	doc := graph.Documents[0]
	if doc.Envelope.ID != "my-profile" {
		t.Errorf("expected id my-profile, got %s", doc.Envelope.ID)
	}
	if doc.Kind != schema.KindProfile {
		t.Errorf("expected kind profile, got %s", doc.Kind)
	}
}

func TestResolve_ResumeWithDependencies(t *testing.T) {
	dir := t.TempDir()

	writeYAML(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAML(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
profile: profile.yaml
`)

	writeYAML(t, dir, "template.yaml", `
schema: resume/v1
kind: template
id: t1
page:
  width: 8.5
  height: 11
margins:
  top: 0.5
  bottom: 0.5
  left: 0.5
  right: 0.5
fonts:
  regular: DejaVu-Sans
  bold: DejaVu-Sans-Bold
colors:
  text: "#333333"
  heading: "#000000"
sizes:
  name: 18
  contact: 10
  heading: 12
  roleTitle: 11
  body: 10
  footer: 8
spacing:
  line: 1.2
  section: 6
  roleGap: 4
  bulletGap: 2
indent:
  bullet: 6
  role: 3
  accomplishment: 6
`)

	resume := writeYAML(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
base: base.yaml
template: template.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(resume)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(graph.Documents) != 4 {
		t.Fatalf("expected 4 documents, got %d", len(graph.Documents))
	}

	kinds := make(map[schema.Kind]bool)
	for _, doc := range graph.Documents {
		kinds[doc.Kind] = true
	}

	for _, kind := range []schema.Kind{schema.KindResume, schema.KindBase, schema.KindProfile, schema.KindTemplate} {
		if !kinds[kind] {
			t.Errorf("missing kind %s in resolved graph", kind)
		}
	}
}

func TestResolve_MissingFile(t *testing.T) {
	dir := t.TempDir()

	resume := writeYAML(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: nonexistent.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	_, err := r.Resolve(resume)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}

	var missing *MissingFileError
	if !errors.As(err, &missing) {
		t.Errorf("expected MissingFileError, got %T: %v", err, err)
	}
}

func TestResolve_NonExistentRoot(t *testing.T) {
	r := &Resolver{}
	_, err := r.Resolve("/nonexistent/path/resume.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent root, got nil")
	}
}

func TestResolve_KindMismatch(t *testing.T) {
	dir := t.TempDir()

	writeYAML(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	resume := writeYAML(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
base: profile.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	_, err := r.Resolve(resume)
	if err == nil {
		t.Fatal("expected kind mismatch error, got nil")
	}

	var mismatch *KindMismatchError
	if !errors.As(err, &mismatch) {
		t.Errorf("expected KindMismatchError, got %T: %v", err, err)
	}
}

func TestResolve_CircularReference(t *testing.T) {
	path := "a.yaml"
	err := (&Resolver{}).resolveFile(path, &Graph{}, map[string]bool{}, []string{path})
	if err == nil {
		t.Fatal("expected circular reference error, got nil")
	}
	var cycle *CycleError
	if !errors.As(err, &cycle) {
		t.Fatalf("expected CycleError, got %T: %v", err, err)
	}
	if len(cycle.Cycle()) != 2 || cycle.Cycle()[0] != path || cycle.Cycle()[1] != path {
		t.Errorf("unexpected cycle chain: %v", cycle.Cycle())
	}
}

func TestResolve_DuplicateID(t *testing.T) {
	dir := t.TempDir()

	writeYAML(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: dup
header:
  name: John Doe
`)

	writeYAML(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: dup
profile: profile.yaml
`)

	resume := writeYAML(t, dir, "resume.yaml", `
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
	_, err := r.Resolve(resume)
	if err == nil {
		t.Fatal("expected duplicate ID error, got nil")
	}

	var dupErr *DuplicateIDError
	if !errors.As(err, &dupErr) {
		t.Errorf("expected DuplicateIDError, got %T: %v", err, err)
	}
}

func TestResolve_UnsupportedSchema(t *testing.T) {
	dir := t.TempDir()

	profile := writeYAML(t, dir, "profile.yaml", `
schema: resume/v0
kind: profile
id: p1
header:
  name: John Doe
`)

	r := &Resolver{}
	_, err := r.Resolve(profile)
	if err == nil {
		t.Fatal("expected unsupported schema error, got nil")
	}

	var unsupported *UnsupportedSchemaError
	if !errors.As(err, &unsupported) {
		t.Errorf("expected UnsupportedSchemaError, got %T: %v", err, err)
	}
}

func TestResolve_RelativePathInSubdir(t *testing.T) {
	dir := t.TempDir()

	sub := filepath.Join(dir, "subdir")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}

	writeYAML(t, sub, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	resume := writeYAML(t, sub, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(resume)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(graph.Documents) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(graph.Documents))
	}
}

func TestResolve_ParentDirectoryReference(t *testing.T) {
	dir := t.TempDir()

	sub := filepath.Join(dir, "jobs")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}

	writeYAML(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	resume := writeYAML(t, sub, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: ../profile.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(resume)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(graph.Documents) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(graph.Documents))
	}
}

func TestResolve_TemplateNoRefs(t *testing.T) {
	dir := t.TempDir()

	template := writeYAML(t, dir, "template.yaml", `
schema: resume/v1
kind: template
id: t1
page:
  width: 8.5
  height: 11
margins:
  top: 0.5
  bottom: 0.5
  left: 0.5
  right: 0.5
fonts:
  regular: DejaVu-Sans
  bold: DejaVu-Sans-Bold
colors:
  text: "#333333"
  heading: "#000000"
sizes:
  name: 18
  contact: 10
  heading: 12
  roleTitle: 11
  body: 10
  footer: 8
spacing:
  line: 1.2
  section: 6
  roleGap: 4
  bulletGap: 2
indent:
  bullet: 6
  role: 3
  accomplishment: 6
`)

	r := &Resolver{}
	graph, err := r.Resolve(template)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(graph.Documents) != 1 {
		t.Fatalf("expected 1 document, got %d", len(graph.Documents))
	}
}

func TestResolve_MissingEnvelopeField(t *testing.T) {
	dir := t.TempDir()

	file := writeYAML(t, dir, "bad.yaml", `
kind: profile
id: p1
header:
  name: John Doe
`)

	r := &Resolver{}
	_, err := r.Resolve(file)
	if err == nil {
		t.Fatal("expected envelope error, got nil")
	}

	var envErr *EnvelopeError
	if !errors.As(err, &envErr) {
		t.Errorf("expected EnvelopeError, got %T: %v", err, err)
	}
}

func TestResolve_BaseWithProfileAndTemplate(t *testing.T) {
	dir := t.TempDir()

	writeYAML(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAML(t, dir, "template.yaml", `
schema: resume/v1
kind: template
id: t1
page:
  width: 8.5
  height: 11
margins:
  top: 0.5
  bottom: 0.5
  left: 0.5
  right: 0.5
fonts:
  regular: DejaVu-Sans
  bold: DejaVu-Sans-Bold
colors:
  text: "#333333"
  heading: "#000000"
sizes:
  name: 18
  contact: 10
  heading: 12
  roleTitle: 11
  body: 10
  footer: 8
spacing:
  line: 1.2
  section: 6
  roleGap: 4
  bulletGap: 2
indent:
  bullet: 6
  role: 3
  accomplishment: 6
`)

	base := writeYAML(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
profile: profile.yaml
template: template.yaml
`)

	r := &Resolver{}
	graph, err := r.Resolve(base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(graph.Documents) != 3 {
		t.Fatalf("expected 3 documents, got %d", len(graph.Documents))
	}
}

func TestResolve_SharedDependency(t *testing.T) {
	// Resume and base both reference the same profile — it should be loaded once.
	dir := t.TempDir()

	writeYAML(t, dir, "profile.yaml", `
schema: resume/v1
kind: profile
id: p1
header:
  name: John Doe
`)

	writeYAML(t, dir, "template.yaml", `
schema: resume/v1
kind: template
id: t1
page:
  width: 8.5
  height: 11
margins:
  top: 0.5
  bottom: 0.5
  left: 0.5
  right: 0.5
fonts:
  regular: DejaVu-Sans
  bold: DejaVu-Sans-Bold
colors:
  text: "#333333"
  heading: "#000000"
sizes:
  name: 18
  contact: 10
  heading: 12
  roleTitle: 11
  body: 10
  footer: 8
spacing:
  line: 1.2
  section: 6
  roleGap: 4
  bulletGap: 2
indent:
  bullet: 6
  role: 3
  accomplishment: 6
`)

	writeYAML(t, dir, "base.yaml", `
schema: resume/v1
kind: base
id: b1
profile: profile.yaml
template: template.yaml
`)

	resume := writeYAML(t, dir, "resume.yaml", `
schema: resume/v1
kind: resume
id: r1
profile: profile.yaml
base: base.yaml
template: template.yaml
target:
  company: Acme
  role: Engineer
`)

	r := &Resolver{}
	graph, err := r.Resolve(resume)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 4 unique files: resume, base, profile, template
	if len(graph.Documents) != 4 {
		t.Fatalf("expected 4 documents (shared deps deduplicated), got %d", len(graph.Documents))
	}
}

func writeYAML(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
	return path
}
