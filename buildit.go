// Package buildit is the library form of the buildit CLI: a Go program builds a
// ResourceSet, hands it to NewConfig with tags, and gets back the same validated resource
// graph that `buildit plan` builds from a buildit.yml.
//
// Providers are awesome providers.CredsProvider values, and the awesome registry is the
// only place buildit looks for them: a program builds the providers its resources name
// with providers.New*CredsProvider, which registers them, before calling NewConfig, and
// NewConfig rejects a resource naming one that is not registered. Two names are reserved.
// `default` is the credentials the program runs with: a registered one is used as is, and
// when none is registered NewConfig builds one from the AWS default credentials chain in
// us-east-1, validated with STS. `main` provisions the resources that name no provider,
// and resolves to the registered `main` if there is one, else to `default`. The registry
// is process-wide, so configs that need different `default`s must not be built or applied
// at the same time.
//
//	_, err := providers.NewDefaultCredsProvider(ctx, providers.DefaultCredsProviderName,
//		providers.WithRegion("us-east-1"), providers.ValidateProvider())
//	...
//	other, err := providers.NewAssumeRoleCredsProvider(ctx, "example-other", ...)
//	...
//	var rs buildit.ResourceSet
//	err = rs.AddCWLogGroup("example-logs", resource.CWLogGroup{Retention: aws.Int32(30)})
//	err = rs.AddCWLogGroupWithProvider(other, "example-logs", resource.CWLogGroup{})
//	...
//	cfg, err := buildit.NewConfig(ctx, rs, buildit.WithGlobalTag("team", "example-team"))
//	...
//	err = cfg.Graph().Apply(ctx)
//
// Config files are a further source: WithConfigPath and the WithVariable / WithTemplates
// options render and merge buildit.yml files into the same Config, with the resources
// added in Go. A provider declared in a config file replaces a registered one of the same
// name, with a warning.
package buildit

import (
	"context"

	"github.com/TouchBistro/buildit/config"
	"github.com/TouchBistro/buildit/util"
	"github.com/pkg/errors"
)

// Config is the merged, normalized and validated buildit config. Its Graph carries the
// Plan, Apply and Destroy operations.
type Config = config.Config

// ResourceSet holds the resources of a config, one map per resource type. Fill it with
// the AddX and AddXWithProvider methods; the exported maps are the YAML form and remain
// available for a consumer that wants to build them directly.
type ResourceSet = config.ResourceSet

// Graph is the dependency graph of a Config's resources.
type Graph = config.Graph

// NewConfig builds and validates a Config from rs and the options: config files (if any)
// are rendered and merged in, `default` is built unless one is registered, the providers
// declared in the files are initialized, and every resource is given its provider and
// tags, normalized and validated, and wired into the graph. Any problem found on the way
// is returned, so a Config that comes back is ready to plan or apply.
//
// Providers come from the awesome providers registry; see the package documentation for
// the reserved names. A resource naming a provider that is not registered (and not
// declared in a config file) is rejected before any AWS call is made on its behalf.
func NewConfig(ctx context.Context, rs ResourceSet, opts ...Option) (*Config, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if err := o.validate(); err != nil {
		return nil, err
	}

	cfg := config.NewConfig(rs, o.globalTags)
	if len(o.root.ConfigPath) > 0 {
		tokens, err := util.LoadVariables(o.root.VariablesPath, o.root.UseEnvars, o.root.Variables...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load variables")
		}
		if err := cfg.CollectFiles(o.root, tokens); err != nil {
			return nil, errors.Wrap(err, "failed to read config files")
		}
	}

	if err := cfg.Generate(ctx, o.root); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ContextWithRetryPolicy returns a context whose AWS API calls retry up to retries
// attempts, with at most backoffSeconds between attempts. Pass it to the Graph's Plan,
// Apply and Destroy; without it they use client.DefaultRetries and client.DefaultBackoff.
func ContextWithRetryPolicy(ctx context.Context, retries, backoffSeconds int) context.Context {
	ctx = context.WithValue(ctx, util.RETRY, retries)
	return context.WithValue(ctx, util.BACKOFF, backoffSeconds)
}
