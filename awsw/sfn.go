package awsw

import (
	"context"
	"fmt"

	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/aws/aws-sdk-go-v2/service/sfn/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type SFN struct {
	*sfn.Client
}

// NewSFN creates a new instance of SFN wrapper
func NewSFN(ctx context.Context, providerName string) SFN {
	return SFN{client.SFN(ctx, providerName)}
}

// StateMachineArnForIdentifier resolves an SFN State Machine ARN from an identifier (ARN or Name).
func (s SFN) StateMachineArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	sfnService := s
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		sfnService = NewSFN(ctx, provider)
	}

	// 1. Try Describe (works if resource is ARN)
	// resource is not ARN here, but Describe might take a name?
	// Actually DescribeStateMachine requires ARN.
	// So we'll use StateMachineArnForName.
	arn, err := sfnService.StateMachineArnForName(ctx, resource)
	if err == nil {
		return arn, nil
	}

	return nil, fmt.Errorf("state machine %q not found: %w", resource, err)
}

// tags

// GetResourceTags returns the tags for the SFN resource or error
func (s SFN) GetResourceTags(ctx context.Context, arn string) (map[string]string, error) {

	// var done bool
	// var token *string
	t := make(map[string]string)

	// fetch the tags & convert to map
	// for !done {
	out, err := s.ListTagsForResource(ctx, &sfn.ListTagsForResourceInput{
		ResourceArn: aws.String(arn),
	})

	if err != nil {
		return nil, err
	}

	for _, tag := range out.Tags {
		if tag.Key != nil && tag.Value != nil {
			t[*tag.Key] = *tag.Value
		}
	}

	return t, nil
}

// AddResourceTags tags the SFN resource with the supplied tag keys/value, returns error if the
// operation fails
func (s SFN) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		var awsTags []types.Tag
		for k, v := range tags {
			awsTags = append(awsTags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := s.TagResource(ctx, &sfn.TagResourceInput{
			ResourceArn: aws.String(arn),
			Tags:        awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"Arn": arn,
		}).Infof("%v tags added to SFN resource", len(tags))
	}

	return nil
}

// DeleteResourceTags removes the supplied tag keys from the SFN resource, or returns an error
// if the oepration fails
func (s SFN) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		var awsTags []string
		for k := range tags {
			awsTags = append(awsTags, k)
		}
		_, err := s.UntagResource(ctx, &sfn.UntagResourceInput{
			ResourceArn: aws.String(arn),
			TagKeys:     awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ResourceId": arn,
		}).Infof("%v tags deleted from resource", len(tags))
	}
	return nil
}

// StateMachineNameForArn returns the state machine name for the supplied sfn arn
func (s SFN) StateMachineNameForArn(ctx context.Context, arn string) (*string, error) {

	out, err := s.DescribeStateMachine(ctx, &sfn.DescribeStateMachineInput{
		StateMachineArn: aws.String(arn),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching state machine for arn %v", arn)
	}

	return out.Name, nil
}

// StateMachineArnForName returns the state machine arn for the supplied name
func (s SFN) StateMachineArnForName(ctx context.Context, name string) (*string, error) {

	done := false
	var token *string

	for !done {
		out, err := s.ListStateMachines(ctx, &sfn.ListStateMachinesInput{
			NextToken: token,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "error listing state machines")
		}

		for _, sm := range out.StateMachines {
			if aws.ToString(sm.Name) == name {
				return sm.StateMachineArn, nil
			}
		}

		token = out.NextToken
		done = token == nil
	}

	return nil, errors.Errorf("no state machine found for name %v", name)
}

// StateMachineArnForNameAndVersion returns the state machine arn for the supplied name and/or version or aliase
func (s SFN) StateMachineArnForNameAndQualifer(ctx context.Context, name string, qualifier *string) (*string, error) {

	// find the state machine for just the name first
	arn, err := s.StateMachineArnForName(ctx, name)
	if err != nil {
		return nil, err
	}

	// when qualifier is not supplied
	if qualifier == nil {
		return arn, nil
	}

	expectedArn := fmt.Sprintf("%s:%s", aws.ToString(arn), aws.ToString(qualifier))

	done := false
	var token *string

	// search versions first
	for !done {
		out, err := s.ListStateMachineVersions(ctx, &sfn.ListStateMachineVersionsInput{
			StateMachineArn: arn,
			NextToken:       token,
		})

		if err != nil {
			return nil, errors.Wrapf(err, "error listing state machine versions")
		}

		for _, smv := range out.StateMachineVersions {
			if util.Coalesce(smv.StateMachineVersionArn, "") == expectedArn {
				return smv.StateMachineVersionArn, nil
			}
		}

		token = out.NextToken
		done = token == nil
	}

	// if not found, check for aliases
	done = false
	token = nil
	for !done {
		out, err := s.ListStateMachineAliases(ctx, &sfn.ListStateMachineAliasesInput{
			StateMachineArn: arn,
			NextToken:       token,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "error listing state machines")
		}

		for _, sma := range out.StateMachineAliases {
			if util.Coalesce(sma.StateMachineAliasArn, "") == expectedArn {
				return sma.StateMachineAliasArn, nil
			}
		}

		token = out.NextToken
		done = token == nil
	}

	return nil, errors.Errorf("no state machine found for name %v", name)
}

// StateMachineVersionsArnsForArn returns an []string containing all state machine version arns
func (s SFN) StateMachineVersionsArnsForArn(ctx context.Context, arn string) ([]string, error) {

	var done bool
	var token *string
	var versions []string

	for !done {
		out, err := s.ListStateMachineVersions(ctx, &sfn.ListStateMachineVersionsInput{
			StateMachineArn: aws.String(arn),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "error fetching state machine versions for %v", arn)
		}

		for _, v := range out.StateMachineVersions {
			versions = append(versions, aws.ToString(v.StateMachineVersionArn))
		}
		token = out.NextToken
		done = token == nil
	}

	return versions, nil
}
