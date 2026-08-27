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

// internal provider is a data structure to hold
// internal representation of buildit config.
type InternalConfig struct {
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
func (i InternalConfig) Graph() *Graph {
	return i.graph
}

func (i InternalConfig) Tags() map[string]string {
	return i.globalTags
}

// Pr returns the internal providers map
func (i InternalConfig) Providers() map[string]*client.AwsProvider {
	return i.providers
}

// Collects the builditConfigs to be included in the internalConfig
func (i *InternalConfig) Collect(cfg builditConfig) {
	i._config = append(i._config, cfg)
}

// SetOverride add an override config
func (i *InternalConfig) SetOverride(cfg builditConfig) {
	i._override = &cfg
}

// Generate uses the collected buildit configs to generate the final internalConfig
func (i *InternalConfig) Generate(ctx context.Context, opts RootOptions) error {
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
	// merge, apply overrides & initialize providers
	for _, c := range i._config {
		if err := i.mergeProviders(c.Providers); err != nil {
			return err
		}
	}
	if err := i.overrideProviders(); err != nil {
		return err
	}

	// initalizing providers
	err := client.InitAwsProviders(ctx, opts.DefaultProvider, i.providers)
	if err != nil {
		return errors.Wrapf(err, "error initilizing providers")
	}

	// pass 3
	// merge buildit configs
	graph := &Graph{}
	var validationErrs []error

	// The tag checks below collect rather than return, so one run reports every problem in
	// the config instead of stopping at the first.
	addErr := func(err error) {
		if err != nil {
			validationErrs = append(validationErrs, err)
		}
	}

	for _, c := range i._config {

		for n, r := range c.Resources.BedrockApplicationInferenceProfile {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("bedrock-application-inference-profile", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("bedrock-application-inference-profile", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.Certificate {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.DomainName = *eId
			r.Context.ProviderName = *ePr
			// r.DomainName = n
			addErr(checkReservedTags("certificate", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("certificate", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.CloudfrontDistribution {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("cloudfront-distribution", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("cloudfront-distribution", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.CloudfrontVpcOrigin {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("cloudfront-vpc-origin", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("cloudfront-vpc-origin", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.CloudfrontFunction {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("cloudfront-function", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("cloudfront-function", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.CWLogGroup {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("cloudwatch-loggroup", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("cloudwatch-loggroup", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.CWMetricAlarm {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("cloudwatch-metricalarm", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("cloudwatch-metricalarm", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.CWSubscriptionFilter {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}
			r.Name = *eId
			r.Context.ProviderName = *ePr
			r.Normalize(ctx)
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.DynamoDB {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("dynamodb-table", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("dynamodb-table", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.ECSService {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("ecs-service", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("ecs-service", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.EFSFileSystem {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("efs-filesystem", n, r.Tags))
			for idx, ap := range r.AccessPoints {
				addErr(checkReservedTags("efs-filesystem", fmt.Sprintf("%v: accessPoints[%d]", n, idx), ap.Tags))
			}
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("efs-filesystem", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.EventBridgeApiDestination {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			r.Normalize(ctx)
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.EventBridgeRule {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("eventbridge-rule", n, r.Tags))
			for idx, t := range r.Targets {
				addErr(checkReservedTags("eventbridge-rule", fmt.Sprintf("%v: targets[%d]", n, idx), t.Tags))
			}
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("eventbridge-rule", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.EventBridgeConnection {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			// The reserved namespace is policed here even though this type's tags go
			// nowhere: CreateConnection takes none, so there is no tagsFor wiring and no
			// applied-check either. See DEVOPS-8900.
			addErr(checkReservedTags("eventbridge-connection", n, r.Tags))
			r.Normalize(ctx)
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.FirehoseDeliveryStream {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("firehose-delivery-stream", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("firehose-delivery-stream", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.IAMPolicy {

			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("iam-policy", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("iam-policy", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.IAMRole {

			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("iam-role", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("iam-role", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.LambdaFn {

			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("lambda-function", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("lambda-function", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.LambdaLayer {

			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("lambda-layer", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.LBTargetGroup {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("lb-targetgroup", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("lb-targetgroup", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.LoadBalancer {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("load-balancer", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("load-balancer", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.MSKConnector {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("msk-connector", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("msk-connector", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.MSKPlugin {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("msk-plugin", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("msk-plugin", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.MSKWorkerConfiguration {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("msk-worker-configuration", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("msk-worker-configuration", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.Route53Record {

			// this is legacy; backward compatibility
			// if we find a "::" in the resource defintion name then use that to infer provier/id, else use legacy '/'
			delim := "::"
			if index := strings.Index(n, delim); index == -1 {
				delim = "/"
			}

			ePr, eId, err := i.getEffectiveProviderAndIdWithDelim(n, r.Context, delim)
			if err != nil {
				return err
			}

			// if this r53 record has a recordName field that also contains a legacy '/' provider delim, we
			// have to fetch the provider from here instead.
			if r.RecordName != nil && strings.Contains(*r.RecordName, "/") {
				ePr, _, err = i.getEffectiveProviderAndIdWithDelim(*r.RecordName, r.Context, "/")
				if err != nil {
					return err
				}
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			r.Normalize(ctx)
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.S3Bucket {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Bucket = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("s3-bucket", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("s3-bucket", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.SDService {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("sd-service", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("sd-service", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.SecurityGroup {

			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("security-group", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("security-group", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.SQSQueue {

			// this is legacy; backward compatibility
			// if we find a "::" in the resource defintion name then use that to infer provier/id, else use legacy '/'
			delim := "::"
			if index := strings.Index(n, delim); index == -1 {
				delim = "/"
			}

			// TODO this is to support existing /
			ePr, eId, err := i.getEffectiveProviderAndIdWithDelim(n, r.Context, delim)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("sqs-queue", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("sqs-queue", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.SNSSubscription {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			r.Normalize(ctx)
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.StandaloneTask {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("standalone-task", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("standalone-task", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.StateMachine {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("state-machine", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("state-machine", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}

		for n, r := range c.Resources.TaskDef {
			ePr, eId, err := i.getEffectiveProviderAndId(n, r.Context)
			if err != nil {
				return err
			}

			r.Name = *eId
			r.Context.ProviderName = *ePr
			addErr(checkReservedTags("taskdef", n, r.Tags))
			r.GlobalTags = i.tagsFor(*eId)
			r.Normalize(ctx)
			addErr(checkBuilditTagsApplied("taskdef", n, r.Tags))
			if err := r.Validate(ctx); err != nil {
				validationErrs = append(validationErrs, err)
				continue
			}
			graph.AddVertex(r, r.DependsOn)
		}
	}

	i.graph = graph
	// apply security group
	if err := i.overrideSecurityGroups(ctx, validationErrs); err != nil {
		return err
	}

	// Display all validation errors
	// This way we can show all errors to the user, instead of
	// exiting after the first one
	if validationErrs != nil {
		for _, err := range validationErrs {
			if verr, ok := err.(*resource.ValidationError); ok {
				log.Errorf("%v (%v)", verr.ResourceIdentifier, verr.ResourceType)
				for _, err := range verr.Messages {
					log.Errorf("\t %v\n", err)
				}
			} else {
				log.Errorf("\t %v\n", err)
			}
		}
		return errors.New("One or more resources failed validation")
	}

	for to, deps := range graph.depdendencies {
		for _, from := range deps {
			err := i.graph.AddEdge(from, to)
			if err != nil {
				return err
			}
		}
	}

	log.Infof("%v resources read from buildit spec", graph.Size())

	return nil
}

// internal helpers

// MergeProviders merges the supplied providers map to the internal providers
func (i *InternalConfig) mergeProviders(providers map[string]*client.AwsProvider) error {

	// initialize internal providers map
	if i.providers == nil {
		i.providers = make(map[string]*client.AwsProvider)
	}

	if len(i.providers) == 0 {
		maps.Copy(i.providers, providers)
		return nil
	}

	for k := range providers {
		// check redef
		if _, ok := i.providers[k]; ok {
			return errors.Errorf("provider %q redefined in context", k)
		}
	}
	maps.Copy(i.providers, providers)
	return nil
}

// tagsFor returns the tag map for a single resource: the merged global tags plus buildit's
// built-in resource-id tag for this resource.
//
// The copy is not optional. i.globalTags is handed to every resource, and `buildit audit`
// turns each of its keys into a tag filter, so writing a per-resource value into it would
// corrupt both.
func (i InternalConfig) tagsFor(id string) map[string]string {
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
func (i *InternalConfig) mergeTags(tags map[string]string) {
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
func (i InternalConfig) getEffectiveProviderAndId(defined_name string, res_ctx resource.Context) (*string, *string, error) {
	return i.getEffectiveProviderAndIdWithDelim(defined_name, res_ctx, resource.DefaultKeyParseDelim)
}

// getEffectiveProviderName uses various field to return the effective provider & resource identifer
func (i InternalConfig) getEffectiveProviderAndIdWithDelim(defined_name string, res_ctx resource.Context, delim string) (*string, *string, error) {
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

	if _, ok := i.providers[prov]; !ok && prov != client.DefaultProvider && prov != client.MainProvider {
		return nil, nil, errors.Errorf("unknown provider name %q for resource %q", prov, defined_name)
	}

	_, name := resource.NewKeyWithDelim(delim, defined_name).Split()

	return &prov, &name, nil
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
func (i *InternalConfig) overrideProviders() error {
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

// overrideSecurityGroups applies override config to security groups
func (i *InternalConfig) overrideSecurityGroups(ctx context.Context, validationErrs []error) error {
	ovverridPrefix := color.Yellow("** Override **")

	if i._override != nil {
		override := *i._override

		// loop through
		for securityGroupPattern, o_sg := range override.Resources.SecurityGroup {
			switch securityGroupPattern {

			// matches everything
			case "*":
				for _, m_vertex := range i.graph.vertices {
					if m_sg, ok := m_vertex.resource.(resource.SecurityGroup); ok { // if vertex is a security group
						// log.Infof(ovverridPrefix+" merging security-group configuration for %q, matching ALL `*`", m_sg.Key())
						log.WithFields(log.Fields{color.Yellow("pattern"): "*", color.Yellow("matched"): m_sg.Key()}).Warnf("overriding security group configuration")
						if err := m_sg.Merge(o_sg); err != nil {
							return err
						}
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
						matched = true
					}
				}

				if !matched {
					if validResourceIdentifier(securityGroupPattern) {

						// TODO this is a terrible repetition of code.
						// need to do something with this pattern (e.g.
						// introduce & implement a ConfigurableResource interface???)
						log.WithFields(log.Fields{color.Yellow("pattern"): securityGroupPattern}).Warnf("adding new security griuop configuration from overrides")

						ePr, eId, err := i.getEffectiveProviderAndId(securityGroupPattern, o_sg.Context)
						if err != nil {
							return err
						}

						o_sg.Name = *eId
						o_sg.Context.ProviderName = *ePr
						// No reserved-tag check here: this method takes validationErrs by
						// value, so anything appended is discarded (DEVOPS-8901). The
						// override's globalTags are still checked up front, and
						// ResourceTags.Merge strips reserved keys regardless, so a bad key
						// cannot reach AWS — it just goes unreported.
						o_sg.GlobalTags = i.tagsFor(*eId)
						o_sg.Normalize(ctx)
						if err := o_sg.Validate(ctx); err != nil {
							validationErrs = append(validationErrs, err)
							continue
						}
						i.graph.AddVertex(o_sg, o_sg.DependsOn)
					} else {
						log.Debugf(ovverridPrefix+" no match for security-group pattern %q, and this is not an allowed resource name, skipping", securityGroupPattern)
					}
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
