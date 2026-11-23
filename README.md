# resume-builder

A small, local, deterministic Go CLI that generates standardized résumé PDFs from version-controlled YAML files.

## Overview

The CLI reads YAML files describing your career profile, base selections, and target-specific résumé definitions, then produces a polished PDF. It is designed to be used by both humans and AI coding agents.

Key principles:

- **Git is the database** — all source data lives as ordinary YAML files.
- **Same inputs produce equivalent output** — builds are deterministic and reproducible.
- **Nothing is included implicitly** — content renders only when explicitly selected.
- **Content and presentation are separate** — templates control layout, never content.
- **No network access required** — everything runs locally with no external services.

## Quick start

```bash
# Validate a resume and its dependencies
resume validate resumes/platform-engineer.yml

# Inspect the fully resolved resume content
resume inspect resumes/platform-engineer.yml

# Build a PDF
resume build resumes/platform-engineer.yml

# Compare two resolved resumes
resume diff resumes/resume-a.yml resumes/resume-b.yml
```

## Commands

| Command | Description |
|---------|-------------|
| `resume validate PATH` | Validate a YAML file and its dependency graph |
| `resume inspect PATH` | Print the fully resolved résumé content |
| `resume diff LEFT RIGHT` | Compare two resolved résumé definitions |
| `resume build PATH` | Generate a PDF and content manifest |
| `resume schema KIND` | Print the JSON Schema for a file kind |
| `resume dependencies PATH` | Print the dependency graph for a file |
| `resume version` | Print CLI and schema version information |

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
    rapid7-ai-soc.yml
    platform-engineer.yml
  templates/
    standard.yml
  output/
```

Commands operate on explicit paths — the CLI does not depend on files being placed in any configured directory.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the high-level package structure and design decisions.

## Ground rules

Development guidelines and code style are documented in [.junie/AGENTS.md](.junie/AGENTS.md).

## License

MIT