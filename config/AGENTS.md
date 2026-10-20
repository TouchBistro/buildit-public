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
- **One key per resource**: `Graph` vertices are keyed on `resource.Key()` (`provider::name`, no type).
  `AddVertex` returns an error on a duplicate key rather than overwriting, because an overwritten
  vertex silently drops out of plan/apply/destroy (DEVOPS-8902). `Generate` collects that error
  with the other validation errors so every collision is reported in one run. Use
  `resourceTypeName` (yaml tag of `ResourceSet`) when a message needs to name a resource type
  the way the user wrote it.
- **Two entry points, one pipeline**: `Parse` reads files; `NewInternalConfig(rs ResourceSet,
  providers, globalTags)` takes the same values built in Go (DEVOPS-8933, libbuildit consumers
  such as wafer). Both produce an `InternalConfig` that `Generate` finishes in three steps:
  `mergeConfigs` (tags, audit tag, providers; no AWS), `client.InitAwsProviders` (STS), then
  `buildGraph` (provider/name resolution, tagsFor, Normalize, Validate, overrides, edges). Keep
  that split: new config-shape checks go in `mergeConfigs` so they fail before any AWS call, and
  `config/resource_set_test.go` runs `mergeConfigs` + `buildGraph` to assert the wired graph
  without credentials. `ResourceSet` is exported and its yaml tags are the `resources:` section
  names, so renaming a field or tag changes both the YAML format and the Go API.
- **Plan data vs rendering**: `plan.go` splits planning in two. `Graph.PlanResult` walks the graph,
  calls `Compare`, and returns `[]PlanEntry` with no logging; `planRenderer` turns entries into the
  logrus output users see. Both sit on `planEach`, which hands over each entry as soon as its
  `Compare` returns: `PlanTargets` renders progressively (a long plan must show progress, not
  print in one burst at the end), `PlanResult` collects. Keep it that way
  (DEVOPS-8928): libbuildit consumers such as wafer render plans with their own field set, and a
  `--json` plan needs the entries as values. New plan outcomes are a `PlanAction` plus a renderer
  case, never a `log.*` call inside `PlanResult`. `config/testdata/plan_*.golden` pin the exact
  output; regenerate with `go test ./config -run TestPlanTargetsOutput -update` only when the
  output is meant to change.
- **Interpolation**: Support `${VAR_NAME}` syntax for variable substitution.
- **Provider Management**: Handle multiple AWS provider configurations (regions, profiles, roles).
- **Format Support**: Support multiple config file formats if necessary, but prioritize YAML.
- **Reserved tag keys**: the `buildit:` tag prefix belongs to buildit. `reserved_tags.go` rejects a
  config that sets a key in it, checking the raw parsed config before `Normalize` merges buildit's
  own tags in — after that point a user's key is indistinguishable from ours. New built-in tag keys
  go under that prefix, and `tagsFor` is where per-resource ones are attached. Never write a
  per-resource value into `globalTags` itself: it is shared by every resource, and `buildit audit`
  turns each of its keys into a tag filter.
