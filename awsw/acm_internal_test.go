package awsw

import "testing"

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
