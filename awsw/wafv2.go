package awsw

import (
	"context"
	"strings"

	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	waftypes "github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	"github.com/pkg/errors"
)

// WAFV2 is a thin wrapper around the AWS WAFv2 SDK client for CloudFront-scoped
// (global) web ACL lookups. The underlying client is pinned to us-east-1, the only
// region AWS serves the CLOUDFRONT scope from.
type WAFV2 struct {
	*wafv2.Client
}

// NewWAFV2 creates a new instance of the WAFV2 wrapper.
func NewWAFV2(ctx context.Context, providerName string) WAFV2 {
	return WAFV2{client.WAFV2Global(ctx, providerName)}
}

// WebACLArnForIdentifier resolves a CloudFront-scoped (global) web ACL ARN from an
// identifier that is either the ARN itself ("arn:" prefix, used as-is) or the web ACL
// Name, resolved by paginating ListWebACLs. An exact name match wins; otherwise a
// case-insensitive match is accepted when it is unambiguous. Returns an error when no
// web ACL matches.
func (w WAFV2) WebACLArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	if strings.HasPrefix(identifier, "arn:") {
		return aws.String(identifier), nil
	}

	var foldMatches []string
	var marker *string
	for {
		out, err := w.ListWebACLs(ctx, &wafv2.ListWebACLsInput{
			Scope:      waftypes.ScopeCloudfront,
			NextMarker: marker,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error listing wafv2 web acls (global/CLOUDFRONT scope)")
		}
		for _, acl := range out.WebACLs {
			if aws.ToString(acl.Name) == identifier {
				return acl.ARN, nil
			}
			if strings.EqualFold(aws.ToString(acl.Name), identifier) {
				foldMatches = append(foldMatches, aws.ToString(acl.ARN))
			}
		}
		next := aws.ToString(out.NextMarker)
		if next == "" {
			break
		}
		// WAF returns a NextMarker whenever more objects *may* be available; an
		// unchanged marker violates the pagination contract and would loop forever,
		// so surface it instead of papering over it.
		if next == aws.ToString(marker) {
			return nil, errors.Errorf("wafv2 ListWebACLs returned an unchanged NextMarker %q; aborting pagination", next)
		}
		marker = out.NextMarker
	}

	switch len(foldMatches) {
	case 1:
		return aws.String(foldMatches[0]), nil
	case 0:
		return nil, errors.Errorf("wafv2 web acl not found for name %q (global/CLOUDFRONT scope)", identifier)
	default:
		return nil, errors.Errorf("multiple wafv2 web acls match name %q case-insensitively: %v", identifier, foldMatches)
	}
}
