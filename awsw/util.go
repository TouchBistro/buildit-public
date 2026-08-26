package awsw

import (
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
)

// ParseIdentifier decomposes an identifier into a resource name and a provider name.
// It supports ARNs, which are returned as-is with no provider.
// It supports provider prefixes delimited by :: or /. :: has priority.
func ParseIdentifier(identifier string) (string, string) {
	if arn.IsARN(identifier) {
		return identifier, ""
	}

	if parts := strings.SplitN(identifier, "::", 2); len(parts) == 2 {
		return parts[1], parts[0]
	}

	if parts := strings.SplitN(identifier, "/", 2); len(parts) == 2 {
		return parts[1], parts[0]
	}

	return identifier, ""
}
