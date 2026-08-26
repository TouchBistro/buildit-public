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
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/aws/aws-sdk-go-v2/service/sfn/types"
	"github.com/pkg/errors"

	// "github.com/aws/aws-sdk-go-v2/aws"
	// "github.com/aws/aws-sdk-go-v2/service/sfn"

	log "github.com/sirupsen/logrus"
)

const (
	HighestPublishedStateMachineVersionKey string = "$highest"
)

// StateMachineAlias represents a state machine alias.

type StateMachineAlias struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Version     string  `yaml:"version"`
	Arn         *string `yaml:"-"` // read from AWS
}

// equals returns true if this alias is equal to the supplied other
func (r StateMachineAlias) equals(other StateMachineAlias) bool {
	return r.Name == other.Name && r.Description == other.Description && r.Version == other.Version
}

// StateMachine represents a state machine resource
type StateMachine struct {
	BaseResource `yaml:",inline"`
	Description *string             `yaml:"description"`
	Definition  string              `yaml:"definition"`
	Role        *string             `yaml:"role"`
	Publish     *bool               `yaml:"publish"`
	DependsOn   []Key               `yaml:"dependsOn"`
	Aliases     []StateMachineAlias `yaml:"aliases"`
	Tags        map[string]string   `yaml:"tags"`
	Name        string              `yaml:"-"`
	GlobalTags  map[string]string   `yaml:"-"`
	Arn         *string             `yaml:"-"` // read from AWS
	Versions    []string            `yaml:"-"` // read from AWS
}

// Key returns the unique key for the resource for this buildit context
func (r StateMachine) Key() Key {
	return NewKey(r.Context.ProviderName, r.Identifier())
}

// Identifier returns the unique id for the resource
func (r StateMachine) Identifier() string {
	return r.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields
func (r *StateMachine) Normalize(ctx context.Context) {

	// Merge globalTags to state machine tags, if key is not already present
	// later we'll use sg.Tags to add/update tags
	if r.Tags == nil {
		r.Tags = make(map[string]string)
	}
	ResourceTags(r.Tags).Merge(r.GlobalTags)

	// default publish = false
	if r.Publish == nil {
		r.Publish = aws.Bool(false)
	}
}

// Validate the state machine input
func (r StateMachine) Validate(ctx context.Context) error {

	var errorMsgs []string

	if r.Identifier() == "" {
		errorMsgs = append(errorMsgs, "state machine name cannot be empty")
	}

	if errorMsgs == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: r.Identifier(),
		ResourceType:       "state machine",
		Messages:           errorMsgs,
	}
}

// Apply builds the state machine
func (r StateMachine) Apply(ctx context.Context) error {

	log.Debugf("creating state machine %v", r.Identifier())

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
		log.WithField("Name", r.Identifier()).Info("state machine already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = r.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update state machine %v", r.Identifier())
		}
		return nil
	}

	return r.apply(ctx)
}

// Destroy the state machine
func (r StateMachine) Destroy(ctx context.Context) error {

	log.Debugf("destroying state machine %v", r.Identifier())

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding state machine %v", r.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("state machine does not exist, nothing to destroy, skippping ")
		return nil
	}

	client := client.SFN(ctx, r.Context.ProviderName)
	_, err = client.DeleteStateMachine(ctx, &sfn.DeleteStateMachineInput{
		StateMachineArn: existing.Arn,
	})

	if err != nil {
		return errors.Wrapf(err, "error destorying state machine %v", r.Name)
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Red("state machine destroyed"))
	return nil
}

// StateMachineDiff respresnts diffs between state machine definition & AWS representation
type StateMachineDiff struct {
	BaseResourceDiff

	diff            bool
	definitionDiff  bool
	roleDiff        bool
	publishRequired bool
	// aliases
	aliasesDiff     bool
	aliasesToAdd    []StateMachineAlias
	aliasesToRemove []StateMachineAlias
	aliasesToUpdate map[StateMachineAlias]StateMachineAlias

	// private diff details
	// TODO: other fields here
	tagsDiff bool
	tagDiff  util.TagDiffResult
}

// Compare fetches the existing state machine and if it exists, checks if this
// resource is equal to the corresponding AWS state machine
func (r StateMachine) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", r.Identifier())
	}

	diffs := &StateMachineDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "state machine does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	// publish
	if util.CoalesceComparable(r.Publish, false) {
		diffs.diff = true
		diffs.publishRequired = true
		diffs.Messages = append(diffs.Messages, "state machine new version may be published")
	}

	// definition
	defi := util.RemoveWhitespace(r.Definition)
	exi := util.RemoveWhitespace(existing.Definition)
	if strings.TrimSpace(defi) != strings.TrimSpace(exi) {
		diffs.diff = true
		diffs.definitionDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("state machine definition has changed %v -> %v", exi, defi))
	}

	// role
	if util.Coalesce(r.Role, "") != util.Coalesce(existing.Role, "") {
		diffs.diff = true
		diffs.roleDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("state machine role has changed %v -> %v",
			util.Coalesce(existing.Role, ""), util.Coalesce(r.Role, "")))
	}

	// alias
	//aliasesToAdd := make(map[string]StateMachineAlias)
	//var aliasesToRemove []StateMachineAlias
	//aliasesToUpdate := make(map[StateMachineAlias]StateMachineAlias)
	var aliasesToAdd []StateMachineAlias
	var aliasesToRemove []StateMachineAlias
	aliasesToUpdate := make(map[StateMachineAlias]StateMachineAlias)

	// assume all defined aliases need to be added
	amap := make(map[string]StateMachineAlias)
	for _, def := range r.Aliases {
		amap[def.Name] = def
	}
	// loop through existing aliases
	for _, exi := range existing.Aliases {
		if def, ok := amap[exi.Name]; ok { // if existing alias is being added
			if !def.equals(exi) { // existing & defined are not the same, need an update this guy
				aliasesToUpdate[def] = exi // def <- exi
				diffs.diff = true
				diffs.aliasesDiff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("alias %v to be updated", exi.Name))
			}
			delete(amap, exi.Name) // eitherway not to be added
		} else { // existing alias is not in the add list, delete em
			aliasesToRemove = append(aliasesToRemove, exi)
			diffs.diff = true
			diffs.aliasesDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("alias %v to be removed", exi.Name))
		}
	}

	for _, v := range amap {
		aliasesToAdd = append(aliasesToAdd, v)
		diffs.diff = true
		diffs.aliasesDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("alias %v to be added", v.Name))
	}

	if diffs.aliasesDiff {
		diffs.aliasesToAdd = aliasesToAdd
		diffs.aliasesToRemove = aliasesToRemove
		diffs.aliasesToUpdate = aliasesToUpdate
	}

	// tags
	if tagDiff := TagDiffForContext(ctx, existing.Tags, r.Tags); tagDiff.HasChanges() {
		diffs.diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(existing.Tags, diffs.tagDiff)...)
	}

	if !diffs.diff {
		return nil, nil
	}

	// return
	return diffs, nil
}

// fetchExisting fetches state machine if exists
func (r StateMachine) fetchExisting(ctx context.Context) (*StateMachine, error) {

	client := client.SFN(ctx, r.Context.ProviderName)
	arn, err := awsw.NewSFN(ctx, r.Context.ProviderName).StateMachineArnForName(ctx, r.Identifier())
	if err != nil {
		// don't throw the error when the state machine doesn't even exist
		if arn == nil {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error fetching state machine for name %v", r.Identifier())
	}

	out, err := client.DescribeStateMachine(ctx, &sfn.DescribeStateMachineInput{
		StateMachineArn: arn,
	})

	if err != nil {
		return nil, err
	}

	// version
	versions, err := awsw.NewSFN(ctx, r.Context.ProviderName).StateMachineVersionsArnsForArn(ctx, *arn)
	if err != nil {
		return nil, err
	}

	var role *string
	if out.RoleArn != nil {
		role, err = awsw.NewIAM(ctx, r.Context.ProviderName).RoleNameForArn(ctx, *out.RoleArn)
		if err != nil {
			return nil, errors.Wrapf(err, "error fetching role name for state machine %v", r.Identifier())
		}
	}

	// aliases
	// TODO: use pagination to capture all aliases
	var aliases []StateMachineAlias
	out2, err := client.ListStateMachineAliases(ctx, &sfn.ListStateMachineAliasesInput{
		StateMachineArn: out.StateMachineArn,
	})
	if err != nil {
		return nil, err
	}

	for _, al := range out2.StateMachineAliases {
		out3, err := client.DescribeStateMachineAlias(ctx, &sfn.DescribeStateMachineAliasInput{
			StateMachineAliasArn: al.StateMachineAliasArn,
		})
		if err != nil {
			return nil, err
		}

		ver := "1" // assume its' ver=1
		if len(out3.RoutingConfiguration) > 0 {
			seg := strings.Split(*out3.RoutingConfiguration[0].StateMachineVersionArn, ":")
			ver = seg[len(seg)-1]
		}
		aliases = append(aliases, StateMachineAlias{
			Name:        *out3.Name,
			Description: util.Coalesce(out3.Description, ""),
			Version:     ver,
			Arn:         out3.StateMachineAliasArn,
		})
	}

	// tags
	tags, err := awsw.NewSFN(ctx, r.Context.ProviderName).GetResourceTags(ctx, *out.StateMachineArn)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching tags for state machine %v", r.Identifier())
	}

	sm := &StateMachine{
		Name:        *out.Name,
		Definition:  *out.Definition,
		Description: out.Description,
		Role:        role,
		Tags:        tags,
		Arn:         out.StateMachineArn,
		Aliases:     aliases,
		Versions:    versions,
	}

	return sm, nil
}

// apply provisions a new state machine
func (r StateMachine) apply(ctx context.Context) error {

	var role *string
	var err error

	if r.Role != nil {
		role, err = awsw.NewIAM(ctx, r.Context.ProviderName).RoleArnForName(ctx, *r.Role)
		if err != nil {
			return errors.Wrapf(err, "error fetching role for name %v", *r.Role)
		}
	}

	var description *string
	if util.CoalesceComparable(r.Publish, false) {
		description = r.Description
	}

	var tags []types.Tag
	for k, v := range r.Tags {
		tags = append(tags, types.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	client := client.SFN(ctx, r.Context.ProviderName)
	out, err := client.CreateStateMachine(ctx, &sfn.CreateStateMachineInput{
		Name:               aws.String(r.Identifier()),
		Definition:         aws.String(r.Definition),
		RoleArn:            role,
		Type:               types.StateMachineTypeStandard, //only supported
		Publish:            *r.Publish,
		VersionDescription: description, // only non-nil, when publishing a new version
		Tags:               tags,
	})

	if err != nil {
		return errors.Wrapf(err, "error creating a new state machine %v", r.Identifier())
	}

	log.WithFields(log.Fields{
		"Name":    r.Identifier(),
		"Arn":     out.StateMachineArn,
		"Version": util.Coalesce(out.StateMachineVersionArn, "not published"),
	}).Info(color.Green("state machine created"))

	// wait for the creation
	// TODO: if required, any wait code here...

	highestVersionArn := out.StateMachineVersionArn // t
	err = r.CreateAliases(ctx, r.Aliases, out.StateMachineArn, highestVersionArn)
	if err != nil {
		return err
	}

	return nil
}

// applyDiffs applies diffs to an existing state machine
func (r StateMachine) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("no updates required for state machine")
		return nil
	}

	resDiffs, ok := diffs.(*StateMachineDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing sg
	existing, ok := resDiffs.Resource.(*StateMachine)
	if !ok {
		return errors.Errorf("cannot retrieve existing state machine")
	}

	var configDiff bool

	highestVersionArn := ""
	if len(existing.Versions) > 0 {
		highestVersionArn = existing.Versions[0] // highest version
	}

	input := &sfn.UpdateStateMachineInput{
		StateMachineArn: existing.Arn,
	}

	// definition
	if resDiffs.definitionDiff {
		configDiff = true
		input.Definition = aws.String(r.Definition)
	}

	if resDiffs.roleDiff {
		configDiff = true
		var arn *string // empty out role arn
		var err error
		if r.Role != nil {
			// if non-nil role supplied, then get the arn for it
			arn, err = awsw.NewIAM(ctx, r.Context.ProviderName).RoleArnForName(ctx, *r.Role)
			if err != nil {
				return errors.Wrapf(err, "error fetching role arn for role %v", *r.Role)
			}
		}
		input.RoleArn = arn
	}

	// publish & version description
	if util.CoalesceComparable(r.Publish, false) {
		configDiff = true
		input.Publish = *r.Publish
		input.Definition = aws.String(r.Definition) // to force a publish
		input.VersionDescription = r.Description
	}

	// var err error
	_ = existing
	client := client.SFN(ctx, r.Context.ProviderName)

	if configDiff {

		out, err := client.UpdateStateMachine(ctx, input)
		if err != nil {
			return errors.Wrapf(err, "error updating state machine")
		}

		if util.CoalesceComparable(r.Publish, false) {
			highestVersionArn = *out.StateMachineVersionArn // update the version arn if we have a new highest
		}

		log.WithFields(log.Fields{
			"Name": r.Identifier(),
			"Arn":  *out.StateMachineVersionArn,
		}).Info(color.Yellow("state machine updated"))

	}

	// tags
	if resDiffs.tagsDiff {
		upserts := resDiffs.tagDiff.Upserts()

		if len(upserts) > 0 {
			// add tag
			err := awsw.NewSFN(ctx, r.Context.ProviderName).AddResourceTags(ctx, *existing.Arn, upserts)
			if err != nil {
				return errors.Wrapf(err, "error tagging sfn resource")
			}
		}

		if len(resDiffs.tagDiff.Deleted) > 0 {
			// remove tag
			err := awsw.NewSFN(ctx, r.Context.ProviderName).DeleteResourceTags(ctx, *existing.Arn, resDiffs.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error un-tagging sfn resource")
			}
		}
	}

	// aliases
	if resDiffs.aliasesDiff {

		if len(resDiffs.aliasesToRemove) > 0 {
			err := r.DeleteAliases(ctx, resDiffs.aliasesToRemove)
			if err != nil {
				return err
			}
		}

		if len(resDiffs.aliasesToUpdate) > 0 {
			err := r.UpdateAliases(ctx, resDiffs.aliasesToUpdate, existing.Arn, &highestVersionArn)
			if err != nil {
				return err
			}
		}

		if len(resDiffs.aliasesToAdd) > 0 {
			err := r.CreateAliases(ctx, resDiffs.aliasesToAdd, existing.Arn, &highestVersionArn)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// CreateAliases creates the supplied list of aliases for the state machine; if any of the alias definitions
// reference $HIGHEST as the version qualifier, the supplied highestVersionArn is used instad of the version string
func (r StateMachine) CreateAliases(ctx context.Context, aliases []StateMachineAlias, arn, highestVersionArn *string) error {

	if arn == nil {
		return errors.Errorf("invliad or nil state function arn supplied")
	}

	client := client.SFN(ctx, r.Context.ProviderName)

	for _, alias := range aliases {

		name := alias.Name
		versionArn := fmt.Sprintf("%v:%v", *arn, alias.Version) // assuming regular numeric version
		if strings.ToLower(alias.Version) == HighestPublishedStateMachineVersionKey {
			if highestVersionArn == nil || len(*highestVersionArn) == 0 {
				return errors.Errorf("invliad or nil highest version arn supplied")
			}
			versionArn = *highestVersionArn
		}

		out2, err := client.CreateStateMachineAlias(ctx, &sfn.CreateStateMachineAliasInput{
			Name:        aws.String(name),
			Description: aws.String(alias.Description),
			RoutingConfiguration: []types.RoutingConfigurationListItem{{
				StateMachineVersionArn: aws.String(versionArn),
				Weight:                 100, // static
			},
			},
		})

		if err != nil {
			return errors.Wrapf(err, "error creating an alias for %v", r.Name)
		}

		log.WithFields(log.Fields{
			"Name":  r.Identifier(),
			"Alias": name,
			"Arn":   *out2.StateMachineAliasArn,
		}).Info(color.Green("state machine alias created"))
	}

	return nil
}

// DeleteAliases destroys the supplied list of supplied aliases from the state machine; the StateMachineAlias must have an arn field
// filled
func (r StateMachine) DeleteAliases(ctx context.Context, aliases []StateMachineAlias) error {

	client := client.SFN(ctx, r.Context.ProviderName)

	for _, alias := range aliases {

		name := alias.Name
		_, err := client.DeleteStateMachineAlias(ctx, &sfn.DeleteStateMachineAliasInput{
			StateMachineAliasArn: alias.Arn,
		})

		if err != nil {
			return errors.Wrapf(err, "error destroying an state machine aliase %v", alias.Name)
		}

		log.WithFields(log.Fields{
			"Name":  r.Identifier(),
			"Alias": name,
		}).Info(color.Red("state machine alias destoryed"))
	}

	return nil
}

// UpdateAliases updates all aliases supplied in the map[defined] -> existing for the
func (r StateMachine) UpdateAliases(ctx context.Context, aliases map[StateMachineAlias]StateMachineAlias, arn, highestVersionArn *string) error {

	if arn == nil {
		return errors.Errorf("invliad or nil state function arn supplied")
	}

	client := client.SFN(ctx, r.Context.ProviderName)
	for def, exi := range aliases {

		name := def.Name
		versionArn := fmt.Sprintf("%v:%v", *arn, def.Version) // assuming regular numeric version
		if strings.ToLower(def.Version) == HighestPublishedStateMachineVersionKey {
			if highestVersionArn == nil || len(*highestVersionArn) == 0 {
				return errors.Errorf("invliad or nil highest version arn supplied")
			}
			versionArn = *highestVersionArn
		}
		_, err := client.UpdateStateMachineAlias(ctx, &sfn.UpdateStateMachineAliasInput{
			StateMachineAliasArn: exi.Arn,
			Description:          &def.Description,
			RoutingConfiguration: []types.RoutingConfigurationListItem{
				{
					StateMachineVersionArn: aws.String(versionArn),
					Weight:                 100,
				},
			},
		})

		if err != nil {
			return errors.Wrapf(err, "error updating an alias %v for state machine %v", def.Name, arn)
		}

		log.WithFields(log.Fields{
			"Name":  r.Identifier(),
			"Alias": name,
		}).Info(color.Yellow("state machine alias updated"))
	}

	return nil
}
