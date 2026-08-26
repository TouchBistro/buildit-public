package awsw

import (
	"context"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/firehose"
	"github.com/aws/aws-sdk-go-v2/service/firehose/types"
	"github.com/pkg/errors"
)

// Firehose wraps the AWS Kinesis Data Firehose client.
type Firehose struct {
	*firehose.Client
}

// NewFirehose creates a new instance of the Firehose wrapper.
func NewFirehose(ctx context.Context, providerName string) Firehose {
	return Firehose{client.Firehose(ctx, providerName)}
}

// DeliveryStreamArnForIdentifier resolves a Firehose delivery stream ARN from an identifier (ARN or Name).
// If the identifier is already an ARN, it is returned directly. If a provider prefix is present,
// the corresponding provider's client is used. Returns nil, nil if the delivery stream does not exist.
func (f Firehose) DeliveryStreamArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)

	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	firehoseService := f
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, errors.Wrapf(err, "provider %q not found", provider)
		}
		firehoseService = NewFirehose(ctx, provider)
	}

	out, err := firehoseService.DescribeDeliveryStream(ctx, &firehose.DescribeDeliveryStreamInput{
		DeliveryStreamName: aws.String(resource),
	})
	if err != nil {
		var rnfe *types.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to describe delivery stream %q", resource)
	}

	if out.DeliveryStreamDescription == nil {
		return nil, errors.Errorf("unexpected empty description for delivery stream %q", resource)
	}
	if out.DeliveryStreamDescription.DeliveryStreamARN == nil {
		return nil, errors.Errorf("unexpected empty ARN for delivery stream %q", resource)
	}
	return out.DeliveryStreamDescription.DeliveryStreamARN, nil
}
