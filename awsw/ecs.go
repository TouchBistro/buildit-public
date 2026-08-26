package awsw

import (
	"context"

	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type ECS struct {
	*ecs.Client
}

// NewECS creates a new instance of ECS wrapper
func NewECS(ctx context.Context, providerName string) ECS {
	return ECS{client.ECS(ctx, providerName)}
}

// ClusterArnForIdentifier resolves an ECS Cluster ARN from an identifier (ARN or Name).
func (e ECS) ClusterArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	ecsService := e
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		ecsService = NewECS(ctx, provider)
	}

	// DescribeClusters accepts both cluster name and ARN.
	// It's efficient and handles the lookup directly.
	out, err := ecsService.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{resource},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe cluster %q: %w", resource, err)
	}

	// Check if any clusters were returned
	if len(out.Clusters) == 0 {
		return nil, fmt.Errorf("cluster %q not found", resource)
	}

	if len(out.Failures) > 0 {
		return nil, fmt.Errorf("failed to resolve cluster %q: %s", resource, aws.ToString(out.Failures[0].Reason))
	}

	cluster := out.Clusters[0]
	// Optional: specific status check? For now, if it exists, it flows.
	if cluster.Status != nil && aws.ToString(cluster.Status) == "INACTIVE" {
		return nil, fmt.Errorf("cluster %q is INACTIVE", resource)
	}

	return cluster.ClusterArn, nil
}

// GetResourceTags returns the tags for the ECS resource or error
func (e ECS) GetResourceTags(ctx context.Context, arn string) (map[string]string, error) {

	out, err := e.ListTagsForResource(ctx, &ecs.ListTagsForResourceInput{
		ResourceArn: &arn,
	})

	if err != nil {
		return nil, err
	}

	t := make(map[string]string)
	for _, tag := range out.Tags {
		if tag.Key != nil && tag.Value != nil {
			t[*tag.Key] = *tag.Value
		}
	}

	return t, nil
}

// AddResourceTags tags the ECS resourse with the supplied tag keys/value, returns error if thhe
// operation fails
func (e ECS) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		var awsTags []types.Tag
		for k, v := range tags {
			awsTags = append(awsTags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := e.TagResource(ctx, &ecs.TagResourceInput{
			ResourceArn: aws.String(arn),
			Tags:        awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ResourceArn": arn,
		}).Infof("%v tags added to ECS resource", len(tags))
	}

	return nil
}

// DeleteResourceTags removes the supplied tag keys from the ECS resource, or returns an error
// if the oepration fails
func (e ECS) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) > 0 {
		var tagKeys []string
		for k := range tags {
			tagKeys = append(tagKeys, k)
		}
		_, err := e.UntagResource(ctx, &ecs.UntagResourceInput{
			ResourceArn: aws.String(arn),
			TagKeys:     tagKeys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to delete tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ResourceArn": arn,
		}).Infof("%v tags deleted from ECS resource", len(tags))
	}
	return nil
}

// EcsClusterArnForName returns the cluster arn for the cluster name, else error
func (e ECS) EcsClusterArnForName(ctx context.Context, clusterName string) (*string, error) {

	out, err := e.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{clusterName},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching cluster for name %v", clusterName)
	}

	for _, c := range out.Clusters {
		if clusterName == aws.ToString(c.ClusterName) {
			return c.ClusterArn, nil
		}
	}
	return nil, errors.Wrapf(err, "cluster %v does not exist", clusterName)
}
