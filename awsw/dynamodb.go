package awsw

import (
	"context"

	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type DynamoDB struct {
	*dynamodb.Client
}

// NewDynamoDB creates a new instance of DynamoDB wrapper
func NewDynamoDB(ctx context.Context, providerName string) DynamoDB {
	return DynamoDB{client.DynamoDB(ctx, providerName)}
}

// TableArnForIdentifier resolves a DynamoDB Table ARN from an identifier (ARN or Table Name).
func (d DynamoDB) TableArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	dynamoService := d
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		dynamoService = NewDynamoDB(ctx, provider)
	}

	// 1. Try Lookup by Table Name
	out, err := dynamoService.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(resource),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe table %q: %w", resource, err)
	}

	if out.Table == nil {
		return nil, fmt.Errorf("table %q not found", resource)
	}

	return out.Table.TableArn, nil
}

// AddResourceTags tags the DynamoDB resource with the supplied tag keys/value, returns error if the operation fails
func (d DynamoDB) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		var awsTags []types.Tag
		for k, v := range tags {
			awsTags = append(awsTags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := d.TagResource(ctx, &dynamodb.TagResourceInput{
			ResourceArn: aws.String(arn),
			Tags:        awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ResourceArn": arn,
		}).Infof("%v tags added to DynamoDB resource", len(tags))
	}

	return nil
}

// DeleteResourceTags removes the supplied tag keys from the DynamoDB resource, or returns an error
// if the operation fails
func (d DynamoDB) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) > 0 {
		var tagKeys []string
		for k := range tags {
			tagKeys = append(tagKeys, k)
		}
		_, err := d.UntagResource(ctx, &dynamodb.UntagResourceInput{
			ResourceArn: aws.String(arn),
			TagKeys:     tagKeys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to delete tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ResourceArn": arn,
		}).Infof("%v tags deleted from DynamoDB resource", len(tags))
	}
	return nil
}
