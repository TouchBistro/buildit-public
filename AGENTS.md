# GitHub Copilot Code Review Instructions for buildit

This document defines the core coding standards and practices for the buildit project. For detailed implementation patterns in specific areas of the codebase, refer to the [Package Guides](#package-guides).

## Package Guides

- [**`cmd/`**](./cmd/AGENTS.md): CLI implementation (Cobra), commands, and flags.
- [**`config/`**](./config/AGENTS.md): Configuration loading, variable merging, and validation.
- [**`resource/`**](./resource/AGENTS.md): Resource lifecycle (Apply, Destroy, Compare), interfaces, and ARN resolution patterns.
- [**`awsw/`**](./awsw/AGENTS.md): AWS SDK wrappers, client access, and common AWS operations.
- [**`util/`**](./util/AGENTS.md): General-purpose utility functions (string, map, slice, file).
- `docs/`: Resource documentation standards and templates.

## General Coding Principles

### Code Organization

- Follow Go standard project layout.
- Keep packages focused on single responsibilities.
- Encapsulate external dependencies (like AWS SDK) within wrapper packages.

### Minimalism and Clarity

- Write concise, self-documenting code.
- Avoid obvious comments; focus on explaining _why_ when necessary.
- Prefer simplicity over cleverness.

### User-facing Messages

- Use clear, actionable language.
- Avoid technical jargon.
- Provide context in error messages.
- Use color output consistently for success (Green), updates (Yellow), and errors (Red).

## Go Language Style & Modern Standards (SOTA)

### Modern Go Conventions

- **Use `any`**: Always use the `any` keyword instead of `interface{}`.
- **Slice & Map Helpers**: Prefer the standard library `slices` and `maps` packages for common operations (Go 1.21+).
- **Structured Logging**: Use log levels correctly and include context fields; avoid `fmt.Printf` for application logs.
- **Generics**: Use generics (`[T any]`) for reusable utility functions and collection types to ensure type safety.
- **Error Wrapping**: Use `errors.Wrap` for adding context and `errors.Is`/`errors.As` for checking errors.

### Naming Conventions

- **Exported**: `PascalCase`
- **Unexported**: `camelCase`
- **Acronyms**: Consistent casing (e.g., `ACMCertificate`, `SNSSubscription`).
- Use short, descriptive names in narrow scopes; full names in package scope.

### Error Handling

- Always wrap errors with context using `github.com/pkg/errors`:
  ```go
  return errors.Wrapf(err, "failed to perform action on %v", identifier)
  ```
- Use `errors.As()` and `errors.Is()` for error checking.
- Return structured errors for business logic failures (e.g., `ValidationError`).

### Logging

- Use `github.com/sirupsen/logrus` for structured logging.
- Include context in log fields: `log.WithFields(log.Fields{"key": value}).Info("message")`.
- Use appropriate levels: `Trace`, `Debug` (verbose operational details), `Info` (user-facing), `Warn`, `Error`, `Fatal`.

### Context Usage

- Always accept `context.Context` as the first parameter and propagate it.
- Check for context cancellation in long-running or blocking operations.

### Pointers and Values

- Use pointers for large structs or when `nil` is meaningful.
- Follow AWS SDK conventions for pointer usage with SDK types.
- Use value receivers unless modification or large copy avoidance is required.

## Testing Standards

- Use table-driven tests where appropriate.
- Name test files `*_test.go`.
- Ensure tests clean up resources on completion.
- Use descriptive names and subtests (`t.Run()`).
- **Generic values only.** Test fixtures must never contain real values: no real AWS account IDs
  (use `123456789012` or the other documentation accounts), no real resource, service, team, or
  person names (use `example-*`, `old-*`/`new-*`, `test-*`), no `tb`-prefixed names (the company
  name-prefix — write `example-*` directly), no internal hostnames (use `example.com`). This repo
  is published as a scrubbed public mirror; the `public-snapshot-check` CI job fails the PR on
  leaks (see `scripts/AGENTS.md`), so a real value blocks your own merge.

## Security

- Never log or commit secrets, credentials, or API keys.
- Use the AWS SDK credential chain.
- Follow the principle of least privilege for IAM requirements.

## Performance & Environment

- **`GOCACHE`**: Set this to `[project_root_directory]/cache` when running Go tools to avoid permission issues in restricted environments.
- Use goroutines for independent operations when appropriate.
- Paginate large AWS result sets.

## When to Document

Documentation is **required** any time you:

- Add a new top-level resource type (create a new file in `docs/resources/`).
- Add a new target type (e.g., `firehose`, `sqs`) to an existing resource such as `eventbridge-rule`.
- Add new fields or sub-structures to an existing resource.

Documentation must be added in the **same PR** as the implementation. A feature is not complete until its corresponding `docs/resources/` entry is updated.

**Generic values only.** Documentation and examples must never contain real values — the same rule as test fixtures (see [Testing Standards](#testing-standards)): documentation AWS accounts, `example-*` names, `example.com` hostnames. `docs/` ships in the public mirror, and the `public-snapshot-check` CI job fails the PR on any leaked account ID, internal name, or non-allowlisted hostname.

## Anti-Patterns to Avoid

- ❌ **Ignoring errors**: Always handle or return errors.
- ❌ **Naked returns**: Avoid in complex functions.
- ❌ **Panic**: Only use in `Normalize()` for critical configuration errors.
- ❌ **Duplicate Logging**: Log once at the appropriate level before returning.
- ❌ **Hard-coding**: Never hard-code regions, account IDs, or identifiers.
- ❌ **Mixing Concerns**: Keep validation and normalization separate.
- ❌ **Unnecessary Diffs**: Don't return a non-nil `ResourceDiff` if there are no changes.

## Review Checklist

- ✅ Error handling with proper context wrapping.
- ✅ Context propagation through the call chain.
- ✅ Consistent naming and logging.
- ✅ Input validation and normalization.
- ✅ Tests covering happy paths and error cases.
- ✅ No hard-coded configuration or secrets.
- ✅ Code quality checks pass: `go fmt`, `go build`, `golangci-lint`.
