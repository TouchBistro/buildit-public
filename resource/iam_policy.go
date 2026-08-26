package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

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

// The IAM Policy type defs were generated based on:
// https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_grammar.html

// IAMPolicyStatement represents a single statement in an IAM policy document.
// It defines the permissions.
type IAMPolicyStatement struct {
	Sid          string `yaml:"Sid" json:"Sid,omitempty"`
	Effect       string `yaml:"Effect" json:"Effect"`
	Principal    any    `yaml:"Principal" json:"Principal,omitempty"` // We can enforce stricter schema later if we want
	NotPrincipal any    `yaml:"NotPrincipal" json:"NotPrincipal,omitempty"`
	Action       any    `yaml:"Action" json:"Action,omitempty"`
	NotAction    any    `yaml:"NotAction" json:"NotAction,omitempty"`
	Resource     any    `yaml:"Resource" json:"Resource,omitempty"`
	NotResource  any    `yaml:"NotResource" json:"NotResource,omitempty"`
	Condition    any    `yaml:"Condition" json:"Condition,omitempty"`
}

// IAMPolicyDocument represents a IAM policy JSON document.
type IAMPolicyDocument struct {
	Version   string               `yaml:"Version" json:"Version,omitempty"`
	ID        string               `yaml:"Id" json:"Id,omitempty"`
	Statement []IAMPolicyStatement `yaml:"Statement" json:"Statement"`
}

// IAMPolicy represents an IAM Policy
type IAMPolicy struct {
	BaseResource `yaml:",inline"`
	Name             string            `yaml:"-"` // Name consists of the path and policy name. ex: buildit/resource-policy
	Description      string            `yaml:"description"`
	Policy           IAMPolicyDocument `yaml:"policy"` // Policy is the policy document represented as a JSON object
	DependsOn        []Key             `yaml:"dependsOn"`
	Tags             map[string]string `yaml:"tags"`
	GlobalTags       map[string]string `yaml:"-"`
	Arn              string            `yaml:"-"` // Arn is never un-marshalled from YAML, only read from AWS for existing objects
	VersionId        string            `yaml:"-"` // VersionId is never un-marshalled from YAML, Only read from AWS for existing objects
	DefaultVersionId string            `yaml:"-"` // DefaultVersionId is never un-marshalled from YAML, only read from AWS for existing objects
}

// Key returns the unique key for the resource for this buildit context
func (p IAMPolicy) Key() Key {
	return NewKey(p.Context.ProviderName, p.Identifier())
}

// Identifier returns the unique identifier
func (p IAMPolicy) Identifier() string {
	return p.Name
}

// PathAndName returns the path and name components of the IAM Policy.
// The path is a sequence of identifiers separated by slashes. It begins
// and ends with a slash.
// If there is no path, '/' will be returned.
func (p IAMPolicy) PathAndName() (string, string) {
	i := strings.LastIndexByte(p.Name, '/')

	// If no / found that means we only have the policy name, no path
	if i == -1 {
		return "/", p.Name
	}

	// Path must start and end with a slash
	return "/" + p.Name[:i+1], p.Name[i+1:]
}

// Normalize will perform any necessary validations to the IAM policy and set any default values.
func (p *IAMPolicy) Normalize(ctx context.Context) {

	p.Policy.fixMapKeys()

	//Merge globalTags to IAM policy tags, if key is not already present
	//later we'll use sg.Tags to add/update tags
	if p.Tags == nil {
		p.Tags = make(map[string]string)
	}
	ResourceTags(p.Tags).Merge(p.GlobalTags)
}

// Validate checks that the IAM policy has a valid configuration.
// If the configuration is invalid an error will be returned
// that contains the details on what was invalid.
func (p IAMPolicy) Validate(ctx context.Context) error {
	var errMessages []string

	// Name should not start or end with a slash
	if p.Name[0] == '/' {
		errMessages = append(errMessages, "policy identifier cannot start with a slash")
	}

	if p.Name[len(p.Name)-1] == '/' {
		errMessages = append(errMessages, "policy identifier cannot end with a slash")
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: p.Identifier(),
		ResourceType:       "IAM Policy",
		Messages:           errMessages,
	}
}

// Apply provisions the IAM policy
func (p IAMPolicy) Apply(ctx context.Context) error {
	log.Debugf("creating iam policy %v", p.Identifier())

	diffs, err := p.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": p.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", p.Identifier()).Info("iam policy already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = p.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update iam policy %v", p.Identifier())
		}
		return nil
	}

	return p.apply(ctx)
}

// Destroy deletes the IAM policy on AWS.
// To ensure the policy object is deleted, first all non-default version Ids are fetched.
// if any existed (due to manually or non-buildit changes) they are deleted.
// next, the policy along with the default version of policy document is deleted
func (p IAMPolicy) Destroy(ctx context.Context) error {
	log.WithField("Name", p.Identifier()).Debug("Checking if iam policy exists")
	awsPolicy, err := p.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to check if iam policy %s exists", p.Identifier())
	}

	if awsPolicy == nil {
		log.WithField("Name", p.Identifier()).Info("iam policy does not exist, nothing to destroy")
		return nil
	}

	log.WithField("Name", p.Identifier()).Debug("Destroying iam policy")

	// delete non-default versions if they exist
	err = deleteNotDefaultPolicyVersions(ctx, awsPolicy.Arn, awsPolicy.DefaultVersionId, p.Context.ProviderName)
	if err != nil {
		return errors.Wrapf(err, "erro deleting non-default policy versions for %v", p.Identifier())
	}

	// delete policy
	iamClient := client.IAM(ctx, p.Context.ProviderName)
	_, err = iamClient.DeletePolicy(ctx, &iam.DeletePolicyInput{
		PolicyArn: aws.String(awsPolicy.Arn),
	})
	if err != nil {
		return errors.Wrapf(err, "failed to delete iam policy %s", p.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": p.Identifier(),
		"ARN":  awsPolicy.Arn,
	}).Info(color.Red("iam policy destroyed"))
	return nil
}

type IAMPolicyDiff struct {
	BaseResourceDiff

	descDiff      bool
	policyDocDiff bool
	tagsDiff      bool
	tagDiff       util.TagDiffResult
}

// Compare fetches the existing iam policy and if it exists, checks if this
// resource definition is equal to the corresponding AWS IAMPolicy
// returns the diffs, if it is not
func (p IAMPolicy) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := p.fetchExisting(ctx)
	if err != nil {
		return nil, fmt.Errorf("error fetch aws resource for %v", p.Identifier())
	}
	diffs := &IAMPolicyDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "iam policy does not exists")
		return diffs, nil
	}

	found := false
	diffs.Resource = existing

	// description
	if p.Description != existing.Description {
		found = true
		diffs.descDiff = true
		diffs.Messages = append(diffs.Messages, "iam policy description is not the same")
	}

	// policy doc
	if p.Policy.equals(&existing.Policy) != Equal {
		found = true
		diffs.policyDocDiff = true
		diffs.Messages = append(diffs.Messages, "iam policy document is not the same")
	}

	// tags
	if tagDiff := TagDiffForContext(ctx, existing.Tags, p.Tags); tagDiff.HasChanges() {
		found = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(existing.Tags, diffs.tagDiff)...)
	}

	if found {
		return diffs, nil
	}

	return nil, nil
}

// fetchExisting fetches the matching IAM policy from AWS.
// If an error occurs a non-nil error is returned. If the policy
// is not found both the policy and error will be nil.
//
// Since the IAM Policy attributes are stored in various types in AWS &
// retrieived via multiple API calls, this fetchExisting() returns the IAMPolicy
// buildit type, rather than /services/iam/types/** object
func (p IAMPolicy) fetchExisting(ctx context.Context) (*IAMPolicy, error) {

	iamClient := client.IAM(ctx, p.Context.ProviderName)
	var nextToken *string

	// Cache the path and policy name so we don't need to compute it every time
	path, policyName := p.PathAndName()
	for {
		respListPolicies, err := iamClient.ListPolicies(ctx, &iam.ListPoliciesInput{
			Marker:     nextToken,
			PathPrefix: &path,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error listing policies")
		}

		for _, awsPolicy := range respListPolicies.Policies {
			if *awsPolicy.PolicyName == policyName {

				// get all fields
				arn := awsPolicy.Arn
				respGetPolicy, err := iamClient.GetPolicy(ctx, &iam.GetPolicyInput{
					PolicyArn: arn,
				})
				if err != nil {
					return nil, errors.Wrapf(err, "failed to get policy %s", p.Identifier())
				}

				tags := make(map[string]string)
				for _, t := range respGetPolicy.Policy.Tags {
					tags[*t.Key] = *t.Value
				}

				// get policy document (only the default versino id)
				respGetPolicyVersion, err := iamClient.GetPolicyVersion(ctx, &iam.GetPolicyVersionInput{
					PolicyArn: arn,
					VersionId: awsPolicy.DefaultVersionId,
				})

				if err != nil {
					return nil, errors.Wrapf(err, "failed to look up policy version for %s", p.Identifier())
				}

				existingPolicyDocument, err := decodePolicyDocument(*respGetPolicyVersion.PolicyVersion.Document)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to decode IAM policy document: %s", p.Identifier())
				}

				policy := &IAMPolicy{
					Arn:              *respGetPolicy.Policy.Arn,
					Name:             *respGetPolicy.Policy.PolicyName,
					Description:      util.Coalesce(respGetPolicy.Policy.Description, ""),
					DefaultVersionId: util.Coalesce(respGetPolicy.Policy.DefaultVersionId, ""),
					Policy:           existingPolicyDocument,
					Tags:             tags,
				}
				return policy, nil
			}
		}

		if respListPolicies.Marker == nil {
			// Reached the end and never found it
			return nil, nil
		}
		nextToken = respListPolicies.Marker
	}
}

// apply provisions a new iam policy
func (p IAMPolicy) apply(ctx context.Context) error {

	iamClient := client.IAM(ctx, p.Context.ProviderName)

	policyJSON, err := json.Marshal(p.Policy)
	if err != nil {
		return errors.Wrapf(err, "failed to encode policy document as JSON: %s", p.Identifier())
	}

	//tags
	var tags []iamtypes.Tag
	for k, v := range p.Tags {
		tags = append(tags, iamtypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	path, name := p.PathAndName()
	respCreatePolicy, err := iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyDocument: aws.String(string(policyJSON)),
		Description:    aws.String(p.Description),
		PolicyName:     aws.String(name),
		Path:           aws.String(path),
		Tags:           tags,
	})
	if err != nil {
		return errors.Wrap(err, "error creating iam policy")
	}

	policyARN := respCreatePolicy.Policy.Arn
	log.WithFields(log.Fields{
		"Name": p.Name,
		"ARN":  *policyARN,
	}).Info(color.Green("iam policy created"))

	return nil
}

// applyDiffs applies diffs to an existing iam policy
//
// during applyDiffs, buildit ensures no previous version of the policy document
// exists. iam policy document versioning is not supported;
// if the diffs include changes to the policy document, then a new version of the
// policy document is created; after that all non-default version Ids are deleted.
// after buildit apply() there should be only 1 version of the policy ducment &
// is set as the default.
func (p IAMPolicy) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": p.Identifier(),
		}).Info("no updates required for iam policy")
		return nil
	}

	polDiffs, ok := diffs.(*IAMPolicyDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing iam policy
	existing, ok := polDiffs.Resource.(*IAMPolicy)
	if !ok {
		return errors.Errorf("cannot retrieve existing iam policy")
	}

	var err error

	// description
	if polDiffs.descDiff {
		return errors.New("invalid diff, iam policy description cannot be updated")
	}

	// policy document
	if polDiffs.policyDocDiff {
		// In order to update a policy we need to:
		// 1. Create a new version of the policy
		// 2. Set the new version as the default version
		// 3. Delete the old version

		iamClient := client.IAM(ctx, p.Context.ProviderName)

		policyJson, err := json.Marshal(p.Policy)
		if err != nil {
			return errors.Wrapf(err, "failed to encode policy document as JSON: %s", p.Identifier())
		}

		// update the policy document
		out, err := iamClient.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
			PolicyArn:      aws.String(existing.Arn),
			PolicyDocument: aws.String(string(policyJson)),
			SetAsDefault:   true, // This takes care of 2. so we don't need to do it separately
		})
		if err != nil {
			return errors.Wrapf(err, "failed to create new version of policy %s", p.Identifier())
		}

		newVersionId := out.PolicyVersion.VersionId
		vers, defaultVer, err := awsw.NewIAM(ctx, p.Context.ProviderName).ListPolicyVersionIds(ctx, existing.Arn)
		if err != nil {
			return errors.Wrapf(err, "error fetching version Ids for policy arn %v", p.Identifier())
		}

		// check update policy document version is actually the default.
		if *defaultVer != *newVersionId {
			return errors.Errorf("error setting the new policy document as default version for iam policy %v", p.Identifier())
		}

		// delete all non-default versions, should be only one, unless manual changes have been made
		for _, ver := range vers {
			if ver != *newVersionId {
				_, err = iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
					PolicyArn: aws.String(existing.Arn),
					VersionId: aws.String(ver),
				})
				if err != nil {
					return errors.Wrapf(err, "failled to delete old version of policy %s", p.Identifier())
				}
			}
		}
	}

	// tags
	if polDiffs.tagsDiff {
		upserts := polDiffs.tagDiff.Upserts()

		if len(upserts) > 0 {
			err = awsw.NewIAM(ctx, p.Context.ProviderName).AddPolicyTags(ctx, existing.Arn, upserts)
			if err != nil {
				return errors.Wrapf(err, "error updating iam policy tags for %v", p.Identifier())
			}
		}
		if len(polDiffs.tagDiff.Deleted) > 0 {
			err = awsw.NewIAM(ctx, p.Context.ProviderName).DeletePolicyTags(ctx, existing.Arn, polDiffs.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error deleting iam policy tags for %v", p.Identifier())
			}
		}
	}

	log.WithFields(log.Fields{
		"Name": p.Identifier(),
		"ARN":  existing.Arn,
	}).Info(color.Yellow("iam policy updated"))

	return nil
}

// fixMapKeys goes through all the policy statements and makes sure any
// nested dictionaries has string keys
func (d *IAMPolicyDocument) fixMapKeys() {
	if d == nil {
		return
	}
	for i, s := range d.Statement {
		s.Principal = util.FixMapKeys(s.Principal)
		s.NotPrincipal = util.FixMapKeys(s.NotPrincipal)
		s.Action = util.FixMapKeys(s.Action)
		s.NotAction = util.FixMapKeys(s.NotAction)
		s.Resource = util.FixMapKeys(s.Resource)
		s.NotResource = util.FixMapKeys(s.NotResource)
		s.Condition = util.FixMapKeys(s.Condition)
		d.Statement[i] = s
	}
}

// deleteNotDefaultPolicyVersions find & delete all non-default policy version for this policy in aws
// TODO: @esiddiqui move to awsw utils
func deleteNotDefaultPolicyVersions(ctx context.Context, arn, defaultVersionId string, providerName string) error {
	iamClient := client.IAM(ctx, providerName)

	// get all policy version & delete no defaults
	var marker *string
	done := false

	for !done {
		out, err := iamClient.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
			PolicyArn: aws.String(arn),
			Marker:    marker,
		})

		if err != nil {
			return errors.Wrapf(err, "error fetching all versions for iam policy %v", arn)
		}

		for _, v := range out.Versions {
			if *v.VersionId != defaultVersionId {
				_, err := iamClient.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{
					PolicyArn: aws.String(arn),
					VersionId: v.VersionId,
				})
				if err != nil {
					return errors.Wrapf(err, "error deleting policy %v, version %v", arn, *v.VersionId)
				}
			}
		}
		done = out.Marker == nil
		marker = out.Marker
	}

	return nil
}

// decodePolicyDocument decodes an IAM policy document that was returned from AWS.
// AWS returns the policy document as a URL encoded JSON string. This function will first
// URL decode the string, then unmarshal it as JSON.
func decodePolicyDocument(ds string) (IAMPolicyDocument, error) {
	// First need to URL decode it
	dec, err := url.PathUnescape(ds)
	if err != nil {
		return IAMPolicyDocument{}, errors.Wrap(err, "failed to URL decode policy document")
	}

	// Unmarshal JSON
	pd := IAMPolicyDocument{}
	err = json.Unmarshal([]byte(dec), &pd)
	if err != nil {
		return IAMPolicyDocument{}, errors.Wrap(err, "failed to decode existing policy document as JSON")
	}

	return pd, nil
}
