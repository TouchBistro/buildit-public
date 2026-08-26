package awsw

import (
	"context"

	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	types "github.com/aws/aws-sdk-go-v2/service/servicediscovery/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type ServiceDiscovery struct {
	*servicediscovery.Client
}

func NewServiceDiscovery(ctx context.Context, providerName string) ServiceDiscovery {
	return ServiceDiscovery{client.ServiceDiscovery(ctx, providerName)}
}

// ServiceArnForIdentifier resolves a ServiceDiscovery Service ARN from an identifier (ARN or ID).
func (s ServiceDiscovery) ServiceArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	sdService := s
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		sdService = NewServiceDiscovery(ctx, provider)
	}

	// 1. Try Lookup by ID
	out, err := sdService.GetService(ctx, &servicediscovery.GetServiceInput{
		Id: aws.String(resource),
	})
	if err == nil {
		return out.Service.Arn, nil
	}

	// 2. Try Lookup by Name (requires iteration as ListServices filter is limited to NamespaceId)
	// We'll iterate all services if ID lookup fails.
	var nextToken *string
	for {
		list, err := sdService.ListServices(ctx, &servicediscovery.ListServicesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list services: %w", err)
		}

		for _, srv := range list.Services {
			if aws.ToString(srv.Name) == resource {
				return srv.Arn, nil
			}
		}

		if list.NextToken == nil {
			break
		}
		nextToken = list.NextToken
	}

	return nil, fmt.Errorf("service discovery service %q not found", resource)
}

// GetResourceTags returns the tags for the ACM resource or error
func (s ServiceDiscovery) GetResourceTags(ctx context.Context, arn string) (map[string]string, error) {

	out, err := s.ListTagsForResource(ctx, &servicediscovery.ListTagsForResourceInput{
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

// AddResourceTags tags the ACM certificate with the supplied tag keys/value, returns error if thhe
// operation fails
func (s ServiceDiscovery) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		var awsTags []types.Tag
		for k, v := range tags {
			awsTags = append(awsTags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := s.TagResource(ctx, &servicediscovery.TagResourceInput{
			ResourceARN: aws.String(arn),
			Tags:        awsTags,
		})

		if err != nil {
			return errors.Wrapf(err, "failed to update tags for service discovery resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ServiceDiscovery ARN": arn,
		}).Infof("%v tags added to service discovery", len(tags))
	}
	return nil
}

// DeleteResourceTags removes the supplied tag keys from the service discovery resource, or returns an error
// if the operation fails
func (s ServiceDiscovery) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {
		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		_, err := s.UntagResource(ctx, &servicediscovery.UntagResourceInput{
			ResourceARN: aws.String(arn),
			TagKeys:     keys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for service discovery resource %s", arn)
		}
		log.WithFields(log.Fields{
			"ServiceDiscovery ARN": arn,
		}).Infof("%v tags deleted from service discovery resource", len(tags))
	}
	return nil
}

// NameSpaceIdFromName returns the service discovery namespace id for the supplied name
func (s ServiceDiscovery) NameSpaceIdFromName(ctx context.Context, namespace string) (*string, error) {
	out, err := s.ListNamespaces(ctx, &servicediscovery.ListNamespacesInput{
		Filters: []types.NamespaceFilter{
			{
				Condition: types.FilterConditionEq,
				Name:      types.NamespaceFilterNameType,
				Values:    []string{"DNS_PRIVATE"},
			},
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "error looking up namespace")
	}

	for _, ns := range out.Namespaces {
		if *ns.Name == namespace {
			return ns.Id, nil
		}
	}
	return nil, errors.New("no private namespace found with the provided name")
}

// ServiceArnForNamespaceAndService returns the service arn for the supplied namesapce & service name
func (s ServiceDiscovery) ServiceArnForNamespaceAndService(ctx context.Context, namespace string, service string) (*string, error) {

	namespaceId, err := s.NameSpaceIdFromName(ctx, namespace)
	if err != nil {
		return nil, errors.Wrap(err, "error looking up service")
	}

	done := false
	var token *string

	for !done {
		out, err := s.ListServices(ctx, &servicediscovery.ListServicesInput{
			NextToken: token,
			Filters: []types.ServiceFilter{
				{
					Condition: types.FilterConditionEq,
					Name:      types.ServiceFilterNameNamespaceId,
					Values:    []string{*namespaceId},
				},
			},
		})
		if err != nil {
			return nil, errors.Wrapf(err, "error looking up service %v.%v", service, namespace)
		}

		for _, srv := range out.Services {
			if aws.ToString(srv.Name) == service {
				return srv.Arn, nil
			}
		}

		token = out.NextToken
		done = token == nil
	}
	return nil, nil
}
