package awsw

import (
	"context"

	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type ELB struct {
	*elbv2.Client
}

// NewELB creates a new instance of ELBv2 wrapper
func NewELB(ctx context.Context, providerName string) ELB {
	return ELB{client.ELB(ctx, providerName)}
}

// LoadBalancerArnForIdentifier resolves an ELBv2 Load Balancer ARN from an identifier (ARN or Name).
func (e ELB) LoadBalancerArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	elbv2Service := e
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		elbv2Service = NewELB(ctx, provider)
	}

	// Direct lookup by Name
	out, err := elbv2Service.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
		Names: []string{resource},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe load balancer %q: %w", resource, err)
	}

	if len(out.LoadBalancers) == 0 {
		return nil, fmt.Errorf("load balancer %q not found", resource)
	}

	return out.LoadBalancers[0].LoadBalancerArn, nil
}

// TargetGroupArnForIdentifier resolves an ELBv2 Target Group ARN from an identifier (ARN or Name).
func (e ELB) TargetGroupArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	elbv2Service := e
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		elbv2Service = NewELB(ctx, provider)
	}

	// Direct lookup by Name
	out, err := elbv2Service.DescribeTargetGroups(ctx, &elbv2.DescribeTargetGroupsInput{
		Names: []string{resource},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe target group %q: %w", resource, err)
	}

	if len(out.TargetGroups) == 0 {
		return nil, fmt.Errorf("target group %q not found", resource)
	}

	return out.TargetGroups[0].TargetGroupArn, nil
}

// GetResourceTags returns the resource tags for the supplied ELBv2 resource by arn, else
// error
func (e ELB) GetResourceTags(ctx context.Context, arn string) (map[string]string, error) {

	out, err := e.DescribeTags(ctx, &elbv2.DescribeTagsInput{
		ResourceArns: []string{arn},
	})

	if err != nil {
		return nil, err
	}

	if len(out.TagDescriptions) == 0 {
		return nil, errors.Errorf("tags not found for resource arn %v", arn)
	}

	t := make(map[string]string)
	for _, tag := range out.TagDescriptions[0].Tags {
		if tag.Key != nil && tag.Value != nil {
			t[*tag.Key] = *tag.Value
		}
	}

	return t, nil
}

// AddResourceTags tags the ELBv2 resource with the supplied tag keys/value, returns error if the
// operation fails
func (e ELB) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		var awsTags []types.Tag
		for k, v := range tags {
			awsTags = append(awsTags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := e.AddTags(ctx, &elbv2.AddTagsInput{
			ResourceArns: []string{arn},
			Tags:         awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ResourceArn": arn,
		}).Infof("%v tags added to ELBv2 resource", len(tags))
	}

	return nil
}

// DeleteResourceTags removes the supplied tag keys from the ELBv2 resource, or returns an error
// if the oepration fails
func (e ELB) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {
	if len(tags) > 0 {
		var tagKeys []string
		for k := range tags {
			tagKeys = append(tagKeys, k)
		}
		_, err := e.RemoveTags(ctx, &elbv2.RemoveTagsInput{
			ResourceArns: []string{arn},
			TagKeys:      tagKeys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to delete tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ResourceArn": arn,
		}).Infof("%v tags deleted from ELBv2 resource", len(tags))
	}
	return nil
}

// DescribeTargetGroupAttributesByArn returns the target group attributes for the supplied target group arn
func (e ELB) DescribeTargetGroupAttributesByArn(ctx context.Context, arn string) ([]types.TargetGroupAttribute, error) {
	out, err := e.DescribeTargetGroupAttributes(ctx, &elbv2.DescribeTargetGroupAttributesInput{
		TargetGroupArn: aws.String(arn),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to describe attributes for target group %s", arn)
	}
	return out.Attributes, nil
}

// FindLoadBalancersForNames returns a map of string/LB for the supplied load balancer names
func (e ELB) FindLoadBalancersForNames(ctx context.Context, names ...string) (map[string]types.LoadBalancer, error) {
	respDescribeLBs, err := e.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
		Names: names,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed listing load balancers by name")
	}

	// make sure we didn't get more load balancers than we asked for
	if len(respDescribeLBs.LoadBalancers) > len(names) {
		return nil, errors.Errorf("expected max %d LBs but got %d", len(names), len(respDescribeLBs.LoadBalancers))
	}

	lbs := make(map[string]types.LoadBalancer)
	for _, lb := range respDescribeLBs.LoadBalancers {
		lbs[aws.ToString(lb.LoadBalancerName)] = lb
	}

	return lbs, nil
}

// FindLoadBalancerByArn returns the load balancer with the supplied ARN, or a non-nil error.
func (e ELB) FindLoadBalancerByArn(ctx context.Context, arn string) (*types.LoadBalancer, error) {
	out, err := e.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []string{arn},
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed describing load balancer %s", arn)
	}

	if len(out.LoadBalancers) != 1 {
		return nil, errors.Errorf("load balancer %s not found", arn)
	}

	return &out.LoadBalancers[0], nil
}

// FindLoadBalancerForName returns a single loadblancer for the supplied load balancer name, or a non-nil error
func (e ELB) FindLoadBalancerForName(ctx context.Context, name string) (*types.LoadBalancer, error) {
	out, err := e.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
		Names: []string{name},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed listing load balancers by name")
	}

	// make sure we didn't get more load balancers than we asked for
	if len(out.LoadBalancers) != 1 || aws.ToString(out.LoadBalancers[0].LoadBalancerName) != name {
		return nil, errors.Errorf("load balancer %s not found", name)
	}

	return &out.LoadBalancers[0], nil
}
