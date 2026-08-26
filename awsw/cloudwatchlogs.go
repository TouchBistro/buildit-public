package awsw

import (
	"context"

	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type CloudWatchLogs struct {
	*cloudwatchlogs.Client
}

func NewCloudWatchLogs(ctx context.Context, providerName string) CloudWatchLogs {
	return CloudWatchLogs{client.CloudWatchLogs(ctx, providerName)}
}

// LogGroupArnForIdentifier resolves a CloudWatch Log Group ARN from an identifier (ARN or Name).
func (c CloudWatchLogs) LogGroupArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	cwlService := c
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		cwlService = NewCloudWatchLogs(ctx, provider)
	}

	// 1. Try Direct Lookup (pagination required to check all results)
	var nextToken *string
	for {
		out, err := cwlService.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{
			LogGroupNamePrefix: aws.String(resource),
			NextToken:          nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe log groups %q: %w", resource, err)
		}

		for _, lg := range out.LogGroups {
			if aws.ToString(lg.LogGroupName) == resource {
				return lg.Arn, nil
			}
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return nil, fmt.Errorf("log group %q not found", resource)
}

// GetResourceTags returns the tags for the cloudwatchlogs resource or error
func (c CloudWatchLogs) GetResourceTags(ctx context.Context, arn string) (map[string]string, error) {
	out, err := c.ListTagsForResource(ctx, &cloudwatchlogs.ListTagsForResourceInput{
		ResourceArn: aws.String(arn),
	})

	if err != nil {
		return nil, err
	}

	mmap := make(map[string]string)
	for k, v := range out.Tags {
		mmap[k] = v
	}

	return mmap, nil
}

// AddResourceTags tags the cloudwatchlogs resource with the supplied tag keys/value, returns error if thhe
// operation fails
func (c CloudWatchLogs) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) > 0 {
		_, err := c.TagResource(ctx, &cloudwatchlogs.TagResourceInput{
			ResourceArn: aws.String(arn),
			Tags:        tags,
		})

		if err != nil {
			return errors.Wrapf(err, "failed to update tags for cloudwatchlogs resource %s", arn)
		}
		log.WithFields(log.Fields{
			"CloudWatchLogs ARN": arn,
		}).Infof("%v tags added to cloudwatchlogs resource", len(tags))
	}
	return nil
}

// DeleteResourceTags removes the supplied tag keys from the cloudwatchlogs resource, or returns an error
// if the oepration fails
func (c CloudWatchLogs) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) > 0 {
		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		_, err := c.UntagResource(ctx, &cloudwatchlogs.UntagResourceInput{
			ResourceArn: aws.String(arn),
			TagKeys:     keys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to delete tags for cloudwatchlogs %s", arn)
		}
		log.WithFields(log.Fields{
			"CloudWatchLogs ARN": arn,
		}).Infof("%v tags deleted from cloudwatchlogs resource", len(tags))
	}
	return nil
}
