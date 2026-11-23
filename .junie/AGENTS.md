# Ground Rules for resume-builder

## Branching

- Every issue gets its own branch.
- Branch names should be descriptive, e.g. `issue-5-file-resolution`.

## Code Style

This is Go. Keep it simple. Any engineer should be able to read and understand the code without special knowledge.

### Keep it straightforward

- No fancy abstractions unless they solve a real, present problem.
- No DI frameworks or container systems. If a type needs a dependency, pass it directly.
- No generic soup. Use generics only when they reduce real duplication.
- Prefer clear, explicit code over clever or compact code.

### Co-locate related code

- Related code lives in the same package. Don't spread a single feature across many packages.
- A command's handler, its request types, its service logic, and its tests all live together.
- Example structure:

```
internal/
  validate/
    command.go
    validator.go
    validator_test.go
  inspect/
    command.go
    resolver.go
    resolver_test.go
  build/
    command.go
    renderer.go
    manifest.go
```

- Don't create separate packages for "interfaces" or "ports." Define interfaces where they're used, not in their own package.

### Package organization

- Use the `internal/` directory for all application code.
- Each CLI command or logical domain gets its own sub-package under `internal/`.
- Shared types (e.g., the YAML envelope, common structs) go in a small shared package like `internal/schema`.
- Keep packages small and focused. One package = one concept.

### Naming

- Use short, clear function and variable names.
- Prefer `resolve` over `performResolution`.
- Package names are short, lowercase, no underscores.

### Error handling

- Define custom error types when the caller needs to distinguish error kinds.
- Use `errors.As` / `errors.Is` for checking.
- Return errors, don't panic.

### Testing

- Tests live alongside the code they test, in `_test.go` files in the same package.
- Use table-driven tests.
- Test the behavior, not the implementation.

### Dependencies

- Keep external dependencies minimal.
- Prefer the standard library where it does the job.
- No hidden state, no global singletons.

### Go standard practices

- Follow effective Go and the standard Go project layout conventions.
- Use `go fmt` and `go vet`.
- Write idiomatic Go: error returns, interfaces satisfied implicitly, receivers chosen by size and mutability.
