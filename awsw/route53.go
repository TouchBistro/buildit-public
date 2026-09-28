package awsw

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Route53 struct {
	*route53.Client
	providerName string
}

func NewRoute53(ctx context.Context, providerName string) Route53 {
	return Route53{Client: client.Route53(ctx, providerName), providerName: providerName}
}

// hostedZoneCache holds each provider's hosted zone listing for the lifetime of the process
// (one CLI invocation = one run). buildit has no hosted-zone resource type, so the set of zones
// in an account cannot change between resources of one run, and every certificate resource
// shares the single listing. The mutex also serialises the slow path so concurrent callers for
// one provider page Route53 once, not once each. Tests reset it with resetHostedZoneCache.
var hostedZoneCache = struct {
	sync.Mutex
	zones map[string][]route53types.HostedZone // providerName -> listing
}{zones: map[string][]route53types.HostedZone{}}

// resetHostedZoneCache clears the process-wide listing so tests do not observe each other's fixtures.
func resetHostedZoneCache() {
	hostedZoneCache.Lock()
	defer hostedZoneCache.Unlock()
	hostedZoneCache.zones = map[string][]route53types.HostedZone{}
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

// FindHostedZoneForRecord returns the public hosted zone that owns recordFQDN: the zone
// whose name is the longest suffix of the record (DEVOPS-8968). DNS resolves through the
// most-specific delegated zone, so for "_abc.uuid.service.example.com." a zone
// "service.example.com." wins over "example.com.". Private zones are never candidates —
// ACM validates over public DNS — and several zones sharing the winning name are an
// ambiguity error rather than a guess, matching the awsw lookup convention. The returned zone is
// a copy, not a pointer into the cached listing, so callers may hold or mutate it freely.
func (r Route53) FindHostedZoneForRecord(ctx context.Context, recordFQDN string) (*route53types.HostedZone, error) {
	zones, err := r.allHostedZones(ctx)
	if err != nil {
		return nil, err
	}
	return pickHostedZoneForRecord(zones, recordFQDN)
}

// allHostedZones pages through every hosted zone in the wrapper's account once per process; the
// listing is served from hostedZoneCache afterwards. Holding the lock across the paginator walk is
// deliberate: a failed listing is not cached, and only one caller per provider pays for the pages.
func (r Route53) allHostedZones(ctx context.Context) ([]route53types.HostedZone, error) {
	hostedZoneCache.Lock()
	defer hostedZoneCache.Unlock()

	if zones, ok := hostedZoneCache.zones[r.providerName]; ok {
		return zones, nil
	}

	var zones []route53types.HostedZone
	pager := route53.NewListHostedZonesPaginator(r.Client, &route53.ListHostedZonesInput{})
	for pager.HasMorePages() {
		out, err := pager.NextPage(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to list hosted zones")
		}
		zones = append(zones, out.HostedZones...)
	}

	log.WithFields(log.Fields{"provider": r.providerName, "zones": len(zones)}).Debug("listed route53 hosted zones")
	hostedZoneCache.zones[r.providerName] = zones
	return zones, nil
}

// pickHostedZoneForRecord is the zone-selection policy: among PUBLIC zones whose name is a
// suffix of recordFQDN on a label boundary, the longest name wins; exactly one zone must
// carry that name. Pure — no AWS calls — so the policy is unit-testable.
//
// Failure modes are reported so the operator can act: no zone at all (the zone may live in
// another account → set dnsValidationDomainName with a provider prefix), only private zones
// match (ACM needs public DNS), or duplicate zone names (set dnsValidationDomainName
// explicitly; the hosted zone ids are listed).
func pickHostedZoneForRecord(zones []route53types.HostedZone, recordFQDN string) (*route53types.HostedZone, error) {
	record := NormalizeDNSName(recordFQDN)

	var private []string
	var best []route53types.HostedZone
	bestLen := -1
	for _, hz := range zones {
		name := NormalizeDNSName(aws.ToString(hz.Name))
		if record != name && !strings.HasSuffix(record, "."+name) {
			continue
		}
		if hz.Config != nil && hz.Config.PrivateZone {
			private = append(private, name)
			continue
		}
		switch {
		case len(name) > bestLen:
			best, bestLen = []route53types.HostedZone{hz}, len(name)
		case len(name) == bestLen:
			best = append(best, hz)
		}
	}

	switch len(best) {
	case 1:
		hz := best[0] // a copy: callers never receive a pointer into the cached listing
		return &hz, nil
	case 0:
		if len(private) > 0 {
			sort.Strings(private)
			return nil, errors.Errorf("only private hosted zone(s) %v match validation record %q — ACM DNS validation needs a public zone; set dnsValidationDomainName or dnsValidationZones to the public zone that owns the record",
				private, record)
		}
		return nil, errors.Errorf("no hosted zone in this account owns validation record %q — set dnsValidationDomainName or dnsValidationZones (provider::zone) if the zone lives in another account or provider",
			record)
	default:
		ids := make([]string, 0, len(best))
		for _, hz := range best {
			ids = append(ids, strings.TrimPrefix(aws.ToString(hz.Id), "/hostedzone/"))
		}
		sort.Strings(ids)
		return nil, errors.Errorf("%d hosted zones named %q could own validation record %q (%v) — set dnsValidationDomainName or dnsValidationZones explicitly to pick one",
			len(best), NormalizeDNSName(aws.ToString(best[0].Name)), record, ids)
	}
}

// NormalizeDNSName lower-cases a DNS name and guarantees the trailing dot so zone names and
// record names compare byte-for-byte regardless of how the caller spelled them. Shared with the
// certificate resource so both sides of a lookup spell names the same way.
func NormalizeDNSName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	return name
}
