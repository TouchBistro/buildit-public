package awsw

import (
	"context"

	"fmt"
	"regexp"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type ACM struct {
	*acm.Client
	// global marks the wrapper as pinned to us-east-1 (CloudFront viewer certificates)
	// so provider overrides in identifiers ("prov::cert") keep the pin.
	global bool
}

// NewACM creates a new instance of ACM wrapper
func NewACM(ctx context.Context, providerName string) ACM {
	return ACM{Client: client.ACM(ctx, providerName)}
}

// NewACMGlobal creates an ACM wrapper pinned to us-east-1 — the only region CloudFront
// reads viewer certificates from, regardless of the provider's configured region.
// Regional consumers (e.g. load balancer listeners, whose certificates must live in the
// load balancer's own region) use NewACM.
func NewACMGlobal(ctx context.Context, providerName string) ACM {
	return ACM{Client: client.ACMGlobal(ctx, providerName), global: true}
}

// acmCertIDRegex matches an ACM certificate id — the UUID tail of a certificate ARN.
var acmCertIDRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

// CertificateArnForIdentifier resolves an ACM Certificate ARN from an identifier: a full
// ARN is returned as-is, a certificate id (UUID) matches the trailing "certificate/{id}"
// segment of the ARN, and anything else matches by Domain Name — with the
// buildit:resource-id tag as the tiebreaker when several certificates share the domain
// (DEVOPS-8880). Errors when no certificate matches; FindCertificateByIdentifier is the
// lifecycle variant that returns (nil, nil) instead.
func (a ACM) CertificateArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	arn, err := a.FindCertificateByIdentifier(ctx, identifier)
	if err != nil {
		return nil, err
	}
	if arn == nil {
		resource, _ := ParseIdentifier(identifier)
		if acmCertIDRegex.MatchString(resource) {
			return nil, errors.Errorf("certificate not found for id %q", resource)
		}
		return nil, errors.Errorf("certificate not found for domain %q", resource)
	}
	return arn, nil
}

// FindCertificateByIdentifier resolves an ACM Certificate ARN from an identifier and
// returns (nil, nil) when no certificate matches. A full ARN is returned as-is, a
// certificate id (UUID) matches the trailing "certificate/{id}" segment of the ARN, and
// anything else matches by Domain Name — falling back to the buildit:resource-id tag
// when no domain matches, so a certificate resource's buildit name (a config that sets
// domainName separately) is also a valid reference. Lookups scan ListCertificates in the
// wrapper's account/region, so the returned ARN is exactly what AWS stores — account,
// region, and partition are never guessed.
//
// A domain name is not unique in ACM — several certificates can share a CN. When more
// than one matches, the buildit:resource-id tag decides: for a buildit-managed
// certificate the tag value is util.SafeTagValue(<resource name>), and the name defaults
// to the domain, so the tagged candidate is the one buildit manages (DEVOPS-8880). The
// tag comparison goes through the same util.SafeTagValue as the writer — a two-sided
// contract; matching the raw identifier instead would silently miss wildcard domains
// ('*' is not a legal tag character). Tags are only consulted when the domain match is
// not exactly one, so the common single-match path costs no extra API calls. A collision
// that the tag cannot settle (none tagged, or several tagged alike) is an error, per the
// awsw resolution convention — never an arbitrary pick.
func (a ACM) FindCertificateByIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	acmService := a
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		if a.global {
			acmService = NewACMGlobal(ctx, provider)
		} else {
			acmService = NewACM(ctx, provider)
		}
	}

	if acmCertIDRegex.MatchString(resource) {
		return acmService.findCertificateByID(ctx, resource)
	}

	scan, err := acmService.scanCertificates(ctx, resource)
	if err != nil {
		return nil, err
	}

	want := util.SafeTagValue(resource)

	switch len(scan.domainMatches) {
	case 0:
		// Name tier: the identifier may be a certificate resource's buildit name
		// rather than a domain — a config that sets domainName separately gives its
		// certificate a name that matches no CN. Costs one ListTagsForCertificate
		// call per certificate in the account, and only on what was previously the
		// not-found path.
		picked, tagged, err := acmService.resolveByResourceIDTag(ctx, scan.allArns, want)
		if err != nil {
			return nil, err
		}
		if len(tagged) > 1 {
			return nil, errors.Errorf(
				"multiple certificates carry buildit:resource-id=%q — cannot disambiguate: %v",
				want, tagged)
		}
		if picked != nil {
			log.WithFields(log.Fields{
				"Name": resource,
				"ARN":  *picked,
			}).Debug("identifier matched no domain; resolved by buildit:resource-id tag")
		}
		return picked, nil
	case 1:
		return &scan.domainMatches[0], nil
	}

	// Ambiguous domain: consult the buildit:resource-id tag on each candidate. One
	// sequential ListTagsForCertificate call per candidate — bounded by the domain
	// collision count (typically 2-3), not by the account's total certificates.
	picked, tagged, err := acmService.resolveByResourceIDTag(ctx, scan.domainMatches, want)
	if err != nil {
		return nil, err
	}
	switch {
	case len(tagged) > 1:
		return nil, errors.Errorf(
			"multiple certificates for domain %q carry buildit:resource-id=%q — cannot disambiguate: %v",
			resource, want, tagged)
	case picked != nil:
		log.WithFields(log.Fields{
			"Domain": resource,
			"ARN":    *picked,
		}).Debug("multiple certificates share the domain; picked the buildit:resource-id-tagged one")
		return picked, nil
	default:
		// No candidate is buildit-managed either: any pick would be arbitrary (the
		// old behavior silently took whichever listed first). Per the awsw
		// resolution convention, ambiguity fails loud at plan time instead of
		// wiring a consumer to the wrong CN twin.
		return nil, errors.Errorf(
			"%d certificates match domain %q and none carries buildit:resource-id=%q — manage the certificate with buildit (or tag it), or reference it by ARN, certificate id, or its buildit resource name: %v",
			len(scan.domainMatches), resource, want, scan.domainMatches)
	}
}

// FindCertificateForResource is the lifecycle lookup for the certificate resource itself:
// domain says what to match in ACM, resourceID says which certificate belongs to this
// resource. The two were the same value until certificates gained a separate domainName
// field — a resource name distinct from the CN is what lets two certificates share a CN
// in one config scope. Returns (nil, nil) when this resource's certificate does not
// exist yet.
//
// Selection:
//   - one domain match: adopted when untagged or tagged as this resource; a CN twin
//     tagged for a DIFFERENT resource is not this one's — (nil, nil), so a second
//     same-CN resource creates its own certificate instead of retagging the first.
//   - several domain matches: the buildit:resource-id tag decides, exactly as in
//     FindCertificateByIdentifier.
//   - no domain match, and the resource is explicitly named (resourceID != domain): a
//     certificate elsewhere already carrying this resource-id means the config's
//     domainName changed under a stable name. Certificate domains are immutable, so
//     that fails loud instead of silently requesting a second certificate and
//     orphaning the first.
func (a ACM) FindCertificateForResource(ctx context.Context, domain, resourceID string) (*string, error) {
	want := util.SafeTagValue(resourceID)

	scan, err := a.scanCertificates(ctx, domain)
	if err != nil {
		return nil, err
	}

	switch len(scan.domainMatches) {
	case 0:
		if resourceID == domain {
			// Default naming (name IS the domain): a domain miss simply means the
			// certificate does not exist; nothing to guard, no extra API calls.
			return nil, nil
		}
		_, tagged, err := a.resolveByResourceIDTag(ctx, scan.allArns, want)
		if err != nil {
			return nil, err
		}
		if len(tagged) > 0 {
			return nil, errors.Errorf(
				"buildit:resource-id=%q already marks certificate(s) %v, whose domain is not %q — a certificate's domain cannot change; destroy the certificate first or rename the resource",
				want, tagged, domain)
		}
		return nil, nil
	case 1:
		arn := scan.domainMatches[0]
		tags, err := a.GetResourceTags(ctx, arn)
		if err != nil {
			var rnfe *types.ResourceNotFoundException
			if errors.As(err, &rnfe) {
				return nil, nil
			}
			return nil, errors.Wrapf(err, "failed to read tags for certificate %v", arn)
		}
		if id, ok := tags[util.BuilditResourceIDTagKey]; ok && id != want {
			// The single CN match belongs to another buildit resource; treating it
			// as this one's would retag — and thereby steal — the twin.
			return nil, nil
		}
		return &arn, nil
	}

	picked, tagged, err := a.resolveByResourceIDTag(ctx, scan.domainMatches, want)
	if err != nil {
		return nil, err
	}
	switch {
	case len(tagged) > 1:
		return nil, errors.Errorf(
			"multiple certificates for domain %q carry buildit:resource-id=%q — cannot disambiguate: %v",
			domain, want, tagged)
	case picked != nil:
		return picked, nil
	default:
		return nil, errors.Errorf(
			"%d certificates match domain %q and none carries buildit:resource-id=%q — manage the certificate with buildit (or tag it) or reference it by ARN/certificate id: %v",
			len(scan.domainMatches), domain, want, scan.domainMatches)
	}
}

// findCertificateByID resolves a certificate id (UUID) against the trailing
// "certificate/{id}" segment of each ARN. Ids are unique per account/region, so the
// first hit wins and the scan can return early.
func (a ACM) findCertificateByID(ctx context.Context, id string) (*string, error) {
	idSuffix := "/" + strings.ToLower(id)

	var nextToken *string
	for {
		out, err := a.ListCertificates(ctx, &acm.ListCertificatesInput{
			// The API's default filter hides certificates whose key type is not RSA_2048
			// (e.g. ECDSA); list every key type so all certificates are candidates.
			Includes:  &types.Filters{KeyTypes: types.KeyAlgorithm("").Values()},
			NextToken: nextToken,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list certificates")
		}

		for _, cert := range out.CertificateSummaryList {
			if strings.HasSuffix(strings.ToLower(aws.ToString(cert.CertificateArn)), idSuffix) {
				return cert.CertificateArn, nil
			}
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return nil, nil
}

// certificateScan is one ListCertificates pass: the ARNs whose DomainName equals the
// searched domain, plus every ARN seen — kept for the resource-id tag tiers that run
// when the domain match is empty.
type certificateScan struct {
	domainMatches []string
	allArns       []string
}

// scanCertificates pages through every certificate in the wrapper's account/region once,
// collecting the ARNs that match the domain and the full ARN list.
func (a ACM) scanCertificates(ctx context.Context, domain string) (certificateScan, error) {
	var scan certificateScan

	var nextToken *string
	for {
		out, err := a.ListCertificates(ctx, &acm.ListCertificatesInput{
			// The API's default filter hides certificates whose key type is not RSA_2048
			// (e.g. ECDSA); list every key type so all certificates are candidates.
			Includes:  &types.Filters{KeyTypes: types.KeyAlgorithm("").Values()},
			NextToken: nextToken,
		})
		if err != nil {
			return scan, errors.Wrap(err, "failed to list certificates")
		}

		for _, cert := range out.CertificateSummaryList {
			arn := aws.ToString(cert.CertificateArn)
			scan.allArns = append(scan.allArns, arn)
			if aws.ToString(cert.DomainName) == domain {
				scan.domainMatches = append(scan.domainMatches, arn)
			}
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	return scan, nil
}

// resolveByResourceIDTag reads the buildit:resource-id tag on each candidate ARN and
// picks the single one whose value equals want. One ListTagsForCertificate call per
// candidate; a candidate deleted between List and ListTags is skipped rather than
// failing the whole lookup.
func (a ACM) resolveByResourceIDTag(ctx context.Context, arns []string, want string) (*string, []string, error) {
	candidates := make([]certCandidate, 0, len(arns))
	for _, arn := range arns {
		tags, err := a.GetResourceTags(ctx, arn)
		if err != nil {
			var rnfe *types.ResourceNotFoundException
			if errors.As(err, &rnfe) {
				continue
			}
			return nil, nil, errors.Wrapf(err, "failed to read tags for certificate %v", arn)
		}
		candidates = append(candidates, certCandidate{arn: arn, tags: tags})
	}

	picked, tagged := pickCertificateByResourceID(candidates, want)
	return picked, tagged, nil
}

// certCandidate pairs a certificate ARN with its tags for disambiguation.
type certCandidate struct {
	arn  string
	tags map[string]string
}

// pickCertificateByResourceID returns the single candidate whose buildit:resource-id tag
// equals want, plus the list of ALL matching ARNs so the caller can reject a still-
// ambiguous result. Pure — no AWS calls — so the selection policy is unit-testable.
func pickCertificateByResourceID(candidates []certCandidate, want string) (*string, []string) {
	var tagged []string
	for _, c := range candidates {
		if c.tags[util.BuilditResourceIDTagKey] == want {
			tagged = append(tagged, c.arn)
		}
	}
	if len(tagged) == 1 {
		return &tagged[0], tagged
	}
	return nil, tagged
}

// GetResourceTags returns the tags for the ACM resource or error
func (a ACM) GetResourceTags(ctx context.Context, arn string) (map[string]string, error) {

	out, err := a.ListTagsForCertificate(ctx, &acm.ListTagsForCertificateInput{
		CertificateArn: aws.String(arn),
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
func (a ACM) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		var awsTags []types.Tag
		for k, v := range tags {
			awsTags = append(awsTags, types.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}
		_, err := a.AddTagsToCertificate(ctx, &acm.AddTagsToCertificateInput{
			CertificateArn: aws.String(arn),
			Tags:           awsTags,
		})

		if err != nil {
			return errors.Wrapf(err, "failed to update tags for acm certificate %s", arn)
		}
		log.WithFields(log.Fields{
			"Certificate ARN": arn,
		}).Infof("%v tags added to acm certificate", len(tags))
	}

	return nil
}

// DeleteResourceTags removes the supplied tag keys from the ACM certificate, or returns an error
// if the oepration fails
func (a ACM) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {
		var awsTags []types.Tag
		for k := range tags {
			awsTags = append(awsTags, types.Tag{
				Key: aws.String(k),
			})
		}
		_, err := a.RemoveTagsFromCertificate(ctx, &acm.RemoveTagsFromCertificateInput{
			CertificateArn: aws.String(arn),
			Tags:           awsTags,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for acm certificate %s", arn)
		}
		log.WithFields(log.Fields{
			"Certificate ARN": arn,
		}).Infof("%v tags deleted from acm certificate", len(tags))
	}
	return nil
}
