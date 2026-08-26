# Configuration Package `config`

This package manages the loading, parsing, and validation of `buildit` configuration files (`buildit.yml`) and variable files.

## Core Components

- **`buildit.yml`**: The main infrastructure-as-code specification (YAML format).
- **Variables**: Support for `.vars` files, environment variables, and CLI overrides.
- **Viper**: Used for merging configuration from multiple sources section by section.

## Loading Process

1.  **Variable Discovery**: `util.ListMatchingFilenames` finds `.vars` files.
2.  **Variable Merging**: `LoadVariables` builds the Viper singleton and merges files, environment variables (`BUILDIT_` prefix), and CLI flags (`--variable key=value`).
3.  **Config Parsing**: `buildit.yml` is parsed into Go structs, and variables are interpolated.
4.  **Normalization**: Default values are set, and data structures are prepared.
5.  **Validation**: Ensures the configuration adheres to business rules and required fields are present.

## Guidelines

- **Fail Fast**: Validate configuration early and return clear, descriptive error messages.
- **Interpolation**: Support `${VAR_NAME}` syntax for variable substitution.
- **Provider Management**: Handle multiple AWS provider configurations (regions, profiles, roles).
- **Format Support**: Support multiple config file formats if necessary, but prioritize YAML.
- **Reserved tag keys**: the `buildit:` tag prefix belongs to buildit. `reserved_tags.go` rejects a
  config that sets a key in it, checking the raw parsed config before `Normalize` merges buildit's
  own tags in — after that point a user's key is indistinguishable from ours. New built-in tag keys
  go under that prefix, and `tagsFor` is where per-resource ones are attached. Never write a
  per-resource value into `globalTags` itself: it is shared by every resource, and `buildit audit`
  turns each of its keys into a tag filter.
