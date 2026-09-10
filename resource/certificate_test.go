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
