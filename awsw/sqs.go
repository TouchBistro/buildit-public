package awsw

import (
	"context"

	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type SQS struct {
	*sqs.Client
}

// NewSQS creates a new instance of SQS wrapper
func NewSQS(ctx context.Context, providerName string) SQS {
	return SQS{client.SQS(ctx, providerName)}
}

// QueueArnForIdentifier resolves a queue ARN from an identifier (ARN or name).
func (s SQS) QueueArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	// If it's already an ARN, ParseIdentifier returns it as the resource and provider as empty
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	sqsService := s
	if provider != "" {
		// Verify provider exists to avoid fatal error in client.SQS
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		sqsService = NewSQS(ctx, provider)
	}

	return sqsService.QueueArnForName(ctx, resource)
}

// QueueArnForName returns the ARN for a queue given its name
func (s SQS) QueueArnForName(ctx context.Context, name string) (*string, error) {
	// 1. Get Queue URL
	urlOut, err := s.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(name),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get URL for queue %q: %w", name, err)
	}

	// 2. Get Queue Attributes (ARN)
	attrOut, err := s.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: urlOut.QueueUrl,
		AttributeNames: []types.QueueAttributeName{
			types.QueueAttributeNameQueueArn,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get attributes for queue %q: %w", name, err)
	}

	arn := attrOut.Attributes[string(types.QueueAttributeNameQueueArn)]
	if arn == "" {
		return nil, fmt.Errorf("queue ARN not found for %q", name)
	}

	return &arn, nil
}

// GetQueueTags returns the tags for the SQS queue or error
func (s SQS) GetQueueTags(ctx context.Context, queueUrl string) (map[string]string, error) {

	// fetch the tags & convert to map
	out, err := s.ListQueueTags(ctx, &sqs.ListQueueTagsInput{
		QueueUrl: aws.String(queueUrl),
	})

	if err != nil {
		return nil, err
	}

	return out.Tags, nil
}

// AddQueueTags tags the SQS queue with the supplied tag keys/value, returns error if the
// operation fails
func (s SQS) AddQueueTags(ctx context.Context, queueUrl string, tags map[string]string) error {

	if len(tags) > 0 {

		_, err := s.TagQueue(ctx, &sqs.TagQueueInput{
			QueueUrl: aws.String(queueUrl),
			Tags:     tags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", queueUrl)
		}
		log.WithFields(log.Fields{
			"Queue Url": queueUrl,
		}).Infof("%v tags added to sqs queue", len(tags))
	}

	return nil
}

// DeleteQueueTags removes the supplied tag keys from the SQS queue, or returns an error
// if the oepration fails
func (s SQS) DeleteQueueTags(ctx context.Context, queueUrl string, tags map[string]string) error {

	if len(tags) > 0 {
		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		_, err := s.UntagQueue(ctx, &sqs.UntagQueueInput{
			QueueUrl: aws.String(queueUrl),
			TagKeys:  keys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for sqs queue %s", queueUrl)
		}
		log.WithFields(log.Fields{
			"Queue Url": queueUrl,
		}).Infof("%v tags deleted from sqs queue", len(tags))
	}
	return nil
}
