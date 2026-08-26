package resource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	//OneHour is number of seconds in 1 hour
	OneHour = 3600
	//TwelveHours is number of seconds in 12 hours
	TwelveHours = 43200
)

// IAMRole represents an IAMRole
type IAMRole struct {
	BaseResource `yaml:",inline"`
	Name               string            `yaml:"-"`
	Description        string            `yaml:"description"`
	MaxSessionDuration int32             `yaml:"maxSessionDuration"`
	Path               string            `yaml:"path"`
	TrustPolicy        IAMPolicyDocument `yaml:"trustPolicy"`
	Permissions        []string          `yaml:"permissions"`
	DependsOn          []Key             `yaml:"dependsOn"`
	Tags               map[string]string `yaml:"tags"`
	GlobalTags         map[string]string `yaml:"-"`
}

// Key returns the unique key for the resource for this buildit context
func (r IAMRole) Key() Key {
	return NewKey(r.Context.ProviderName, r.Identifier())
}

// Identifier returns the unique identifier
func (r IAMRole) Identifier() string {
	return r.Name
}

// TODO(@maintainer): Does it make sense to have this a separate method or should
// we combine it into Validate?

// Normalize will perform any necessary validations to the IAM role and set any default values.
func (r *IAMRole) Normalize(ctx context.Context) {
	r.TrustPolicy.fixMapKeys()

	//if maxSessionDuration is not provided, ignore & set to default 1 hour
	if r.MaxSessionDuration == 0 {
		r.MaxSessionDuration = OneHour
	}

	//Merge profile to IAM role tags, if key is not already present
	//later we'll use sg.Tags to add/update tags
	if r.Tags == nil {
		r.Tags = make(map[string]string)
	}

	ResourceTags(r.Tags).Merge(r.GlobalTags)
}

// Validate checks that the IAM role has a valid configuration.
// If the configuration is invalid an error will be returned
// that contains the details on what was invalid.
func (r IAMRole) Validate(ctx context.Context) error {
	var errMessages []string

	if r.MaxSessionDuration < OneHour || r.MaxSessionDuration > TwelveHours {
		errMessages = append(errMessages, "maxSessionDuration must be set between 3600 & 43200")
	}

	// Make sure the policy names follow the correct naming format
	// The names should not start or end with a slash
	for _, p := range r.Permissions {
		if p[0] == '/' {
			msg := fmt.Sprintf("permission name cannot start with a slash: %q", p)
			errMessages = append(errMessages, msg)
		}

		if p[len(p)-1] == '/' {
			msg := fmt.Sprintf("permission name cannot end with a slash: %q", p)
			errMessages = append(errMessages, msg)
		}
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: r.Identifier(),
		ResourceType:       "IAM Role",
		Messages:           errMessages,
	}
}

// Apply provisions the IAM role
func (r IAMRole) Apply(ctx context.Context) error {
	log.Debugf("creating iam role %v", r.Identifier())

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
		log.WithField("Name", r.Identifier()).Info("iam role already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = r.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update iam role %v", r.Identifier())
		}
		return nil
	}

	return r.apply(ctx)
}

// Destroy deletes the IAM role on AWS.
func (r IAMRole) Destroy(ctx context.Context) error {
	log.WithField(
		"Name", r.Identifier(),
	).Debug("Checking if iam role exists")

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to check if iam role %s exists", r.Identifier())
	}

	if existing == nil {
		log.WithField("Name", r.Identifier()).Info("iam role does not exist, nothing to destroy")
		return nil
	}

	log.WithField("Name", r.Identifier()).Debug("Destroying iam role")

	iamClient := client.IAM(ctx, r.Context.ProviderName)

	// We need to detach all policies from the role before we can delete it
	attachedPolicies, err := awsw.NewIAM(ctx, r.Context.ProviderName).ListAttachedPoliciesByRole(ctx, *existing.RoleName)
	if err != nil {
		return errors.Wrapf(err, "failed to get policies attached to role %s", r.Identifier())
	}

	for _, ap := range attachedPolicies {
		_, err = iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
			RoleName:  existing.RoleName,
			PolicyArn: ap.PolicyArn,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to detach policy %s from role %s", *ap.PolicyArn, r.Identifier())
		}
	}

	_, err = iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
		RoleName: existing.RoleName,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to delete IAM role %s", r.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
		"ARN":  *existing.Arn,
	}).Info(color.Red("iam role destroyed"))

	return nil
}

type IAMRoleDiff struct {
	BaseResourceDiff

	pathDiff           bool
	descDiff           bool
	maxSessionDiff     bool
	trustPolicyDiff    bool
	policiesDiff       bool
	policyNamesToAdd   []string
	policyArnsToDelete []string
	tagsDiff           bool
	tagDiff            util.TagDiffResult
}

// Compare fetches the existing iam role if it exists, checks if this
// resource definition is equal to the corresponding AWS IAMRole
// returns any diffs if it isn't
func (r IAMRole) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return nil, fmt.Errorf("error fetch aws resource for %v", r.Identifier())
	}
	diffs := &IAMRoleDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "iam role does not exists")
		return diffs, nil
	}

	diff := false
	diffs.Resource = existing

	// path
	if r.Path != *existing.Path {
		diff = true
		diffs.pathDiff = true
		diffs.Messages = append(diffs.Messages, "iam role path is not the same")
	}

	// description
	if r.Description != util.Coalesce(existing.Description, "") {
		diff = true
		diffs.descDiff = true
		diffs.Messages = append(diffs.Messages, "iam role description is not the same")
	}

	// max session duration
	if r.MaxSessionDuration != util.CoalesceComparable(existing.MaxSessionDuration, 0) {
		diff = true
		diffs.descDiff = true
		diffs.Messages = append(diffs.Messages, "iam role max session duration is not the same")
	}

	// trust policy
	existingTrustPolicy, err := decodePolicyDocument(*existing.AssumeRolePolicyDocument)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode trust policy document: %s", r.Identifier())
	}

	if r.TrustPolicy.equals(&existingTrustPolicy) != Equal {
		diff = true
		diffs.trustPolicyDiff = true
		// diffs.Messages = append(diffs.Messages, fmt.Sprintf("iam role trust policy is not the same \n%v -> %v\n", existingTrustPolicy, r.TrustPolicy))
		diffs.Messages = append(diffs.Messages, "iam role trust policy is not the same")
	}

	// policies attached
	attachedPolicies, err := awsw.NewIAM(ctx, r.Context.ProviderName).ListAttachedPoliciesByRole(ctx, *existing.RoleName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get policies attached to role %s", r.Identifier())
	}
	existingPolicies := make(map[string]iamtypes.AttachedPolicy)
	for _, ap := range attachedPolicies {
		existingPolicies[*ap.PolicyName] = ap
	}

	for _, perm := range r.Permissions {
		p := IAMPolicy{Name: perm}
		_, name := p.PathAndName()

		if _, ok := existingPolicies[name]; ok {
			delete(existingPolicies, name)
			continue
		}

		diff = true
		diffs.policiesDiff = true
		diffs.policyNamesToAdd = append(diffs.policyNamesToAdd, p.Name)
	}

	if len(diffs.policyNamesToAdd) > 0 {
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v policy attachments are missing", len(diffs.policyNamesToAdd)))
	}

	for _, ep := range existingPolicies {
		diff = true
		diffs.policiesDiff = true
		diffs.policyArnsToDelete = append(diffs.policyArnsToDelete, *ep.PolicyArn)
	}

	if len(diffs.policyArnsToDelete) > 0 {
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v policy attachments were removed", len(diffs.policyArnsToDelete)))
	}

	// tags
	existingTags := make(map[string]string)
	for _, tg := range existing.Tags {
		existingTags[*tg.Key] = *tg.Value
	}

	if tagDiff := TagDiffForContext(ctx, existingTags, r.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(existingTags, diffs.tagDiff)...)
	}

	if diff {
		return diffs, nil
	}

	return nil, nil
}

// fetchExisting fetches the matching IAM role from AWS.
// If an error occurs a non-nil error is returned. If the role
// is not found both the role and error will be nil.
func (r IAMRole) fetchExisting(ctx context.Context) (*iamtypes.Role, error) {
	iamClient := client.IAM(ctx, r.Context.ProviderName)
	respGetRole, err := iamClient.GetRole(ctx, &iam.GetRoleInput{
		RoleName: &r.Name,
	})
	if err != nil {
		// The AWS SDK returns an error if no role was found
		var noSuchEntity *iamtypes.NoSuchEntityException
		if errors.As(err, &noSuchEntity) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to look up iam role %s", r.Identifier())
	}
	return respGetRole.Role, nil
}

// Apply provisions the IAM role
func (r IAMRole) apply(ctx context.Context) error {

	iamClient := client.IAM(ctx, r.Context.ProviderName)

	//TODOL @esiddiqui make this map[string]string to []iam/types/Tag a helper func
	var tags []iamtypes.Tag
	for k, v := range r.Tags {
		tags = append(tags, iamtypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	//TODO: @esiddiqui make this marshalling/unmarshalling a helper func
	trustPolicyJSON, err := json.Marshal(r.TrustPolicy)
	if err != nil {
		return errors.Wrapf(err, "failed to encode trust policy document as JSON: %s", r.Identifier())
	}

	respCreateRole, err := iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		AssumeRolePolicyDocument: aws.String(string(trustPolicyJSON)),
		Description:              aws.String(r.Description),
		RoleName:                 aws.String(r.Name),
		Path:                     aws.String(r.Path),
		MaxSessionDuration:       aws.Int32(r.MaxSessionDuration),
		PermissionsBoundary:      nil, //TODO:@esiddiqui add support for boundary
		Tags:                     tags,
	})
	if err != nil {
		return errors.Wrapf(err, "error creating IAM role %s", r.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
		"ARN":  *respCreateRole.Role.Arn,
	}).Info(color.Green("iam role created"))

	// attach policies
	for _, p := range r.Permissions {
		awsPolicy, err := IAMPolicy{
			BaseResource: r.BaseResource, // enforce the same as role
			Name:         p}.fetchExisting(ctx)
		if err != nil {
			return errors.Wrapf(err, "error looking up policy %s", p)
		}
		if awsPolicy == nil {
			return errors.Errorf("IAM policy does not exist %s", p)
		}

		_, err = iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
			RoleName:  aws.String(r.Name),
			PolicyArn: aws.String(awsPolicy.Arn),
		})
		if err != nil {
			return errors.Wrapf(err, "error attaching policy %s to %s role", p, r.Identifier())
		}

		log.WithFields(log.Fields{
			"Policy": p,
			"Role":   r.Identifier(),
		}).Info(color.Green("iam policy attached to role"))
	}

	return nil
}

// applyDiffs updates the resource using the diffs supplied or returns an error
func (r IAMRole) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("no updates required for iam role")
		return nil
	}

	roleDiffs, ok := diffs.(*IAMRoleDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing iam policy
	existing, ok := roleDiffs.Resource.(*iamtypes.Role)
	if !ok {
		return errors.Errorf("cannot retrieve existing iam policy")
	}

	var err error

	// path (invalid)
	if roleDiffs.pathDiff {
		return errors.New("invalid diff, iam role path cannot be updated")
	}

	updateInput := &iam.UpdateRoleInput{
		RoleName: existing.RoleName,
	}

	// description
	if roleDiffs.descDiff {
		updateInput.Description = aws.String(r.Description)
	}

	// max session duration
	if roleDiffs.maxSessionDiff {
		updateInput.MaxSessionDuration = aws.Int32(r.MaxSessionDuration)
	}

	iamClient := client.IAM(ctx, r.Context.ProviderName)

	if roleDiffs.descDiff || roleDiffs.maxSessionDiff {
		_, err = iamClient.UpdateRole(ctx, updateInput)
		if err != nil {
			return errors.Wrapf(err, "failed to update IAM role %s", r.Identifier())
		}
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
			"ARN":  *existing.Arn,
		}).Info(color.Yellow("iam role updated"))
	}

	// trust policy
	if roleDiffs.trustPolicyDiff {
		policyJSON, err := json.Marshal(r.TrustPolicy)
		if err != nil {
			return errors.Wrapf(err, "failed to encode trust policy document as JSON: %s", r.Identifier())
		}
		_, err = iamClient.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{
			RoleName:       existing.RoleName,
			PolicyDocument: aws.String(string(policyJSON)),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update trust policy document for role %s", r.Identifier())
		}
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
			"ARN":  *existing.Arn,
		}).Info(color.Yellow("iam role trust policy updated"))
	}

	// attach new policies
	for _, polName := range roleDiffs.policyNamesToAdd {
		log.Debugf("attaching policy %v to role %v", polName, r.Identifier())
		policy, err := IAMPolicy{
			BaseResource: r.BaseResource, // enforce the same as Role
			Name:         polName}.fetchExisting(ctx)
		if err != nil {
			return errors.Wrapf(err, "error looking up policy %s", polName)
		}
		if policy == nil {
			return errors.Errorf("IAM policy does not exist %s", polName)
		}

		_, err = iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
			RoleName:  existing.RoleName,
			PolicyArn: aws.String(policy.Arn),
		})

		if err != nil {
			return errors.Wrapf(err, "error attaching policy %s to role %s", policy.Arn, r.Identifier())
		}
	}
	if len(roleDiffs.policyNamesToAdd) > 0 {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
			"ARN":  *existing.Arn,
		}).Info(color.Green(fmt.Sprintf("%v policies attached to role", len(roleDiffs.policyNamesToAdd))))
	}

	// remove not-required policies
	for _, arn := range roleDiffs.policyArnsToDelete {
		_, err = iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
			RoleName:  existing.RoleName,
			PolicyArn: aws.String(arn),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to detach policy %s from role %s", arn, r.Identifier())
		}
	}
	if len(roleDiffs.policyArnsToDelete) > 0 {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
			"ARN":  *existing.Arn,
		}).Info(color.Red(fmt.Sprintf("%v policies detached from role", len(roleDiffs.policyArnsToDelete))))
	}

	// tags
	if roleDiffs.tagsDiff {
		upserts := roleDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			err = awsw.NewIAM(ctx, r.Context.ProviderName).AddRoleTags(ctx, *existing.RoleName, upserts)
			if err != nil {
				return errors.Wrapf(err, "error updating iam role tags for %v", r.Identifier())
			}
		}
		if len(roleDiffs.tagDiff.Deleted) > 0 {
			err = awsw.NewIAM(ctx, r.Context.ProviderName).DeleteRoleTags(ctx, *existing.RoleName, roleDiffs.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error deleting iam role tags for %v", r.Identifier())
			}
		}
	}

	return nil
}
