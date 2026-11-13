# Command Package `cmd`

This package implements the CLI interface for `buildit` using the Cobra framework.

## Structure

- `cmd/buildit/main.go`: The entry point of the CLI binary (`go build ./cmd/buildit`). The
  module root is the `buildit` library package, so `main` cannot live there.
- `cmd/root.go`: Defines the root `buildit` command and global flags. Its persistent pre-run
  builds the `default` provider from the `--default-provider-type` family of flags
  (`client.InitDefaultProvider`) before `config.Parse`, replacing any registered `default`; the
  shared pipeline only builds one when none is registered.
- `cmd/[command].go`: Each major CLI command (apply, destroy, etc.) has its own file.

## Command Implementation Pattern

Commands are created using constructor functions:

```go
func newCommandName(c *config.Container) *cobra.Command {
    return &cobra.Command{
        Use:     "command-name",
        Aliases: []string{"alias"},
        Args:    cobra.NoArgs,
        Short:   "Short description",
        RunE: func(cmd *cobra.Command, args []string) error {
            // Implementation
            return nil
        },
    }
}
```

## Flag Definitions

- Use descriptive help messages.
- Group related flags together.
- Use sensible defaults.
- Mark deprecated flags with `MarkDeprecated()`.

## Guidelines

- **Argument Validation**: Use `cobra.NoArgs`, `cobra.ExactArgs(n)`, etc., to enforce usage.
- **Error Handling**: Return errors from `RunE` to allow Cobra to handle them and exit with appropriate codes.
- **Dependency Injection**: Use the `config.Container` to pass shared dependencies like configuration loaders or clients.
- **User Output**: Use `color` and `logrus` for user-facing output. Ensure output is actionable and readable.
