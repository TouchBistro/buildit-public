package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The resource name is the certificate's identity (Identifier and buildit:resource-id);
// the CN lives in DomainName and defaults to the name. Separating the two is what lets
// two certificates share a CN in one config scope, so the default and the override are
// pinned here.
func TestACMCertificate_DomainNameDefaultsToName(t *testing.T) {
	t.Run("domainName defaults to the resource name", func(t *testing.T) {
		c := ACMCertificate{
			Name:             "api.example.com",
			ValidationDomain: "default/example.com",
		}
		c.Normalize(context.Background())

		assert.Equal(t, "api.example.com", c.DomainName)
		assert.Equal(t, "api.example.com", c.Identifier())
	})

	t.Run("an explicit domainName is kept and does not change the identity", func(t *testing.T) {
		c := ACMCertificate{
			Name:             "example-api-cert-blue",
			DomainName:       "api.example.com",
			ValidationDomain: "default/example.com",
		}
		c.Normalize(context.Background())

		assert.Equal(t, "api.example.com", c.DomainName)
		assert.Equal(t, "example-api-cert-blue", c.Identifier())
	})
}

// dnsValidationDomainName is optional (DEVOPS-8968): empty stays empty so the zone is
// discovered per validation record, while a supplied value is normalized exactly as before.
func TestACMCertificate_Normalize_ValidationDomain(t *testing.T) {
	t.Run("omitted stays empty", func(t *testing.T) {
		c := ACMCertificate{Name: "api.example.com"}
		c.Normalize(context.Background())
		assert.Equal(t, "", c.ValidationDomain)
	})

	t.Run("supplied gets the trailing dot", func(t *testing.T) {
		c := ACMCertificate{Name: "api.example.com", ValidationDomain: "default/example.com"}
		c.Normalize(context.Background())
		assert.Equal(t, "default/example.com.", c.ValidationDomain)
	})

	t.Run("supplied with a trailing dot is unchanged", func(t *testing.T) {
		c := ACMCertificate{Name: "api.example.com", ValidationDomain: "dns::example.com."}
		c.Normalize(context.Background())
		assert.Equal(t, "dns::example.com.", c.ValidationDomain)
	})
}

// An explicit dnsValidationDomainName takes precedence over discovery and keeps accepting
// both provider delimiters; a bare zone means the main provider.
func TestACMCertificate_ExplicitValidationTarget(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantProvider string
		wantZone     string
	}{
		{"legacy slash delimiter", "dns/example.com.", "dns", "example.com."},
		{"double-colon delimiter", "dns::example.com.", "dns", "example.com."},
		{"bare zone defaults to main provider", "example.com.", "main", "example.com."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, zone := explicitValidationTarget(tt.input)
			assert.Equal(t, tt.wantProvider, provider)
			assert.Equal(t, tt.wantZone, zone)
		})
	}
}

// resolveValidationTarget with an explicit value must not reach AWS: the zone is whatever the
// operator named, in the provider they named, and its id is left for Route53Record to look up.
func TestACMCertificate_ResolveValidationTarget_ExplicitPrecedence(t *testing.T) {
	c := ACMCertificate{
		Name:             "api.example.com",
		ValidationDomain: "dns/example.com.",
		BaseResource:     BaseResource{Context: Context{ProviderName: "main"}},
	}
	c.Normalize(context.Background())
	target, err := c.resolveValidationTarget(context.Background(), "api.example.com", "_abc.api.example.com.")
	assert.NoError(t, err)
	assert.Equal(t, validationTarget{provider: "dns", zone: "example.com."}, target)
}

// Normalize folds both explicit fields into one per-domain lookup (validationZones) so the
// apply, destroy and plan paths never disagree on where a record goes. A lone
// dnsValidationDomainName fans out to every domain; dnsValidationZones is normalized per entry
// (case, trailing dots) and wins over the legacy field; neither leaves the map nil.
func TestACMCertificate_Normalize_ValidationZones(t *testing.T) {
	t.Run("omitted leaves placement to discovery", func(t *testing.T) {
		c := ACMCertificate{Name: "api.example.com", SAN: []string{"api.example.io"}}
		c.Normalize(context.Background())
		assert.Nil(t, c.validationZones)
	})

	t.Run("legacy field covers the CN and every SAN", func(t *testing.T) {
		c := ACMCertificate{
			Name:             "api.example.com",
			SAN:              []string{"*.api.example.com", "API.example.io."},
			ValidationDomain: "dns::example.com",
		}
		c.Normalize(context.Background())
		assert.Equal(t, map[string]string{
			"api.example.com.":   "dns::example.com.",
			"*.api.example.com.": "dns::example.com.",
			"api.example.io.":    "dns::example.com.",
		}, c.validationZones)
	})

	t.Run("per-domain map is normalized and wins over the legacy field", func(t *testing.T) {
		c := ACMCertificate{
			Name: "api.example.com",
			SAN:  []string{"*.api.example.com", "api.example.io"},
			ValidationZones: map[string]string{
				"API.example.com":    "dns::example.com",
				"*.api.example.com.": "dns/example.com.",
				"api.example.io":     "example.io",
			},
			ValidationDomain: "dns::other.example.com",
		}
		c.Normalize(context.Background())
		assert.Equal(t, map[string]string{
			"api.example.com.":   "dns::example.com.",
			"*.api.example.com.": "dns/example.com.",
			"api.example.io.":    "example.io.",
		}, c.validationZones)
	})
}

// dnsValidationZones is all-or-nothing: a partial map would silently mix explicit placement
// with discovery, so Validate demands an entry for the CN and every SAN, rejects keys that
// are not on the certificate, and refuses to combine it with dnsValidationDomainName. Every
// problem is reported at once.
func TestACMCertificate_Validate_ValidationZones(t *testing.T) {
	tests := []struct {
		name     string
		cert     ACMCertificate
		wantMsgs []string
	}{
		{
			name: "no placement fields is valid (discovery)",
			cert: ACMCertificate{Name: "api.example.com", SAN: []string{"api.example.io"}},
		},
		{
			name: "legacy field alone is valid",
			cert: ACMCertificate{Name: "api.example.com", SAN: []string{"api.example.io"}, ValidationDomain: "dns::example.com"},
		},
		{
			name: "complete map is valid, keys compared case- and dot-insensitively",
			cert: ACMCertificate{
				Name: "api.example.com",
				SAN:  []string{"*.api.example.com", "api.example.io"},
				ValidationZones: map[string]string{
					"API.example.com.":  "dns::example.com",
					"*.api.example.com": "dns::example.com",
					"api.example.io":    "dns::example.io",
				},
			},
		},
		{
			name: "missing domains, foreign keys, empty zones and the legacy field are all reported",
			cert: ACMCertificate{
				Name:             "api.example.com",
				SAN:              []string{"api.example.io", "api.example.net"},
				ValidationDomain: "dns::example.com",
				ValidationZones: map[string]string{
					"api.example.io":  "",
					"www.example.com": "dns::example.com",
				},
			},
			wantMsgs: []string{
				"set either dnsValidationZones or dnsValidationDomainName, not both",
				`dnsValidationZones entry "api.example.io" has no hosted zone`,
				`dnsValidationZones key "www.example.com" is not the certificate domain or a san`,
				`dnsValidationZones has no entry for "api.example.com"; list every domain on the certificate, or omit the field to discover zones`,
				`dnsValidationZones has no entry for "api.example.net"; list every domain on the certificate, or omit the field to discover zones`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cert.Normalize(context.Background())
			err := tt.cert.Validate(context.Background())
			if tt.wantMsgs == nil {
				assert.NoError(t, err)
				return
			}
			var ve *ValidationError
			if assert.ErrorAs(t, err, &ve) {
				assert.Equal(t, "certificate", ve.ResourceType)
				assert.Equal(t, tt.cert.Identifier(), ve.ResourceIdentifier)
				assert.Equal(t, tt.wantMsgs, ve.Messages)
			}
		})
	}
}

// With dnsValidationZones each domain's record goes to the zone named for that domain, in that
// provider, without reaching AWS — the cross-account, multi-zone case discovery cannot serve.
// The domain ACM reports is matched case- and dot-insensitively against the config key.
func TestACMCertificate_ResolveValidationTarget_PerDomain(t *testing.T) {
	c := ACMCertificate{
		Name: "api.example.com",
		SAN:  []string{"*.api.example.com", "api.example.io"},
		ValidationZones: map[string]string{
			"api.example.com":   "dns::example.com",
			"*.api.example.com": "dns::example.com",
			"api.example.io":    "other/example.io",
		},
		BaseResource: BaseResource{Context: Context{ProviderName: "main"}},
	}
	c.Normalize(context.Background())

	tests := []struct {
		domain string
		record string
		want   validationTarget
	}{
		{"api.example.com", "_abc.api.example.com.", validationTarget{provider: "dns", zone: "example.com."}},
		{"*.api.example.com", "_abc.api.example.com.", validationTarget{provider: "dns", zone: "example.com."}},
		{"API.example.io.", "_def.api.example.io.", validationTarget{provider: "other", zone: "example.io."}},
	}
	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			target, err := c.resolveValidationTarget(context.Background(), tt.domain, tt.record)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, target)
		})
	}
}

// The relative record name handed to Route53Record is the FQDN minus the zone.
func TestACMCertificate_ValidationRecordName(t *testing.T) {
	tests := []struct {
		name   string
		fqdn   string
		zone   string
		expect string
	}{
		{"record under the apex zone", "_abc.api.example.com.", "example.com.", "_abc.api"},
		{"record under a nested zone", "_abc.uuid.service.example.com.", "service.example.com.", "_abc.uuid"},
		{"record not under the zone is passed through", "_abc.api.example.io.", "example.com.", "_abc.api.example.io."},
		{"label boundary is respected", "_abc.notexample.com.", "example.com.", "_abc.notexample.com."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, validationRecordName(tt.fqdn, tt.zone))
		})
	}
}

// Plan-time discovery checks the CN and every SAN — each may sit in a different zone — with
// wildcards reduced to their parent (the validation record for *.example.com is under
// example.com) and duplicates dropped.
func TestACMCertificate_ValidationLookupDomains(t *testing.T) {
	tests := []struct {
		name    string
		domains []string
		expect  []string
	}{
		{"CN only", []string{"api.example.com"}, []string{"api.example.com."}},
		{"SANs across zones are all kept", []string{"api.example.com", "api.example.io", "www.example.com"},
			[]string{"api.example.com.", "api.example.io.", "www.example.com."}},
		{"wildcard collapses onto its parent and de-dupes", []string{"example.com", "*.example.com", "*.Example.com"},
			[]string{"example.com."}},
		{"trailing dots are normalized", []string{"api.example.com.", "api.example.com"}, []string{"api.example.com."}},
		{"nothing to look up", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, validationLookupDomains(tt.domains))
		})
	}
}

// Domains with an explicit placement never reach discovery: a lone dnsValidationDomainName
// covers them all, and a complete dnsValidationZones covers each by name, so the plan-time
// zone check makes no AWS call in either case. The operator owns the placement decision.
func TestACMCertificate_CheckValidationZones_SkipsWhenExplicit(t *testing.T) {
	t.Run("legacy single zone", func(t *testing.T) {
		c := ACMCertificate{Name: "api.example.com", SAN: []string{"api.example.io"}, ValidationDomain: "dns/example.com."}
		c.Normalize(context.Background())
		assert.Nil(t, c.discoveredDomains())
		assert.NoError(t, c.checkValidationZones(context.Background()))
	})

	t.Run("per-domain zones", func(t *testing.T) {
		c := ACMCertificate{
			Name:            "api.example.com",
			SAN:             []string{"api.example.io"},
			ValidationZones: map[string]string{"api.example.com": "dns::example.com", "api.example.io": "dns::example.io"},
		}
		c.Normalize(context.Background())
		assert.Nil(t, c.discoveredDomains())
		assert.NoError(t, c.checkValidationZones(context.Background()))
	})

	t.Run("discovery keeps every domain", func(t *testing.T) {
		c := ACMCertificate{Name: "api.example.com", SAN: []string{"*.example.io"}}
		c.Normalize(context.Background())
		assert.Equal(t, []string{"api.example.com", "*.example.io"}, c.discoveredDomains())
	})
}
