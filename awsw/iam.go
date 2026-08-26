package awsw

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	types "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type IAM struct {
	*iam.Client
}

// NewIAM creates a new instance of IAM wrapper
func NewIAM(ctx context.Context, providerName string) IAM {
	return IAM{client.IAM(ctx, providerName)}
}

// RoleArnForIdentifier resolves an IAM Role ARN from an identifier (ARN or RoleName).
func (i IAM) RoleArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	iamService := i
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		iamService = NewIAM(ctx, provider)
	}

	// Direct lookup by Role Name
	out, err := iamService.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(resource),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get role %q: %w", resource, err)
	}

	return out.Role.Arn, nil
}

// PolicyArnForIdentifier resolves an IAM Policy ARN from an identifier (ARN or PolicyName).
func (i IAM) PolicyArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	iamService := i
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		iamService = NewIAM(ctx, provider)
	}

	// To resolve a policy by name, we need candidate ARNs.
	// To construct customer-managed ARNs, we need the account ID.
	stsProvider := provider
	if stsProvider == "" {
		stsProvider = client.MainProvider
	}
	stsClient := NewSTS(ctx, stsProvider)
	accountID, err := stsClient.GetAccountID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID for policy lookup: %w", err)
	}

	// Try customer-managed and AWS-managed formats, with fallback to raw
	// <name> if policy/<name> is not the right path.
	candidates := []string{
		fmt.Sprintf("arn:aws:iam::%s:policy/%s", accountID, resource),
		fmt.Sprintf("arn:aws:iam::%s:%s", accountID, resource),
		fmt.Sprintf("arn:aws:iam::aws:policy/%s", resource),
		fmt.Sprintf("arn:aws:iam::aws:%s", resource),
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}

		_, err = iamService.GetPolicy(ctx, &iam.GetPolicyInput{
			PolicyArn: aws.String(candidate),
		})
		if err == nil {
			return aws.String(candidate), nil
		}
	}

	return nil, fmt.Errorf("policy %q not found (checked custom and aws-managed with and without policy/ prefix)", resource)
}

// AddPolicyTags tags the IAM policy with the supplied tag keys/value, returns error if thhe
// operation fails
func (i IAM) AddPolicyTags(ctx context.Context, policyArn string, tags map[string]string) error {
	if len(tags) > 0 {
		var awsTags []types.Tag
		for k, v := range tags {
			awsTags = append(awsTags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := i.TagPolicy(ctx, &iam.TagPolicyInput{
			PolicyArn: aws.String(policyArn),
			Tags:      awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for iam policy %s", policyArn)
		}
		log.WithFields(log.Fields{
			"ResourceId": policyArn,
		}).Infof("%v tags added to iam policy", len(tags))
	}

	return nil
}

// DeletePolicyTags removes the supplied tag keys from the IAM policy, or returns an error
// if the oepration fails
func (i IAM) DeletePolicyTags(ctx context.Context, policyArn string, tags map[string]string) error {
	if len(tags) > 0 {
		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		_, err := i.UntagPolicy(ctx, &iam.UntagPolicyInput{
			PolicyArn: aws.String(policyArn),
			TagKeys:   keys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for iam policy %s", policyArn)
		}
		log.WithFields(log.Fields{
			"ResourceId": policyArn,
		}).Infof("%v tags deleted from iam policy", len(tags))
	}
	return nil
}

// GetRoleTags fetches resource tags from the iam role, or returns an error
func (i IAM) GetRoleTags(ctx context.Context, roleName string) (map[string]string, error) {
	var done bool
	var marker *string
	tags := make(map[string]string)

	for !done {
		out, err := i.ListRoleTags(ctx, &iam.ListRoleTagsInput{
			RoleName: aws.String(roleName),
			Marker:   marker,
		})
		if err != nil {
			return nil, err
		}

		for _, t := range out.Tags {
			if t.Key != nil && t.Value != nil {
				tags[*t.Key] = *t.Value
			}
		}

		done = out.Marker == nil
		marker = out.Marker
	}
	return tags, nil
}

// AddRoleTags tags the IAM role with the supplied tag keys/value, returns error if thhe
// operation fails
func (i IAM) AddRoleTags(ctx context.Context, roleName string, tags map[string]string) error {
	if len(tags) > 0 {
		var awsTags []types.Tag
		for k, v := range tags {
			awsTags = append(awsTags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := i.TagRole(ctx, &iam.TagRoleInput{
			RoleName: aws.String(roleName),
			Tags:     awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for iam role %s", roleName)
		}
		log.WithFields(log.Fields{
			"ResourceName": roleName,
		}).Infof("%v tags added to iam role", len(tags))
	}
	return nil
}

// DeleteRoleTags removes the supplied tag keys from the IAM role, or returns an error
// if the oepration fails
func (i IAM) DeleteRoleTags(ctx context.Context, roleName string, tags map[string]string) error {
	if len(tags) > 0 {
		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		_, err := i.UntagRole(ctx, &iam.UntagRoleInput{
			RoleName: aws.String(roleName),
			TagKeys:  keys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for iam role %s", roleName)
		}
		log.WithFields(log.Fields{
			"ResourceName": roleName,
		}).Infof("%v tags deleted from iam role", len(tags))
	}
	return nil
}

// returns a []string with versionIds of all policy document versions & the default policy version id
// for the supplied policy arn, else non-nil error
func (i IAM) ListPolicyVersionIds(ctx context.Context, policyArn string) ([]string, *string, error) {
	done := false
	var marker *string
	var defaultVersionId *string
	var versionIds []string

	for !done {
		out, err := i.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{
			PolicyArn: aws.String(policyArn),
			Marker:    marker,
		})
		if err != nil {
			return nil, nil, err
		}

		for _, ver := range out.Versions {
			versionIds = append(versionIds, aws.ToString(ver.VersionId))
			if ver.IsDefaultVersion {
				defaultVersionId = ver.VersionId
			}
		}

		done = out.Marker == nil
		marker = out.Marker
	}

	return versionIds, defaultVersionId, nil
}

// ListAttachedPoliciesByRole returns a list if attached policies for the supplied role name
func (i IAM) ListAttachedPoliciesByRole(ctx context.Context, roleName string) ([]types.AttachedPolicy, error) {
	var nextToken *string
	var policies []types.AttachedPolicy
	for {
		respListAttachedPolicies, err := i.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{
			Marker:   nextToken,
			RoleName: &roleName,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list policies attached to role %s", roleName)
		}

		policies = append(policies, respListAttachedPolicies.AttachedPolicies...)
		if respListAttachedPolicies.Marker == nil {
			// rached the end
			return policies, nil
		}
		nextToken = respListAttachedPolicies.Marker
	}
}

// RoleNameForArn return the IAM role name for the supplied arn
// Note: this is very slow & not recommneded for use
func (i IAM) RoleNameForArn(ctx context.Context, arn string) (*string, error) {
	done := false
	var marker *string

	for !done {
		out, err := i.ListRoles(ctx, &iam.ListRolesInput{
			MaxItems: aws.Int32(100),
			Marker:   marker,
		})
		if err != nil {
			return nil, err
		}

		for _, r := range out.Roles {
			if aws.ToString(r.Arn) == arn {
				return r.RoleName, nil
			}
		}

		marker = out.Marker
		done = !out.IsTruncated
	}

	return nil, errors.Errorf("role not found for arn %v", arn)
}

// RoleArnForName returns an IAM role arn for the supplied role name
func (i IAM) RoleArnForName(ctx context.Context, roleName string) (*string, error) {
	if roleName == "" {
		return nil, nil
	}

	out, err := i.GetRole(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, err
	}

	if out.Role != nil {
		return out.Role.Arn, nil
	}
	return nil, errors.New("iam role with the specified name not found")
}
