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
- **Two entry points, one pipeline**: `Parse` reads files; `FromResourceSet(rs, globalTags)`
  takes the same values built in Go (DEVOPS-8933, DEVOPS-9105; libbuildit consumers such as
  wafer, via the root `buildit` package). `CollectFiles` is `Parse` without `Generate`, so the two
  sources merge. Both produce a `Config` that `Generate` finishes in three steps: `mergeConfigs`
  (tags, audit tag, file-declared providers; no AWS), `initProviders` (`default` unless one is
  registered, then the file-declared providers; STS), then `buildGraph` (provider/name
  resolution, tagsFor, Normalize, Validate, overrides, edges). Keep that split: new config-shape
  checks go in `mergeConfigs` so they fail before any AWS call, and `config/resource_set_test.go`
  runs `mergeConfigs` + `buildGraph` to assert the wired graph without credentials. `ResourceSet`
  is exported and its yaml tags are the `resources:` section names, so renaming a field or tag
  changes both the YAML format and the Go API. Every resource type must have an `AddX` /
  `AddXWithProvider` pair in `resource_set.go` (`TestResourceSetAddMethodsCoverEveryType` fails
  otherwise).
- **One implementation of the per-resource steps** (DEVOPS-8930): `graphBuilder` in `builder.go`
  resolves provider and name, checks reserved (or unsupported) tags, applies `tagsFor`, calls
  `Normalize` (an error is collected as `failed normalizing <type> "<name>"` and skips the
  resource), checks buildit's tags landed, calls `Validate`, and adds the vertex. Every source
  goes through `admit`: `buildGraph`'s loop per type and an override-added security group. A new
  resource type is one loop in `buildGraph` naming its fields in a `wiring`; leave `globalTags`
  out for a type AWS cannot tag, which is what switches the builder from applying tags to
  rejecting them. Never repeat the steps inline. Nested sub-resource tags (EFS access points,
  EventBridge targets) are checked in the type's loop before `admit`. `sqs-queue` sets
  `skipValidate` because its `Validate` was never called at load; turning it on is a behaviour
  change for existing configs. Resource types live in buildit: there is no way to add a type
  from another module to a `ResourceSet`.
- **Validation failures are a value**: `buildGraph` returns `ValidationErrors` (every failure,
  `errors.As`-reachable) and logs nothing. `cmd/root.go` prints `Lines()` once and keeps the
  one-line summary as the fatal error, so CLI output is unchanged. Do not log a failure where it
  is collected. `Generate` also rejects a dependency cycle (in `graphBuilder.build`, once the
  edges are in), so the CLI and library callers of `config.FromResourceSet` + `Generate` or
  `buildit.NewConfig` all get it; `buildit validate` has no check of its own.
- **Providers come from the awesome registry only.** The config holds nothing but the
  `providers:` entries of the files it read; everything else is resolved by name through
  `client.Provider`, which maps `main` to the registered `main` or else `default`, and buildit
  never writes `default` (unless none is registered) or `main` to the registry. `knownProvider`
  accepts the reserved names, a file-declared provider, or any registered name, so an
  unregistered prefix fails in `buildGraph` with `unknown provider name` before any AWS call.
  `AddXWithProvider` checks with `client.CheckProviderRegistered` that the provider it was handed
  is the one registered under its name, not an older object a later build replaced. The CLI
  builds `default` from its flags in `cmd/root.go` before `Parse`; a library consumer registers
  its own, or gets one from the AWS default credentials chain. Tests that need a `default`
  register an unvalidated `providers.NewDefaultCredsProvider` under
  `providers.DefaultCredsProviderName`; the registry is process-wide and a name cannot be
  unregistered, so order such tests within one function.
- **Plan data vs rendering**: `plan.go` splits planning in two. `Graph.PlanResult` walks the graph,
  calls `Compare`, and returns `[]PlanEntry` with no logging; `planRenderer` turns entries into the
  logrus output users see. Both sit on `planEach`, which hands over each entry as soon as its
  `Compare` returns: `PlanTargets` renders progressively (a long plan must show progress, not
  print in one burst at the end), `PlanResult` collects. Keep it that way
  (DEVOPS-8928): libbuildit consumers such as wafer render plans with their own field set, and a
  `--json` plan needs the entries as values. New plan outcomes are a `PlanAction` plus a renderer
  case, never a `log.*` call inside `PlanResult`. `PlanEntry.Changes` carries a diff's structured
  `[]resource.Change` when the resource's diff implements `resource.DetailedResourceDiff`
  (DEVOPS-8929); it is nil for resources that have not adopted it, so consumers fall back to
  `Differences`. `config/testdata/plan_*.golden` pin the exact
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
