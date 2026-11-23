# Architecture

This document describes the high-level package structure of the resume-builder CLI.

## Layout

```
resume-builder/
  cmd/
    resume/              # CLI entry point (main.go)
  internal/
    schema/              # Shared YAML envelope and Go structs
    resolve/             # File resolution and content inheritance
    validate/            # Validation command and logic
    inspect/             # Inspect command and resolved output
    diff/                # Diff command and comparison logic
    dependencies/        # Dependencies command and graph traversal
    build/               # Build command, PDF rendering, manifest
    template/            # Template handling and rendering rules
  fonts/                 # Embedded font files for PDF generation
```

## Packages

### `cmd/resume`

CLI entry point. Parses subcommands and delegates to the appropriate `internal/` package. Keeps the main package thin — only flag parsing and dispatch.

**Related issues:** #2 (scaffold Go module and CLI entry point)

---

### `internal/schema`

Shared types used across all commands:

- YAML file envelope (`schema`, `kind`, `id`)
- Go structs for `profile`, `base`, `resume`, and `template` kinds
- JSON Schema definitions for agent discovery
- Common validation helpers

This is the only package that defines the data model. All other packages import from here.

**Related issues:** #3 (YAML envelope and structs), #4 (JSON Schema definitions)

---

### `internal/resolve`

File resolution and content inheritance:

- Resolves relative file references between YAML files
- Walks the `extends` chain for base inheritance
- Detects circular references, missing files, duplicate IDs
- Merges base defaults with resume-specific overrides
- Produces a fully resolved resume structure ready for rendering

This package handles the "graph" aspect — how files reference each other and how content flows from profile → base → resume.

**Related issues:** #5 (YAML file resolution), #6 (content resolution with base inheritance)

---

### `internal/validate`

Validation command:

- Validates a single YAML file and its entire dependency graph
- Checks structure, required fields, referenced kinds, schema versions
- Returns structured errors with file paths and line numbers
- Supports both human-readable and JSON output

**Related issues:** #7 (validate command)

---

### `internal/inspect`

Inspect command:

- Resolves a resume and prints the complete content
- Shows section order, headings, source file for each item
- Indicates whether each item was inherited or target-specific
- Supports both human-readable and JSON output

**Related issues:** #8 (inspect command)

---

### `internal/diff`

Diff command:

- Resolves two resume definitions and compares them
- Reports added, removed, changed, and reordered content
- Reports metadata, template, and page-limit changes
- Supports both human-readable and JSON output

**Related issues:** #9 (diff command)

---

### `internal/dependencies`

Dependencies command:

- Walks the dependency graph from a given resume file
- Prints all referenced files and their relationships
- Supports both human-readable and JSON output

**Related issues:** #10 (dependencies command)

---

### `internal/build`

Build command and PDF generation:

- Orchestrates validation → resolution → rendering
- Uses Maroto v2 to generate the PDF
- Produces the resolved content manifest (JSON)
- Performs visible-text integrity verification
- Returns structured build result with warnings and fingerprints

This is the heaviest package — it ties together resolution, rendering, and verification.

**Related issues:** #11 (Maroto v2 and PDF rendering), #12 (standard template), #13 (build command with manifest), #14 (visible-text verification), #15 (reproducibility), #17 (page-break handling)

---

### `internal/template`

Template handling:

- Parses template YAML files
- Provides rendering configuration (fonts, colors, spacing, margins)
- Enforces that templates cannot introduce resume content
- Supplies the default `standard` template

**Related issues:** #12 (standard template)

---

### `fonts/`

Static directory containing embedded font files used during PDF generation. Fonts are bundled with the executable so the CLI requires no external dependencies at runtime.

**Related issues:** #11 (PDF rendering)

## Design decisions

- **One package = one command or one shared concept.** Each CLI command lives in its own package under `internal/`. Shared types live in `schema`.
- **No DI framework.** Dependencies are passed directly. If a command needs a resolver, it constructs one or receives it as a parameter.
- **No separate interface packages.** Interfaces are defined where they are used, not in their own package.
- **Tests live alongside code.** Each package contains its own `_test.go` files with table-driven tests.
- **Internal only.** All application code lives under `internal/` or `cmd/`. Nothing is exported for external consumers.

## Build flow

```
resume build PATH
  → validate (internal/validate)
    → resolve (internal/resolve)
      → schema (internal/schema)
  → render (internal/build)
    → template (internal/template)
    → fonts/
  → verify (internal/build)
  → output: PDF + manifest JSON
```
