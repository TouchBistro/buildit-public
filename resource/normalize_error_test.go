package resource

import (
	"context"
	"testing"

	"github.com/TouchBistro/awesome/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Normalize reports a failed lookup as an error (DEVOPS-8926). An unknown provider name is
// the one failure reachable without AWS: the client accessor refuses it before any call.
func TestNormalizeReturnsLookupError(t *testing.T) {
	s := &SNSSubscription{TopicName: "example-topic", EndpointName: "example-function"}
	s.Context.ProviderName = "example-missing"

	err := s.Normalize(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, providers.ErrUnknownProvider)
	assert.Contains(t, err.Error(), `provider "example-missing" not found`)
	assert.Nil(t, s.TopicArn, "a failed Normalize leaves the resolved fields untouched")
}
