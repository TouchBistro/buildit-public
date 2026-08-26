# Command Package `cmd`

This package implements the CLI interface for `buildit` using the Cobra framework.

## Structure

- `main.go`: The entry point of the application.
- `cmd/root.go`: Defines the root `buildit` command and global flags.
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
