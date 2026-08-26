package awsw

import (
	"context"

	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	types "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type CloudWatch struct {
	*cloudwatch.Client
}

func NewCloudWatch(ctx context.Context, providerName string) CloudWatch {
	return CloudWatch{client.CloudWatch(ctx, providerName)}
}

// AlarmArnForIdentifier resolves a CloudWatch Alarm ARN from an identifier (ARN or Alarm Name).
func (c CloudWatch) AlarmArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	cwService := c
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		cwService = NewCloudWatch(ctx, provider)
	}

	// 1. Try treating as Alarm Name
	out, err := cwService.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{
		AlarmNames: []string{resource},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe alarm %q: %w", resource, err)
	}
	// DescribeAlarms returns what it found.
	for _, alarm := range out.MetricAlarms {
		if aws.ToString(alarm.AlarmName) == resource {
			return alarm.AlarmArn, nil
		}
	}
	// Also check composite alarms just in case users use them
	for _, alarm := range out.CompositeAlarms {
		if aws.ToString(alarm.AlarmName) == resource {
			return alarm.AlarmArn, nil
		}
	}

	// 2. Fallback: Construct ARN?
	// format: arn:aws:cloudwatch:region:account:alarm:alarm_name
	// We need account and region.
	// If it wasn't found in DescribeAlarms, it likely doesn't exist, so construction might be misleading.
	// We'll stick to returning error if not found.

	return nil, fmt.Errorf("alarm %q not found", resource)
}

// GetResourceTags returns the tags for the cloudwatch resource or error
func (c CloudWatch) GetResourceTags(ctx context.Context, arn string) (map[string]string, error) {
	out, err := c.ListTagsForResource(ctx, &cloudwatch.ListTagsForResourceInput{
		ResourceARN: aws.String(arn),
	})

	if err != nil {
		return nil, err
	}

	mmap := make(map[string]string)
	for _, t := range out.Tags {
		if t.Key != nil && t.Value != nil {
			mmap[*t.Key] = *t.Value
		}
	}

	return mmap, nil
}

// AddResourceTags tags the cloudwatch resource with the supplied tag keys/value, returns error if thhe
// operation fails
func (c CloudWatch) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) > 0 {
		var awsTags []types.Tag
		for k, v := range tags {
			awsTags = append(awsTags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := c.TagResource(ctx, &cloudwatch.TagResourceInput{
			ResourceARN: aws.String(arn),
			Tags:        awsTags,
		})

		if err != nil {
			return errors.Wrapf(err, "failed to update tags for cloudwatch resource %s", arn)
		}
		log.WithFields(log.Fields{
			"Cloudwatch ARN": arn,
		}).Infof("%v tags added to cloudwatch resource", len(tags))
	}
	return nil
}

// DeleteResourceTags removes the supplied tag keys from the cloudewatch resource, or returns an error
// if the oepration fails
func (c CloudWatch) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) > 0 {
		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		_, err := c.UntagResource(ctx, &cloudwatch.UntagResourceInput{
			ResourceARN: aws.String(arn),
			TagKeys:     keys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for cloudwatch %s", arn)
		}
		log.WithFields(log.Fields{
			"CloudWatch ARN": arn,
		}).Infof("%v tags deleted from cloudwatch", len(tags))
	}
	return nil
}
