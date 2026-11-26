package config

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// Config is the merged, normalized and validated form of a buildit config: the resource
// graph, the global tags and the providers. Parse builds one from files; FromResourceSet builds
// one from values constructed in Go, and the two may be combined (CollectFiles).
type Config struct {
	_config   []builditConfig
	_override *builditConfig

	// generated internal config structures
	providers  map[string]*client.AwsProvider
	graph      *Graph
	globalTags map[string]string
}

// Gr prepares the internal DAG for use by:
//
//	  i- supplying finalized global tags to all internal resources
//	 ii- normalizing all resources
//	iii- performing validations
func (i Config) Graph() *Graph {
	return i.graph
}

func (i Config) Tags() map[string]string {
	return i.globalTags
}

// Providers returns the providers declared in the `providers:` sections of the config
// files. Providers a Go consumer registered with the awesome providers package are not
// included: they are already built, and are found by name through the registry.
func (i Config) Providers() map[string]*client.AwsProvider {
	return i.providers
}

// FromResourceSet builds a Config from values constructed in Go, the way Parse builds one from
// files, so a library consumer can reach Generate's output with no file I/O.
//
// rs and globalTags are what the `resources:` and `globalTags:` sections of a single
// buildit.yml would have parsed to. Providers come from the awesome providers registry,
// where the consumer registers them before calling Generate, or are declared in config
// files merged in with CollectFiles. Call Generate on the result; it still initializes the
// `default` provider when none is registered (which may reach AWS) and normalizes and
// validates every resource exactly as it does for a parsed config.
//
// There is no file content to hash, so the audit tag is derived from an empty SHA list.
// A consumer that wants a meaningful audit value sets `audit` in globalTags, or passes
// RootOptions.NoAuditTags.
func FromResourceSet(rs ResourceSet, globalTags map[string]string) *Config {
	ic := &Config{}
	ic.Collect(builditConfig{
		Resources:  rs,
		GlobalTags: globalTags,
	})
	return ic
}

// Collects the builditConfigs to be included in the internalConfig
func (i *Config) Collect(cfg builditConfig) {
	i._config = append(i._config, cfg)
}

// SetOverride add an override config
func (i *Config) SetOverride(cfg builditConfig) {
	i._override = &cfg
}

// Generate uses the collected buildit configs to generate the final internalConfig
func (i *Config) Generate(ctx context.Context, opts RootOptions) error {
	if err := i.mergeConfigs(opts); err != nil {
		return err
	}

	if err := i.initProviders(ctx); err != nil {
		return errors.Wrapf(err, "error initializing providers")
	}

	return i.buildGraph(ctx)
}

// initProviders is Generate's second pass, the only one that may reach AWS: `default` is
// built unless one is registered, then the providers declared in the config files are
// initialized. Nothing else is written to the awesome registry: `main` resolves through
// client.Provider, and providers a Go consumer registered are found there by name.
func (i *Config) initProviders(ctx context.Context) error {
	if err := client.EnsureDefaultProvider(ctx); err != nil {
		return err
	}
	return client.InitNamedProviders(ctx, i.providers)
}

// mergeConfigs is Generate's first two passes: global tags (plus the audit tag) and
// providers are merged across every collected config and the override is applied to the
// providers. It costs no AWS calls, so a config mistake found here fails before any.
func (i *Config) mergeConfigs(opts RootOptions) error {
	// buildit owns the buildit: tag namespace. globalTags are checked up front because a
	// reserved key here lands on every resource; per-resource tags are checked in pass 3,
	// alongside the rest of each resource's validation.
	if err := i.checkReservedGlobalTags(); err != nil {
		return err
	}

	// pass 1
	// merge global tags
	shas := []string{}
	for _, c := range i._config {
		i.mergeTags(c.GlobalTags)
		shas = append(shas, c.SHA)
	}

	// skip audit tags is --no-audit supplied
	if !opts.NoAuditTags {
		log.Debug("adding audit tags")
		// generate audit tag
		slices.Sort(shas)
		d := []byte(strings.Join(shas, ""))
		hash := fmt.Sprintf("%x", sha256.Sum256(d))

		// attach audit tag
		if i.globalTags == nil {
			i.globalTags = make(map[string]string)
		}

		if _, ok := i.globalTags["audit"]; !ok {
			i.globalTags["audit"] = hash
		}
	}

	// pass 2
	// merge & apply overrides to providers
	for _, c := range i._config {
		if err := i.mergeProviders(c.Providers); err != nil {
			return err
		}
	}
	return i.overrideProviders()
}

// buildGraph is Generate's third pass: every resource in every collected config goes
// through the graphBuilder's steps (provider and name, tags, Normalize, Validate, vertex);
// the override's resources are applied and edges are built from the dependsOn lists.
//
// Each loop below only names the fields of its type the steps work on. A type AWS cannot
// tag passes no globalTags, which is what makes the builder reject tags set on it instead
// of applying buildit's. The steps themselves live in builder.go and nowhere else.
//
// It is separate from Generate so the wiring can be tested without the provider
// initialization that sits between them, which calls STS. It expects mergeConfigs to have
// run: a resource naming a provider that was never merged is rejected here.
//
// Validation failures come back as a ValidationErrors holding every one of them.
func (i *Config) buildGraph(ctx context.Context) error {
	b := &graphBuilder{cfg: i, graph: &Graph{}}

	for _, c := range i._config {
		rs := c.Resources

		for n, r := range rs.BedrockApplicationInferenceProfile {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.Certificate {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.CloudfrontDistribution {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.CloudfrontVpcOrigin {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.CloudfrontFunction {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.CWLogGroup {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.CWMetricAlarm {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.CWSubscriptionFilter {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn}); err != nil {
				return err
			}
		}
		for n, r := range rs.DynamoDB {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.ECSService {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.EFSFileSystem {
			for idx, ap := range r.AccessPoints {
				b.addErr(checkReservedTags("efs-filesystem", fmt.Sprintf("%v: accessPoints[%d]", n, idx), ap.Tags))
			}
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.EventBridgeApiDestination {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn}); err != nil {
				return err
			}
		}
		for n, r := range rs.EventBridgeRule {
			for idx, t := range r.Targets {
				b.addErr(checkReservedTags("eventbridge-rule", fmt.Sprintf("%v: targets[%d]", n, idx), t.Tags))
			}
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.EventBridgeConnection {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn}); err != nil {
				return err
			}
		}
		for n, r := range rs.FirehoseDeliveryStream {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.IAMPolicy {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.IAMRole {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.LambdaFn {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.LambdaLayer {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn}); err != nil {
				return err
			}
		}
		for n, r := range rs.LBTargetGroup {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.LoadBalancer {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.MSKConnector {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.MSKPlugin {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.MSKWorkerConfiguration {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.Route53Record {
			w := wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, legacyDelim: true}
			// A recordName carrying the legacy `provider/` prefix names the provider.
			if r.RecordName != nil && strings.Contains(*r.RecordName, "/") {
				w.providerFrom = *r.RecordName
			}
			if err := admit(ctx, b, n, &r, w); err != nil {
				return err
			}
		}
		for n, r := range rs.S3Bucket {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Bucket, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.SDService {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.SecurityGroup {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.SQSQueue {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags, legacyDelim: true, skipValidate: true}); err != nil {
				return err
			}
		}
		for n, r := range rs.SNSSubscription {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn}); err != nil {
				return err
			}
		}
		for n, r := range rs.StandaloneTask {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.StateMachine {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
		for n, r := range rs.TaskDef {
			if err := admit(ctx, b, n, &r, wiring{name: &r.Name, context: &r.Context, tags: &r.Tags, dependsOn: &r.DependsOn, globalTags: &r.GlobalTags}); err != nil {
				return err
			}
		}
	}

	i.graph = b.graph
	if err := i.overrideSecurityGroups(ctx, b); err != nil {
		return err
	}

	if err := b.build(); err != nil {
		return err
	}

	log.Infof("%v resources read from buildit spec", b.graph.Size())

	return nil
}

// internal helpers

// MergeProviders merges the supplied providers map to the internal providers
func (i *Config) mergeProviders(cfgs map[string]*client.AwsProvider) error {

	// initialize internal providers map
	if i.providers == nil {
		i.providers = make(map[string]*client.AwsProvider)
	}

	if len(i.providers) == 0 {
		maps.Copy(i.providers, cfgs)
		return nil
	}

	for k := range cfgs {
		// check redef
		if _, ok := i.providers[k]; ok {
			return errors.Errorf("provider %q redefined in context", k)
		}
	}
	maps.Copy(i.providers, cfgs)
	return nil
}

// tagsFor returns the tag map for a single resource: the merged global tags plus buildit's
// built-in resource-id tag for this resource.
//
// The copy is not optional. i.globalTags is handed to every resource, and `buildit audit`
// turns each of its keys into a tag filter, so writing a per-resource value into it would
// corrupt both.
func (i Config) tagsFor(id string) map[string]string {
	tags := maps.Clone(i.globalTags)
	if tags == nil {
		// globalTags is only allocated when audit tags are on, so it is nil for a config
		// with no globalTags run under --no-audit.
		tags = make(map[string]string, 1)
	}

	tags[util.BuilditResourceIDTagKey] = util.SafeTagValue(id)

	return tags
}

// MergeTags merges the supplied tags to the internal tags
func (i *Config) mergeTags(tags map[string]string) {
	if len(tags) == 0 {
		return
	}

	if i.globalTags == nil {
		i.globalTags = tags
		return
	}

	maps.Copy(i.globalTags, tags)
}

// getEffectiveProviderName uses various field to return the effective provider & resource identifer
func (i Config) getEffectiveProviderAndId(defined_name string, res_ctx resource.Context) (*string, *string, error) {
	return i.getEffectiveProviderAndIdWithDelim(defined_name, res_ctx, resource.DefaultKeyParseDelim)
}

// getEffectiveProviderName uses various field to return the effective provider & resource identifer
func (i Config) getEffectiveProviderAndIdWithDelim(defined_name string, res_ctx resource.Context, delim string) (*string, *string, error) {
	// check resource name
	var providerFromNameDefined *string
	var providerFromContextDefined *string

	// when defined_name contains the provider name
	if strings.Contains(defined_name, delim) {
		pro, _ := resource.NewKeyWithDelim(delim, defined_name).Split()
		providerFromNameDefined = &pro
	}

	// when context contains the provider name
	if res_ctx.ProviderName != "" {
		providerFromContextDefined = &res_ctx.ProviderName
	}

	// when a provider name is supplied in both places, it must be the same
	if providerFromNameDefined != nil && providerFromContextDefined != nil {
		if *providerFromNameDefined != *providerFromContextDefined {
			return nil, nil, errors.Errorf(
				"ambiguous provider definition for resource %q, both %q & %q found",
				defined_name, *providerFromNameDefined, *providerFromContextDefined)
		}
	}

	prov := util.Coalesce(providerFromNameDefined, util.Coalesce(providerFromContextDefined, client.MainProvider))

	if !i.knownProvider(prov) {
		return nil, nil, errors.Errorf("unknown provider name %q for resource %q", prov, defined_name)
	}

	_, name := resource.NewKeyWithDelim(delim, defined_name).Split()

	return &prov, &name, nil
}

// knownProvider reports whether a resource may name prov: one of the reserved names, a
// provider declared in a config file, or one registered with the awesome providers
// package. Checked while the graph is built, so an unregistered prefix fails before any
// AWS call is made on its behalf.
func (i Config) knownProvider(prov string) bool {
	if prov == client.DefaultProvider || prov == client.MainProvider {
		return true
	}
	if _, ok := i.providers[prov]; ok {
		return true
	}
	_, err := client.Provider(prov)
	return err == nil
}

// helpers to deal with override config

// this section contains all override logic for buildit config.
// currently only `providers` & security-group` resources  support
// override configuration. these objects are referred to as resources
// in the following sections.
//
// all resource types which support config override follow the
// general rule:
//
// [1] when using wildcard '*' matcher as the resource name, the
//     attributes are merged into the respective resource definition.
//
//     NOTE:
//     while the rules for merging are resource type specific, but
//     a general rule-of-thumb is a merge happens only when a non-default or
//     non-zero value for an attribute is supplied in the override config
//
// [2] when using a valid regular expression as the resource name, the
//     override config is merged attribute by attribute for all
//     resources that match that regex
//
// [2] when the regular expression does not match any resources, but satisfies
//     this regex "^[a-ZA-A0-9_/-]+$" i.e. only contains alphanumeric,
//     underscrore '_',  or dash '-' characeters, then a new resource is
//     provisioned  using the supplied override values; In this case all
//     required field are to be supplied, unlike the merge use-cases.

// merge applies the override configuration to the supplied conf using
// the override logic & returns the merged version

// overrideProviders applies override config to providers section
func (i *Config) overrideProviders() error {
	overridePrefix := color.Yellow("** Override **")

	if i._override != nil {
		override := *i._override

		for providerName, o_pr := range override.Providers {
			switch providerName {
			case "*":
				for m_pr_name, m_pr := range i.providers {
					log.WithFields(log.Fields{color.Yellow("pattern"): "*", color.Yellow("provider"): m_pr_name}).Warnf("overriding provider configuration")
					m_pr.Merge(*o_pr)
				}
			default:
				prs, err := findMatchingKeys_String(providerName, i.providers)
				if err != nil {
					return err
				}
				var e_pr *client.AwsProvider
				if len(prs) != 0 {
					for _, pr := range prs {
						var ok bool
						if e_pr, ok = i.providers[pr]; ok {
							log.WithFields(log.Fields{color.Yellow("pattern"): providerName, color.Yellow("matched"): pr}).Warnf("overriding provider configuration")

							e_pr.Merge(*o_pr)
							i.providers[pr] = e_pr
						}
					}
				} else {
					if validResourceIdentifier(providerName) {
						log.WithFields(log.Fields{color.Yellow("pattern"): providerName}).Warnf("adding new provider configuration from overrides")
						e_pr = o_pr
						i.providers[providerName] = e_pr
					} else {
						log.Debugf(overridePrefix+" no match for provider pattern %q, and this is not an allowed resource name, skipping", providerName)
					}
				}
			}
		}
	}
	return nil
}

// overrideSecurityGroups applies the override config's security-group section to
// the graph. A security group the override adds goes through the builder like any
// other resource, so one that fails Validate fails the run (DEVOPS-8901).
func (i *Config) overrideSecurityGroups(ctx context.Context, b *graphBuilder) error {
	ovverridPrefix := color.Yellow("** Override **")

	if i._override != nil {
		override := *i._override

		// loop through
		for securityGroupPattern, o_sg := range override.Resources.SecurityGroup {
			// A merge would strip a reserved key silently, so every pattern is checked;
			// only a new resource is left to the builder, which checks it itself.
			reserved := checkReservedTags("security-group", securityGroupPattern, o_sg.Tags)

			switch securityGroupPattern {

			// matches everything
			case "*":
				b.addErr(reserved)
				for key, m_vertex := range i.graph.vertices {
					if m_sg, ok := m_vertex.resource.(resource.SecurityGroup); ok { // if vertex is a security group
						// log.Infof(ovverridPrefix+" merging security-group configuration for %q, matching ALL `*`", m_sg.Key())
						log.WithFields(log.Fields{color.Yellow("pattern"): "*", color.Yellow("matched"): m_sg.Key()}).Warnf("overriding security group configuration")
						if err := m_sg.Merge(o_sg); err != nil {
							return err
						}
						// m_sg is a copy; without these two writes the merge is a no-op.
						m_vertex.resource = m_sg
						i.graph.depdendencies[key] = m_sg.DependsOn
					}
				}

			// matches patterns
			default:
				matchingKeys, err := findMatchingKeys_Key(securityGroupPattern, i.graph.vertices)
				if err != nil {
					return err
				}

				matched := false
				for _, key := range matchingKeys {
					ver := i.graph.vertices[key]                                      // get the resource for the matching key
					if existing_sg, ok := ver.resource.(resource.SecurityGroup); ok { // if this key is a security group
						// log.Infof(ovverridPrefix+" merging security-group configuration for %q, matching pattern %q", key, securityGroupPattern)
						log.WithFields(log.Fields{color.Yellow("pattern"): securityGroupPattern, color.Yellow("matched"): key}).Warnf("overriding security group configuration")
						if err := existing_sg.Merge(o_sg); err != nil {
							return err
						}
						ver.resource = existing_sg // re-assign it back to the same vertex
						// Edges are built from this snapshot after overrides run, so a
						// dependsOn merged in above is lost unless it is refreshed here.
						i.graph.depdendencies[key] = existing_sg.DependsOn
						matched = true
					}
				}

				if !matched && validResourceIdentifier(securityGroupPattern) {
					log.WithFields(log.Fields{color.Yellow("pattern"): securityGroupPattern}).Warnf("adding new security group configuration from overrides")
					w := wiring{name: &o_sg.Name, context: &o_sg.Context, tags: &o_sg.Tags, dependsOn: &o_sg.DependsOn, globalTags: &o_sg.GlobalTags}
					if err := admit(ctx, b, securityGroupPattern, &o_sg, w); err != nil {
						return err
					}
					continue
				}

				b.addErr(reserved)
				if !matched {
					log.Debugf(ovverridPrefix+" no match for security-group pattern %q, and this is not an allowed resource name, skipping", securityGroupPattern)
				}
			}
		}
	} // if override==nil

	return nil
}

// validResourceIdentifier returns a true, if the supplied
// resource identifier `key` is a valid override key.
func validResourceIdentifier(key string) bool {
	regexpr := `^[a-zA-Z0-9_\-]+$`
	matches, err := regexp.Match(regexpr, []byte(key))
	if err != nil {
		return false
	}
	return matches
}

// findMatchingKeys_String takes a regular expression & a map[string]T where T:any as an input
// and checks if any of the keys match the expression; all those matching keys are
// returned, else a non-nil error if there a problem parsing the regex etc
func findMatchingKeys_String[T any](regex string, target map[string]T) ([]string, error) {
	return util.FindMatchingKeys(regex, target, func(k string) []byte { return []byte(k) })
}

// findMatchingKeys_Key takes a regular expression & a map[Key]T where T:any as an input
// and checks if any of the keys match the expression; all those matching keys are
// returned, else a non-nil error if there a problem parsing the regex etc
func findMatchingKeys_Key[T any](regex string, target map[resource.Key]T) ([]resource.Key, error) {
	return util.FindMatchingKeys(regex, target, func(k resource.Key) []byte { return []byte(k.String()) })
}
