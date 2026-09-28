package awsw

import (
	"testing"

	"github.com/TouchBistro/buildit/util"
)

// acmCertIDRegex decides whether an identifier is treated as a certificate id
// (matched against the ARN's trailing "certificate/{id}" segment) or as a domain
// name (matched against the certificate's DomainName). A misclassification makes
// the lookup silently search the wrong field, so the boundary is pinned here.
func TestACMCertIDRegex(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		isID       bool
	}{
		{"lowercase uuid", "5a420568-9f60-48de-a513-427adca04c0a", true},
		{"uppercase uuid", "5A420568-9F60-48DE-A513-427ADCA04C0A", true},
		{"domain name", "api.example.com", false},
		{"wildcard domain", "*.api.example.com", false},
		{"hyphenated name with four dashes", "foo-bar-baz-qux-quux", false},
		{"uuid with wrong group lengths", "5a420568-9f60-48de-a513427a-dca04c0a", false},
		{"non-hex uuid shape", "5a42z568-9f60-48de-a513-427adca04c0a", false},
		{"arn", "arn:aws:acm:us-east-1:123456789012:certificate/5a420568-9f60-48de-a513-427adca04c0a", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acmCertIDRegex.MatchString(tt.identifier); got != tt.isID {
				t.Errorf("acmCertIDRegex.MatchString(%q) = %v, want %v", tt.identifier, got, tt.isID)
			}
		})
	}
}

// pickCertificateByResourceID is the disambiguation policy for domains shared by several
// certificates (DEVOPS-8880): exactly one buildit:resource-id-tagged candidate wins;
// zero tagged means no pick (caller falls back to first-match); more than one tagged is
// surfaced so the caller can fail loud.
func TestPickCertificateByResourceID(t *testing.T) {
	const key = util.BuilditResourceIDTagKey
	arnA, arnB, arnC := "arn:aws:acm::123456789012:certificate/aaa", "arn:aws:acm::123456789012:certificate/bbb", "arn:aws:acm::123456789012:certificate/ccc"

	tests := []struct {
		name         string
		candidates   []certCandidate
		want         string
		pick         *string
		taggedLen    int
		unclaimedLen int
	}{
		{
			name: "single tagged candidate wins",
			candidates: []certCandidate{
				{arn: arnA, tags: map[string]string{"team": "example"}},
				{arn: arnB, tags: map[string]string{key: "api.example.com"}},
				{arn: arnC, tags: nil},
			},
			want: "api.example.com", pick: &arnB, taggedLen: 1, unclaimedLen: 2,
		},
		{
			name: "no tagged candidate yields no pick; the untagged one is unclaimed",
			candidates: []certCandidate{
				{arn: arnA, tags: map[string]string{}},
				{arn: arnB, tags: map[string]string{key: "other.example.com"}},
			},
			want: "api.example.com", pick: nil, taggedLen: 0, unclaimedLen: 1,
		},
		{
			// Two same-CN certificates already created by two other resources in the
			// same config: nothing is unclaimed, so the third resource creates its own
			// (the case that failed as "none carries buildit:resource-id" before).
			name: "every same-CN certificate claimed by another resource leaves nothing unclaimed",
			candidates: []certCandidate{
				{arn: arnA, tags: map[string]string{key: "example-cert-dupe"}},
				{arn: arnB, tags: map[string]string{key: "example-cert-ncerts"}},
			},
			want: "example-cert-orig", pick: nil, taggedLen: 0, unclaimedLen: 0,
		},
		{
			name: "two tagged candidates stay ambiguous",
			candidates: []certCandidate{
				{arn: arnA, tags: map[string]string{key: "api.example.com"}},
				{arn: arnB, tags: map[string]string{key: "api.example.com"}},
			},
			want: "api.example.com", pick: nil, taggedLen: 2, unclaimedLen: 0,
		},
		{
			name: "wildcard domain matches only through SafeTagValue",
			candidates: []certCandidate{
				// The writer stamped SafeTagValue("*.example.com") — the reader must
				// compare against the same sanitized form, never the raw domain.
				{arn: arnA, tags: map[string]string{key: "_.example.com"}},
			},
			want: util.SafeTagValue("*.example.com"), pick: &arnA, taggedLen: 1, unclaimedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickCertificateByResourceID(tt.candidates, tt.want)
			if len(got.tagged) != tt.taggedLen {
				t.Errorf("tagged = %v, want %v entries", got.tagged, tt.taggedLen)
			}
			if len(got.unclaimed) != tt.unclaimedLen {
				t.Errorf("unclaimed = %v, want %v entries", got.unclaimed, tt.unclaimedLen)
			}
			switch {
			case tt.pick == nil && got.picked != nil:
				t.Errorf("pick = %v, want nil", *got.picked)
			case tt.pick != nil && got.picked == nil:
				t.Errorf("pick = nil, want %v", *tt.pick)
			case tt.pick != nil && got.picked != nil && *got.picked != *tt.pick:
				t.Errorf("pick = %v, want %v", *got.picked, *tt.pick)
			}
		})
	}
}
