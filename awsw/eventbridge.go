package awsw

import (
	"context"

	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	ebrtypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type EventBridge struct {
	*eventbridge.Client
}

func NewEventBridge(ctx context.Context, providerName string) EventBridge {
	return EventBridge{client.EventBridge(ctx, providerName)}
}

// RuleArnForIdentifier resolves an EventBridge Rule ARN from an identifier (ARN or Name).
func (e EventBridge) RuleArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	ebService := e
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		ebService = NewEventBridge(ctx, provider)
	}

	// Direct lookup by Name
	out, err := ebService.DescribeRule(ctx, &eventbridge.DescribeRuleInput{
		Name: aws.String(resource),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe rule %q: %w", resource, err)
	}

	return out.Arn, nil
}

// BusArnForIdentifier resolves an EventBridge Event Bus ARN from an identifier (ARN or Name).
func (e EventBridge) BusArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	ebService := e
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		ebService = NewEventBridge(ctx, provider)
	}

	// Direct lookup by Name
	out, err := ebService.DescribeEventBus(ctx, &eventbridge.DescribeEventBusInput{
		Name: aws.String(resource),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe event bus %q: %w", resource, err)
	}

	return out.Arn, nil
}

// GetResourceTags returns the tags for the EventBridge resource or error
func (e EventBridge) GetResourceTags(ctx context.Context, arn string) (map[string]string, error) {

	t := make(map[string]string)

	// fetch the tags & convert to map
	out, err := e.ListTagsForResource(ctx, &eventbridge.ListTagsForResourceInput{
		ResourceARN: aws.String(arn),
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

// AddResourceTags tags the EventBridge resource with the supplied tag keys/value, returns error if the
// operation fails
func (e EventBridge) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		var awsTags []ebrtypes.Tag
		for k, v := range tags {
			awsTags = append(awsTags, ebrtypes.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := e.TagResource(ctx, &eventbridge.TagResourceInput{
			ResourceARN: aws.String(arn),
			Tags:        awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ARN": arn,
		}).Infof("%v tags added to eventbridge resource", len(tags))
	}

	return nil
}

// DeleteResourceTags removes the supplied tag keys from the EventBridge resource, or returns an error
// if the oepration fails
func (e EventBridge) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {
		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		_, err := e.UntagResource(ctx, &eventbridge.UntagResourceInput{
			ResourceARN: aws.String(arn),
			TagKeys:     keys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for eventbridge resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ARN": arn,
		}).Infof("%v tags deleted from resource", len(tags))
	}
	return nil
}

// ConnectionArnForName returns the ARN for an eventbridge api connection for the supplied name, else error
func (e EventBridge) ConnectionArnForName(ctx context.Context, name string) (*string, error) {

	var token *string
	done := false

	for !done {
		out, err := e.ListConnections(ctx, &eventbridge.ListConnectionsInput{
			NextToken:  token,
			NamePrefix: aws.String(name),
		})

		if err != nil {
			return nil, err
		}

		for _, cc := range out.Connections {
			if aws.ToString(cc.Name) == name {
				return cc.ConnectionArn, nil
			}
		}

		token = out.NextToken
		done = out.NextToken == nil
	}
	return nil, nil
}

// ConnectionNameForArn returns the name for an eventbridge api connection for the supplied arn, else error
func (e EventBridge) ConnectionNameForArn(ctx context.Context, arn string) (*string, error) {

	var token *string
	done := false

	for !done {
		out, err := e.ListConnections(ctx, &eventbridge.ListConnectionsInput{
			NextToken: token,
		})

		if err != nil {
			return nil, err
		}

		for _, cc := range out.Connections {
			if aws.ToString(cc.ConnectionArn) == arn {
				return cc.Name, nil
			}
		}

		token = out.NextToken
		done = out.NextToken == nil
	}
	return nil, nil
}

// ApiDestinationForName returns the arn for an eventbridge api destination for the suppplied name, else error
func (e EventBridge) ApiDestinationArnForName(ctx context.Context, name string) (*string, error) {
	var token *string
	done := false

	for !done {
		out, err := e.ListApiDestinations(ctx, &eventbridge.ListApiDestinationsInput{
			NamePrefix: aws.String(name),
			NextToken:  token,
		})

		if err != nil {
			return nil, err
		}

		for _, ad := range out.ApiDestinations {
			if aws.ToString(ad.Name) == name {
				return ad.ApiDestinationArn, nil
			}
		}

		token = out.NextToken
		done = out.NextToken == nil
	}
	return nil, nil
}

// ApiDestinationEndpointForName returns the invocation endpoint for an eventbridge api destination for the suppplied name, else error
func (e EventBridge) ApiDestinationEndpointForName(ctx context.Context, name string) (*string, error) {
	var token *string
	done := false

	for !done {
		out, err := e.ListApiDestinations(ctx, &eventbridge.ListApiDestinationsInput{
			NamePrefix: aws.String(name),
			NextToken:  token,
		})

		if err != nil {
			return nil, err
		}

		for _, ad := range out.ApiDestinations {
			if aws.ToString(ad.Name) == name {
				return ad.InvocationEndpoint, nil
			}
		}

		token = out.NextToken
		done = out.NextToken == nil
	}
	return nil, nil
}
