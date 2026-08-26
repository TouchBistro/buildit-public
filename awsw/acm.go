package awsw

import (
	"context"

	"fmt"
	"regexp"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
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
// segment of the ARN, and anything else matches by Domain Name. Both lookups scan
// ListCertificates in the wrapper's account/region, so the returned ARN is exactly what
// AWS stores — account, region, and partition are never guessed.
func (a ACM) CertificateArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
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

	isID := acmCertIDRegex.MatchString(resource)
	idSuffix := "/" + strings.ToLower(resource)

	var nextToken *string
	for {
		out, err := acmService.ListCertificates(ctx, &acm.ListCertificatesInput{
			// The API's default filter hides certificates whose key type is not RSA_2048
			// (e.g. ECDSA); list every key type so all certificates are candidates.
			Includes:  &types.Filters{KeyTypes: types.KeyAlgorithm("").Values()},
			NextToken: nextToken,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list certificates")
		}

		for _, cert := range out.CertificateSummaryList {
			if isID {
				if strings.HasSuffix(strings.ToLower(aws.ToString(cert.CertificateArn)), idSuffix) {
					return cert.CertificateArn, nil
				}
			} else if aws.ToString(cert.DomainName) == resource {
				return cert.CertificateArn, nil
			}
		}

		if out.NextToken == nil {
			break
		}
		nextToken = out.NextToken
	}

	if isID {
		return nil, errors.Errorf("certificate not found for id %q", resource)
	}
	return nil, errors.Errorf("certificate not found for domain %q", resource)
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
