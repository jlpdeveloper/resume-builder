package resolve

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/jlpdeveloper/resume-builder/internal/schema"
	"gopkg.in/yaml.v3"
)

// Document holds a parsed YAML file with its resolved path and parsed content.
type Document struct {
	Path     string // Absolute path to the file
	Envelope schema.Envelope
	Kind     schema.Kind
	Profile  string // File reference to a profile (set for base/resume)
	Base     string // File reference to a base (set for resume)
	Template string // File reference to a template (set for base/resume)
	Raw      []byte // Raw YAML bytes
}

// Graph is the result of resolving a file and all its dependencies.
type Graph struct {
	Root      string // Path to the root file that was resolved
	Documents []*Document
}

// Resolver loads and resolves YAML file references.
type Resolver struct{}

// Resolve loads the file at path and recursively resolves all its
// file references, returning a dependency graph.
func (r *Resolver) Resolve(path string) (*Graph, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	graph := &Graph{Root: abs}
	visited := make(map[string]bool)
	chain := make([]string, 0, 4)

	if err := r.resolveFile(abs, graph, visited, chain); err != nil {
		return nil, err
	}

	return graph, nil
}

// resolveFile loads a single file and recursively resolves its references.
func (r *Resolver) resolveFile(path string, graph *Graph, visited map[string]bool, chain []string) error {
	// Check for circular references.
	if slices.Contains(chain, path) {
		cycle := append(slices.Clone(chain), path)
		return &CycleError{cycle: cycle}
	}

	// Skip if already loaded.
	if visited[path] {
		return nil
	}

	// Read and parse the file.
	doc, err := r.load(path)
	if err != nil {
		return err
	}

	// Validate envelope.
	if err := doc.Envelope.Validate(); err != nil {
		return &EnvelopeError{path: path, err: err}
	}

	// Check schema version.
	if doc.Envelope.Schema != schema.SchemaV1 {
		return &UnsupportedSchemaError{
			path:   path,
			schema: doc.Envelope.Schema,
		}
	}

	// Check for duplicate IDs.
	for _, d := range graph.Documents {
		if d.Envelope.ID == doc.Envelope.ID && d.Path != path {
			return &DuplicateIDError{
				id:    doc.Envelope.ID,
				path:  path,
				exist: d.Path,
			}
		}
	}

	graph.Documents = append(graph.Documents, doc)
	visited[path] = true
	chain = append(chain, path)

	// Resolve file references based on kind.
	dir := filepath.Dir(path)

	switch doc.Kind {
	case schema.KindResume:
		// Resume can reference profile, base, and template.
		for ref, kind := range map[string]schema.Kind{
			doc.Profile:  schema.KindProfile,
			doc.Base:     schema.KindBase,
			doc.Template: schema.KindTemplate,
		} {
			if ref == "" {
				continue
			}
			resolved := filepath.Join(dir, ref)
			resolved, err = filepath.Abs(resolved)
			if err != nil {
				return fmt.Errorf("resolve reference %q: %w", ref, err)
			}

			if err := r.checkExists(resolved); err != nil {
				return &MissingFileError{path: resolved, referencedBy: path}
			}

			if err := r.checkKind(resolved, kind); err != nil {
				return err
			}

			if err := r.resolveFile(resolved, graph, visited, chain); err != nil {
				return err
			}
		}
	case schema.KindBase:
		// Base can reference profile and template.
		for ref, kind := range map[string]schema.Kind{
			doc.Profile:  schema.KindProfile,
			doc.Template: schema.KindTemplate,
		} {
			if ref == "" {
				continue
			}
			resolved := filepath.Join(dir, ref)
			resolved, err = filepath.Abs(resolved)
			if err != nil {
				return fmt.Errorf("resolve reference %q: %w", ref, err)
			}

			if err := r.checkExists(resolved); err != nil {
				return &MissingFileError{path: resolved, referencedBy: path}
			}

			if err := r.checkKind(resolved, kind); err != nil {
				return err
			}

			if err := r.resolveFile(resolved, graph, visited, chain); err != nil {
				return err
			}
		}
	}

	return nil
}

// load reads and parses a YAML file into a Document.
func (r *Resolver) load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &MissingFileError{path: path, referencedBy: "root"}
	}

	var doc Document
	doc.Path = path
	doc.Raw = data

	// Parse the envelope fields first.
	var envelope struct {
		Schema string `yaml:"schema"`
		Kind   string `yaml:"kind"`
		ID     string `yaml:"id"`
	}
	if err := yaml.Unmarshal(data, &envelope); err != nil {
		return nil, &ParseError{path: path, err: err}
	}

	doc.Envelope = schema.Envelope{
		Schema: envelope.Schema,
		Kind:   schema.Kind(envelope.Kind),
		ID:     envelope.ID,
	}
	doc.Kind = doc.Envelope.Kind

	// Parse kind-specific file references.
	switch schema.Kind(envelope.Kind) {
	case schema.KindResume:
		var resume struct {
			Profile  string `yaml:"profile"`
			Base     string `yaml:"base"`
			Template string `yaml:"template"`
		}
		if err := yaml.Unmarshal(data, &resume); err != nil {
			return nil, &ParseError{path: path, err: err}
		}
		doc.Profile = resume.Profile
		doc.Base = resume.Base
		doc.Template = resume.Template
	case schema.KindBase:
		var base struct {
			Profile  string `yaml:"profile"`
			Template string `yaml:"template"`
		}
		if err := yaml.Unmarshal(data, &base); err != nil {
			return nil, &ParseError{path: path, err: err}
		}
		doc.Profile = base.Profile
		doc.Template = base.Template
	}

	return &doc, nil
}

// checkExists returns an error if the file does not exist.
func (r *Resolver) checkExists(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return fmt.Errorf("stat file: %w", err)
	}
	return nil
}

// checkKind loads the envelope of a file and verifies it matches the expected kind.
func (r *Resolver) checkKind(path string, expected schema.Kind) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return &MissingFileError{path: path, referencedBy: ""}
	}

	var envelope struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &envelope); err != nil {
		return &ParseError{path: path, err: err}
	}

	if schema.Kind(envelope.Kind) != expected {
		return &KindMismatchError{
			path:     path,
			expected: expected,
			actual:   schema.Kind(envelope.Kind),
		}
	}

	return nil
}

// ---- Custom error types ----

// CycleError is returned when a circular reference chain is detected.
type CycleError struct {
	cycle []string
}

func (e *CycleError) Error() string {
	return "circular reference detected: " + filepath.Join(e.cycle...)
}

// Cycle returns the chain of file paths forming the cycle.
func (e *CycleError) Cycle() []string {
	return e.cycle
}

// FilePath returns the first file in the detected cycle.
func (e *CycleError) FilePath() string {
	if len(e.cycle) == 0 {
		return ""
	}
	return e.cycle[0]
}

// MissingFileError is returned when a referenced file does not exist.
type MissingFileError struct {
	path         string
	referencedBy string
}

func (e *MissingFileError) Error() string {
	if e.referencedBy != "" {
		return fmt.Sprintf("missing file %q (referenced by %q)", e.path, e.referencedBy)
	}
	return fmt.Sprintf("missing file: %s", e.path)
}

func (e *MissingFileError) Unwrap() error {
	return fs.ErrNotExist
}

// FilePath returns the missing dependency path.
func (e *MissingFileError) FilePath() string {
	return e.path
}

// KindMismatchError is returned when a referenced file has the wrong kind.
type KindMismatchError struct {
	path     string
	expected schema.Kind
	actual   schema.Kind
}

func (e *KindMismatchError) Error() string {
	return fmt.Sprintf(
		"kind mismatch for %q: expected %q, got %q",
		e.path, e.expected, e.actual,
	)
}

// FilePath returns the path with the unexpected kind.
func (e *KindMismatchError) FilePath() string {
	return e.path
}

// DuplicateIDError is returned when two resolved files share the same ID.
type DuplicateIDError struct {
	id    string
	path  string
	exist string
}

func (e *DuplicateIDError) Error() string {
	return fmt.Sprintf(
		"duplicate id %q: %s and %s",
		e.id, e.path, e.exist,
	)
}

// FilePath returns the file containing the duplicate ID.
func (e *DuplicateIDError) FilePath() string {
	return e.path
}

// UnsupportedSchemaError is returned when a file uses an unsupported schema version.
type UnsupportedSchemaError struct {
	path   string
	schema string
}

func (e *UnsupportedSchemaError) Error() string {
	return fmt.Sprintf(
		"unsupported schema version %q in %s (expected %q)",
		e.schema, e.path, schema.SchemaV1,
	)
}

// FilePath returns the file with the unsupported schema version.
func (e *UnsupportedSchemaError) FilePath() string {
	return e.path
}

// EnvelopeError wraps an envelope validation error with the file path.
type EnvelopeError struct {
	path string
	err  error
}

func (e *EnvelopeError) Error() string {
	return fmt.Sprintf("envelope error in %s: %s", e.path, e.err)
}

func (e *EnvelopeError) Unwrap() error {
	return e.err
}

// FilePath returns the file with the invalid envelope.
func (e *EnvelopeError) FilePath() string {
	return e.path
}

// ParseError is returned when a YAML file cannot be parsed.
type ParseError struct {
	path string
	err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("parse error in %s: %s", e.path, e.err)
}

func (e *ParseError) Unwrap() error {
	return e.err
}

// FilePath returns the file that could not be parsed.
func (e *ParseError) FilePath() string {
	return e.path
}
