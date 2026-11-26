# resume-builder

A small, local, deterministic Go CLI that generates standardized résumé PDFs from version-controlled YAML files.

## Overview

The CLI reads YAML files describing your career profile, base selections, and target-specific résumé definitions, then produces a polished PDF. It is designed to be used by both humans and AI coding agents.

Key principles:

- **Git is the database** — all source data lives as ordinary YAML files.
- **Same inputs and timestamp produce identical output** — pass `--timestamp` for reproducible PDF bytes.
- **Nothing is included implicitly** — content renders only when explicitly selected.
- **Content and presentation are separate** — templates control layout, never content.
- **No network access required** — everything runs locally with no external services.

## For humans

### Run the CLI

From the repository root, use `go run ./cmd/resume` (Go 1.27 or newer). These examples use the checked-in sample résumé:

```bash
# Check the YAML and all referenced files
go run ./cmd/resume validate resumes/example.yml

# Preview the fully resolved content and its sources
go run ./cmd/resume inspect resumes/example.yml

# Create a PDF and a content manifest
go run ./cmd/resume build resumes/example.yml --output output/example.pdf --manifest output/example.manifest.json

# Use a fixed timestamp for byte-for-byte reproducible builds
go run ./cmd/resume build resumes/example.yml --output output/example.pdf --manifest output/example.manifest.json --timestamp 2026-01-02T03:04:05Z

# Show which profile, base, and template files a résumé uses
go run ./cmd/resume dependencies resumes/example.yml

# Compare two resolved résumé definitions
go run ./cmd/resume diff resumes/example.yml resumes/another.yml
```

The build command creates missing output directories. If you have built or installed the executable, replace `go run ./cmd/resume` with `resume` (or `.\resume.exe` in PowerShell).

### Commands

| Command | Description |
|---------|-------------|
| `resume validate PATH` | Validate a YAML file and its dependency graph |
| `resume inspect PATH` | Print fully resolved content, sections, and source attribution |
| `resume diff LEFT RIGHT` | Compare two resolved résumé definitions |
| `resume build PATH [--output PDF] [--manifest JSON] [--timestamp RFC3339]` | Generate a PDF and content manifest |
| `resume dependencies PATH` | Print the dependency graph for a file |
| `resume version` | Print the CLI version |

Add `--json` to supported commands for machine-readable output.

### Review a résumé

1. Run `validate` on the résumé YAML. Fix errors and review any warnings before building.
2. Run `inspect` and check that the resolved content, source files, section order, and selected items are what you intended. Use `--json` when you want to inspect the structured report.
3. Build the PDF and manifest. The manifest records resolved sections, input hashes, page count, and warnings.
4. Open the PDF and review every page for correct reading order, legibility, wrapping, missing or duplicated content, link behavior, and unintended blank space. A successful build does not replace visual review.
5. When revising a résumé, run `diff` against the previous definition to review content changes. It compares resolved content, not PDF appearance; it returns exit status 1 when differences are found.
6. If a file reference is unclear, run `dependencies` to see the profile, base, and template graph.

Commands take explicit paths; files do not have to be in the conventional directories shown below. The CLI resolves file references relative to the YAML file that contains each reference.

## For AI agents

See [AI_GUIDE.md](AI_GUIDE.md) for instructions on where to put YAML files, how to structure profile/base/resume documents, and how to map career facts into résumé sections without inventing content.

## File kinds

YAML files use one of four kinds:

- **`profile`** — Reusable career information (contact, experience, skills, education)
- **`base`** — Shared selections and defaults inherited by multiple résumés
- **`resume`** — Target-specific definition (company, role, content selections)
- **`template`** — Presentation rules only (fonts, colors, spacing, layout)

## Typical repository structure

```
resume/
  profile.yml
  bases/
    staff-engineer.yml
  resumes/
    example.yml
  templates/
    standard.yml
  output/
```

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the high-level package structure and design decisions.

## Ground rules

Development guidelines and code style are documented in [.junie/AGENTS.md](.junie/AGENTS.md).

## License

MIT