package awsw

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
)

type Route53 struct {
	*route53.Client
}

func NewRoute53(ctx context.Context, providerName string) Route53 {
	return Route53{client.Route53(ctx, providerName)}
}

// HostedZoneArnForIdentifier resolves a Route53 Hosted Zone ARN from an identifier (ARN, ID, or Domain Name).
func (r Route53) HostedZoneArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	r53Service := r
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		r53Service = NewRoute53(ctx, provider)
	}

	// 1. Try Lookup by ID (hostedzone/ID or just ID)
	id := resource
	if !strings.HasPrefix(id, "/hostedzone/") && !strings.Contains(id, ".") {
		// If it doesn't look like a domain name and isn't prefixed, try prepending
		id = "/hostedzone/" + id
	}

	out, err := r53Service.GetHostedZone(ctx, &route53.GetHostedZoneInput{
		Id: aws.String(id),
	})
	if err == nil {
		// Construction: arn:aws:route53:::hostedzone/<id>
		// The ID from GetHostedZone might be /hostedzone/ID
		rawId := aws.ToString(out.HostedZone.Id)
		rawId = strings.TrimPrefix(rawId, "/hostedzone/")
		arn := fmt.Sprintf("arn:aws:route53:::hostedzone/%s", rawId)
		return &arn, nil
	}

	// 2. Try Lookup by Domain Name
	domainId, err := r53Service.FindHostedZoneIdForDomain(ctx, resource)
	if err == nil {
		rawId := strings.TrimPrefix(aws.ToString(domainId), "/hostedzone/")
		arn := fmt.Sprintf("arn:aws:route53:::hostedzone/%s", rawId)
		return &arn, nil
	}

	return nil, fmt.Errorf("hosted zone %q not found", resource)
}

// FindHostedZoneIdForDomain returns the route53 hosted-zone ID for the supplied domain name
func (r Route53) FindHostedZoneIdForDomain(ctx context.Context, domain string) (*string, error) {
	if !strings.HasSuffix(domain, ".") {
		domain = domain + "."
	}

	dnsName := aws.String(domain)
	var hostedZoneId *string
	for {
		out, err := r.ListHostedZonesByName(ctx, &route53.ListHostedZonesByNameInput{
			DNSName:      dnsName,
			HostedZoneId: hostedZoneId,
		})
		if err != nil {
			return nil, err
		}

		for _, hz := range out.HostedZones {
			if domain == aws.ToString(hz.Name) {
				return hz.Id, nil
			}
			// ListHostedZonesByName returns zones in alphabetical order.
			// If we've passed the domain name alphabetically, we can stop.
			if aws.ToString(hz.Name) > domain {
				return nil, fmt.Errorf("hosted zone %s not found", domain)
			}
		}

		if !out.IsTruncated {
			break
		}
		dnsName = out.NextDNSName
		hostedZoneId = out.NextHostedZoneId
	}

	return nil, fmt.Errorf("hosted zone %s not found", domain)
}
