package awsw

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
)

type Kafka struct {
	*kafka.Client
}

// NewKafka creates a new instance of Kafka (MSK Cluster) wrapper
func NewKafka(ctx context.Context, providerName string) Kafka {
	return Kafka{client.Kafka(ctx, providerName)}
}

// ClusterArnForIdentifier resolves an MSK Cluster ARN from an identifier (ARN or Name).
func (k Kafka) ClusterArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	kafkaService := k
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		kafkaService = NewKafka(ctx, provider)
	}

	// MSK DescribeCluster requires a ClusterArn.
	// So we must list clusters to match by name.
	var nextToken *string
	for {
		out, err := kafkaService.ListClusters(ctx, &kafka.ListClustersInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list clusters: %w", err)
		}

		for _, cluster := range out.ClusterInfoList {
			if aws.ToString(cluster.ClusterName) == resource {
				return cluster.ClusterArn, nil
			}
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return nil, fmt.Errorf("MSK cluster %q not found", resource)
}
