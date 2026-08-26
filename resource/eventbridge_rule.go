package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	eventbridgetypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

// EventBridgeRule represents an EventBridge rule; only Cron (Schedule) rules are supported
type EventBridgeRule struct {
	BaseResource `yaml:",inline"`
	Name               string               `yaml:"-"`
	Description        string               `yaml:"description"`
	EventBusName       *string              `yaml:"eventbusName"`
	ScheduleExpression *string              `yaml:"scheduleExpression,omitempty"`
	EventPattern       *string              `yaml:"eventPattern,omitempty"`
	Enabled            *bool                `yaml:"enabled"`
	Targets            []*EventBridgeTarget `yaml:"targets"`
	GlobalTags         map[string]string    `yaml:"-"`
	Tags               map[string]string    `yaml:"tags"`
	DependsOn          []Key                `yaml:"dependsOn"`
}

// Key returns the unique key for the resource for this buildit context
func (r EventBridgeRule) Key() Key {
	return NewKey(r.Context.ProviderName, r.Identifier())
}

// Identifier returns name of the eventbridge rule
func (r EventBridgeRule) Identifier() string {
	return r.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (r *EventBridgeRule) Normalize(ctx context.Context) {

	if r.EventBusName == nil || len(*r.EventBusName) == 0 {
		r.EventBusName = aws.String("default")
	}

	if r.Enabled == nil {
		r.Enabled = aws.Bool(true)
	}

	// Merge globalTags to Target Group tags, if key is not already present
	// later we'll use s.Tags to add/update tags
	if r.Tags == nil {
		r.Tags = make(map[string]string)
	}
	ResourceTags(r.Tags).Merge(r.GlobalTags)

	// set up the eventbridge targets..
	for _, target := range r.Targets {
		target.RuleName = r.Name
		target.EventBusName = *r.EventBusName
		target.Normalize(ctx)
	}
}

// Validate checks that the input provided is correct
func (r EventBridgeRule) Validate(ctx context.Context) error {

	var errMessages []string

	// schedule or event pattern supplied
	if r.ScheduleExpression != nil && r.EventPattern != nil {
		errMessages = append(errMessages, "cannot supply both a schedule & event pattern for the rule")
	}

	for _, target := range r.Targets {
		err := target.Validate(ctx, r.Context)
		if err != nil {
			vErr, ok := err.(*ValidationError)
			if ok {
				errMessages = append(errMessages, vErr.Messages...)
			}
		}
	}
	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: r.Identifier(),
		ResourceType:       "EventBridgeRule",
		Messages:           errMessages,
	}
}

// Apply creates a new eventbridge rule
func (r EventBridgeRule) Apply(ctx context.Context) error {

	log.Debugf("creating eventbridge rule %v", r.Name)

	diffs, err := r.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", r.Identifier()).Info("eventbridge rule already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}

		err = r.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update eventbridge rule %v", r.Identifier())
		}
		return nil
	}

	return r.apply(ctx)
}

// Destroy removes the eventbridge rule
func (r EventBridgeRule) Destroy(ctx context.Context) error {

	log.Debugf("destroying evemtbridge rule %v", r.Identifier())

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding eventbridge rule %v", r.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("eventbridge rule does not exist, nothing to destroy, skippping ")
		return nil
	}

	ebrClient := client.EventBridge(ctx, r.Context.ProviderName)
	err = r.destroyExistingTargets(ctx)

	if err != nil {
		return errors.Wrapf(err, "error deleting targets for eventbridge rule %v", r.Identifier())
	}

	//now delete the rule
	_, err = ebrClient.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
		Force:        false, //never force delete rules
		EventBusName: r.EventBusName,
		Name:         aws.String(r.Name),
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting event bridge rule %v", r.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Red("eventbridge rule destroyed"))

	return nil
}

// eventbridge rule diff
type EventBridgeRuleDiff struct {
	BaseResourceDiff

	descriptionDiff        bool
	eventbusDiff           bool
	scheduleExpressionDiff bool
	eventPatternDiff       bool
	enableddiff            bool
	targetDiff             bool
	tagsDiff               bool
	tagDiff                util.TagDiffResult
}

// Compare fetches the existing eventbridge rule & if it exists, also fetches the targets defined for it,
// and checks if it is equal to the corresponding eventbridge rule & targets, else returns the diffs
func (r EventBridgeRule) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching existing eventbridge rule %v", r.Identifier())
	}

	diffs := &EventBridgeRuleDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "eventbridge rule does not exist")
		return diffs, nil
	}

	diffs.Resource = existing
	diffsFound := false

	existingDescription := ""
	if existing.Description != nil {
		existingDescription = *existing.Description
	}
	if r.Description != existingDescription {
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("eventbridge rule '%v' description is not the same", r.Identifier()))
		diffs.descriptionDiff = true
		diffsFound = true
	}

	if *r.EventBusName != *existing.EventBusName {
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("eventbridge rule '%v' event bus is not the same", r.Identifier()))
		diffs.eventbusDiff = true
		diffsFound = true
	}

	if strings.TrimSpace(util.Coalesce(r.ScheduleExpression, "")) != strings.TrimSpace(util.Coalesce(existing.ScheduleExpression, "")) {
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("eventbridge rule '%v' schedule expression is not the same %v -> %v", r.Identifier(),
			util.Coalesce(existing.ScheduleExpression, ""), util.Coalesce(r.ScheduleExpression, "")))
		diffs.scheduleExpressionDiff = true
		diffsFound = true
	}

	if strings.TrimSpace(util.Coalesce(r.EventPattern, "")) != strings.TrimSpace(util.Coalesce(existing.EventPattern, "")) {
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("eventbridge rule '%v' event pattern  is not the same %v -> %v", r.Identifier(),
			util.Coalesce(existing.EventPattern, ""), util.Coalesce(r.EventPattern, "")))
		diffs.eventPatternDiff = true
		diffsFound = true
	}

	existingRuleEnabled := existing.State == eventbridgetypes.RuleStateEnabled
	if *r.Enabled != existingRuleEnabled {
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("eventbridge rule '%v' enable setting is not the same", r.Identifier()))
		diffs.enableddiff = true
		diffsFound = true
	}

	//targets

	for _, target := range r.Targets {

		existingTarget, err := target.fetchExisting(ctx, r.Context)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot fetch existing rule")
		}
		//if target not found
		if existingTarget == nil {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("existing eventbridge rule '%v' has no target ID %v defined", r.Identifier(), target.ID))
			diffs.targetDiff = true
			diffsFound = true
			break
		}

		eq, msgs, err := target.equals(ctx, r.Context, existingTarget)
		if err != nil {
			return nil, err
		}

		if !eq {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("eventbridge rule '%v' target %v is not equal to existing target definition", r.Identifier(), target.ID))
			diffs.Messages = append(diffs.Messages, msgs...)
			diffs.targetDiff = true
			diffsFound = true
			break
		}
	}

	//when no diff's found, check if there were targets previously added, but removed in new list...
	if !diffs.targetDiff {

		ebrClient := client.EventBridge(ctx, r.Context.ProviderName)
		out, err := ebrClient.ListTargetsByRule(ctx, &eventbridge.ListTargetsByRuleInput{
			EventBusName: r.EventBusName,
			Rule:         aws.String(r.Name),
		})

		if err != nil {
			return nil, errors.Wrapf(err, "error fetching targets for eventbridge rule %v", r.Identifier())
		}

		if len(out.Targets) != len(r.Targets) {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf(
				"eventbridge rule '%v' number of targets defined(%v) is not the same as existing (%v)", r.Identifier(), len(r.Targets), len(out.Targets)))
			diffs.targetDiff = true
			diffsFound = true
		}
	}

	//tags
	awsTags, err := awsw.NewEventBridge(ctx, r.Context.ProviderName).GetResourceTags(ctx, *existing.Arn)
	if err != nil {
		return nil, err
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, r.Tags); tagDiff.HasChanges() {
		diffsFound = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if !diffsFound {
		return nil, nil
	}

	return diffs, nil
}

// fetchExisting returns the existing eventbridge rule details if found
func (r EventBridgeRule) fetchExisting(ctx context.Context) (*eventbridgetypes.Rule, error) {
	ebrClient := client.EventBridge(ctx, r.Context.ProviderName)
	out, err := ebrClient.ListRules(ctx, &eventbridge.ListRulesInput{
		NamePrefix:   aws.String(r.Name),
		EventBusName: r.EventBusName,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error while listing event bridge rules")
	}

	if len(out.Rules) == 0 {
		return nil, nil
	}

	for _, rule := range out.Rules {
		if r.Name == *rule.Name {
			return &rule, nil
		}
	}

	return nil, nil
}

// apply provisions a new eventbridge rule & it's targets
func (r EventBridgeRule) apply(ctx context.Context) error {

	err := r.applyRule(ctx)
	if err != nil {
		return errors.Wrapf(err, "error creating or updating eventbridge rule %v", r.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Green("eventbridge rule created"))

	// Targets
	err = r.applyTargets(ctx)
	if err != nil {
		return errors.Wrapf(err, "error adding targets to eventbridge rule %v", r.Identifier())
	}
	return nil
}

// applyDiffs applies diffs to an existing eventbridge rule & it's targets
func (r EventBridgeRule) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info(color.Green("no updates required for eventbridge rule"))
	}

	ebDiffs, ok := diffs.(*EventBridgeRuleDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	//fetch existing eventbridge
	existing, ok := ebDiffs.Resource.(*eventbridgetypes.Rule)
	if !ok {
		return errors.Errorf("cannot retrieve existing eventbridge rule")
	}

	var err error

	// rule
	ruleDiff := ebDiffs.descriptionDiff || ebDiffs.eventbusDiff || ebDiffs.scheduleExpressionDiff || ebDiffs.eventPatternDiff || ebDiffs.enableddiff
	if ruleDiff {
		err = r.applyRule(ctx)
		if err != nil {
			return errors.Wrapf(err, "error updating rule %v", r.Identifier())
		}
		log.WithField("Name", r.Identifier()).Info(color.Yellow("eventbridge rule updated"))
	}

	// tags
	if ebDiffs.tagsDiff {
		upserts := ebDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			err = awsw.NewEventBridge(ctx, r.Context.ProviderName).AddResourceTags(ctx, *existing.Arn, upserts)
			if err != nil {
				return errors.Wrapf(err, "error updating eventbridge rule tags for %v", r.Identifier())
			}
		}

		if len(ebDiffs.tagDiff.Deleted) > 0 {
			err = awsw.NewEventBridge(ctx, r.Context.ProviderName).DeleteResourceTags(ctx, *existing.Arn, ebDiffs.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error deleting eventbridge rule tags for %v", r.Identifier())
			}
		}
	}

	// targets
	if ebDiffs.targetDiff {
		//remove and delete all targets
		err := r.destroyExistingTargets(ctx)
		if err != nil {
			return errors.Wrapf(err, "error removing targets from eventbridge rule %v", r.Identifier())
		}
		//add targets
		err = r.applyTargets(ctx)
		if err != nil {
			return errors.Wrapf(err, "error adding targets to eventbridge rule %v", r.Identifier())
		}
	}

	return nil
}

// applyRule uses the put-rule api to add/update the rule.
func (r EventBridgeRule) applyRule(ctx context.Context) error {

	//Tags
	tags := toEventBridgeTag(r.Tags)

	var roleArn *string //TODO: @esiddiqui not used... whoa ?

	ruleState := eventbridgetypes.RuleStateEnabled
	if !*r.Enabled {
		ruleState = eventbridgetypes.RuleStateDisabled
	}

	ebrClient := client.EventBridge(ctx, r.Context.ProviderName)
	_, err := ebrClient.PutRule(ctx, &eventbridge.PutRuleInput{
		Name:               aws.String(r.Name),
		Description:        aws.String(r.Description),
		EventBusName:       r.EventBusName,
		ScheduleExpression: r.ScheduleExpression,
		EventPattern:       r.EventPattern,
		RoleArn:            roleArn,
		State:              ruleState,
		Tags:               tags,
	})

	if err != nil {
		return errors.Wrapf(err, "error creating event bridge rule %v", r.Identifier())
	}
	return nil
}

// applyTargets goes through the list of targets for this rule and adds/updates them.
func (r EventBridgeRule) applyTargets(ctx context.Context) error {
	for _, target := range r.Targets {
		target.EventBusName = *r.EventBusName
		target.RuleName = r.Name
		target.Tags = InheritedTags(r.Tags)
		target.Normalize(ctx)
		err := target.Apply(ctx, r.Context)
		if err != nil {
			return errors.Wrapf(err, "error adding target %v to eventbridge rule %v", target.ID, r.Identifier())
		}
	}
	return nil
}

// destroyExistingTargets will remove all existing targets
func (r EventBridgeRule) destroyExistingTargets(ctx context.Context) error {

	ebrClient := client.EventBridge(ctx, r.Context.ProviderName)
	out, err := ebrClient.ListTargetsByRule(ctx, &eventbridge.ListTargetsByRuleInput{
		EventBusName: r.EventBusName,
		Rule:         aws.String(r.Name),
	})

	if err != nil {
		return errors.Wrapf(err, "error fetching targets for eventbridge rule %v", r.Identifier())
	}

	ids := make([]string, 0)
	for _, target := range out.Targets {
		ids = append(ids, *target.Id)
	}

	if len(ids) > 0 {
		_, err = ebrClient.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
			Rule:         aws.String(r.Identifier()),
			EventBusName: aws.String(*r.EventBusName),
			Ids:          ids,
		})

		if err != nil {
			return errors.Wrapf(err, "error deleting existing targets for eventbridge rule %v", r.Identifier())
		}
	}

	log.WithFields(log.Fields{
		"Rule":        r.Name,
		"Num Targets": len(ids),
	}).Info(color.Red("eventbridge targets destroyed"))

	return nil
}

// toEventBridgeTag converts a map[string]string to an array of *eventbridge.Tag
func toEventBridgeTag(tags map[string]string) []eventbridgetypes.Tag {
	var ebTags []eventbridgetypes.Tag
	for k, v := range tags {
		ebTags = append(ebTags, eventbridgetypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	return ebTags
}
