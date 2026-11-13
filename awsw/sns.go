package awsw

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/pkg/errors"
)

type SNS struct {
	*sns.Client
}

// NewSNS creates a new instance of SNS wrapper
func NewSNS(ctx context.Context, providerName string) (SNS, error) {
	c, err := client.SNS(ctx, providerName)
	if err != nil {
		return SNS{}, err
	}
	return SNS{c}, nil
}

// TopicArnForIdentifier resolves a topic ARN from an identifier (ARN or name).
func (s SNS) TopicArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	// If it's already an ARN, ParseIdentifier returns it as the resource and provider as empty
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	snsService := s
	if provider != "" {
		var err error
		snsService, err = NewSNS(ctx, provider)
		if err != nil {
			return nil, err
		}

	}

	// Fetch Account ID using the active provider for this lookup.
	stsProvider := provider
	if stsProvider == "" {
		stsProvider = client.MainProvider
	}
	stsClient, err := NewSTS(ctx, stsProvider)
	if err != nil {
		return nil, err
	}

	accountID, err := stsClient.GetAccountID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account ID for provider %q: %w", stsProvider, err)
	}

	// Get Region from the client config
	region := snsService.Options().Region

	// Construct ARN
	arn := fmt.Sprintf("arn:aws:sns:%s:%s:%s", region, accountID, resource)

	// Verify existence
	_, err = snsService.GetTopicAttributes(ctx, &sns.GetTopicAttributesInput{
		TopicArn: &arn,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to verify SNS topic %q: %w", arn, err)
	}

	return &arn, nil
}

// TopicArnForName returns the qualified topic arn for the supplied name
func (s SNS) TopicArnForName(ctx context.Context, name string) (*string, error) {
	// fetch the SNS topic
	var token *string
	topicName := regexp.MustCompile(`[^:/]*$`)
	for {
		out, err := s.ListTopics(ctx, &sns.ListTopicsInput{
			NextToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, topic := range out.Topics {
			if topicName.FindString(aws.ToString(topic.TopicArn)) == name {
				return topic.TopicArn, nil
			}
		}
		if out.NextToken == nil {
			break
		}
		token = out.NextToken
	}
	return nil, errors.Errorf("SNS topic %v by name not found", name)
}
