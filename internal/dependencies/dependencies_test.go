package dependencies

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDependencyYAML(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolve_SingleFileWithoutDependencies(t *testing.T) {
	path := writeDependencyYAML(t, t.TempDir(), "profile.yml",
		"schema: resume/v1", "kind: profile", "id: p1",
	)
	graph, err := Resolve(path)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Root != path || len(graph.Nodes) != 1 || len(graph.References) != 0 {
		t.Fatalf("unexpected single-file graph: %+v", graph)
	}
	if graph.Nodes[0].Kind != "profile" || graph.Nodes[0].ID != "p1" {
		t.Errorf("unexpected node: %+v", graph.Nodes[0])
	}
}

func TestResolve_TransitiveReferences(t *testing.T) {
	dir := t.TempDir()
	profile := writeDependencyYAML(t, dir, "profile.yml", "schema: resume/v1", "kind: profile", "id: p1")
	template := writeDependencyYAML(t, dir, "template.yml", "schema: resume/v1", "kind: template", "id: t1")
	base := writeDependencyYAML(t, dir, "base.yml",
		"schema: resume/v1", "kind: base", "id: b1", "profile: profile.yml", "template: template.yml",
	)
	resume := writeDependencyYAML(t, dir, "resume.yml",
		"schema: resume/v1", "kind: resume", "id: r1",
		"profile: profile.yml", "base: base.yml", "template: template.yml",
	)
	graph, err := Resolve(resume)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 4 || len(graph.References) != 5 {
		t.Fatalf("expected four unique files and five references, got %+v", graph)
	}
	if graph.Root != resume {
		t.Errorf("unexpected root %q, want %q", graph.Root, resume)
	}
	for _, want := range []struct{ from, field, to string }{
		{resume, "profile", profile},
		{resume, "base", base},
		{resume, "template", template},
		{base, "profile", profile},
		{base, "template", template},
	} {
		if !hasReference(graph, want.from, want.field, want.to) {
			t.Errorf("missing reference %+v in %+v", want, graph.References)
		}
	}
}

func TestRun_HumanTreeAndJSON(t *testing.T) {
	dir := t.TempDir()
	writeDependencyYAML(t, dir, "profile.yml", "schema: resume/v1", "kind: profile", "id: p1")
	resume := writeDependencyYAML(t, dir, "resume.yml",
		"schema: resume/v1", "kind: resume", "id: r1", "profile: profile.yml",
		"target:", "  company: Acme", "  role: Engineer",
	)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if code := Run([]string{resume}, stdout, stderr, false); code != 0 {
		t.Fatalf("expected successful dependencies command, got %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "profile: profile.yml [profile, id=p1]") {
		t.Errorf("human output omitted reference chain or node metadata: %s", stdout.String())
	}

	stdout.Reset()
	if code := Run([]string{resume}, stdout, stderr, true); code != 0 {
		t.Fatalf("expected JSON dependencies success, got %d: %s", code, stderr.String())
	}
	var graph Graph
	if err := json.NewDecoder(stdout).Decode(&graph); err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != 2 || len(graph.References) != 1 || graph.References[0].Field != "profile" {
		t.Fatalf("unexpected JSON graph: %+v", graph)
	}
}

func TestRun_ResolutionFailure(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	if code := Run([]string{filepath.Join(t.TempDir(), "missing.yml")}, stdout, stderr, true); code == 0 {
		t.Fatal("expected resolution failure to return non-zero")
	}
	var result map[string]string
	if err := json.NewDecoder(stdout).Decode(&result); err != nil {
		t.Fatalf("expected JSON error, got %q: %v", stdout.String(), err)
	}
	if result["error"] == "" {
		t.Fatalf("expected error message, got %+v", result)
	}
}

func hasReference(graph Graph, from, field, to string) bool {
	for _, reference := range graph.References {
		if reference.From == from && reference.Field == field && reference.To == to {
			return true
		}
	}
	return false
}
