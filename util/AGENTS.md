# Utility Package `util`

This package contains general-purpose utility functions and types used across the `buildit` project.

## Core Utilities

### Coalesce Helpers (`coalesce.go`)

- Functions for providing default values for pointers: `Coalesce`, `CoalesceInt32`, `CoalesceFloat64`.
- Generic `CoalesceComparable[T comparable]` for any comparable type.

### File Operations (`file.go`)

- `ListMatchingFilenames`: Support for glob patterns, directory listing with extension filters.
- Use this for any configuration or variable file discovery.

### String Manipulation (`string.go`, `names.go`)

- `StringPtrEquals`, `ToStringPtr`: Helpers for `*string`.
- `RemoveWhitespace`: Cleans up strings.
- `ParseName`: Splits `provider/resource` strings.
- `SplitNameQualifier`: Splits `name:qualifier` strings.

### Collections (`map.go`, `slice.go`)

- `StringMap`: Custom type with `Keys`, `Equals`, and `Convert` (diffing) methods.
- `MapConvert`, `MapEquals`: Generic map comparison helpers.
- `StringSliceEquals`, `DiffStringSlices`: Helpers for set-based slice comparison.

### General Utils (`util.go`)

- `FixMapKeys`: Normalizes YAML unmarshaled maps to `map[string]any`.
- `Contains`, `ContainsComparable`: Membership checks (prefer standard `slices.Contains` where applicable).
- `SleepWithContext`: Context-aware sleep.
- `SliceElementsEqual`: Unordered equality check for slices.

## Conventions

- **Modern Go**: Always use `any` instead of `interface{}`.
- **Standard Library First**: Before adding a new utility, check if `slices` or `maps` packages in the standard library already provide the functionality.
- Keep utilities generic. Domain-specific logic (e.g., AWS-specific) should generally reside in higher-level packages unless they are fundamental string/data helpers.
- Use generics where appropriate to avoid duplication.
- Ensure all new utilities have corresponding tests in `*_test.go`.
