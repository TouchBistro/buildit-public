package resource

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnsupportedTagsRejected covers the convention for resource types whose AWS API has
// no tagging support: a config that sets tags must fail Validate with the standard
// UnsupportedTagsMessage instead of having the value silently ignored. The lambda-layer
// counterpart lives in resource/lambda; eventbridge-connection is covered in
// eventbridge_connection_test.go.
func TestUnsupportedTagsRejected(t *testing.T) {
	ctx := context.Background()
	tags := map[string]string{"team": "example-team"}

	tests := []struct {
		name     string
		validate func() error
	}{
		{
			name: "cloudwatch-subscriptionfilter",
			validate: func() error {
				r := CWSubscriptionFilter{Name: "example-filter", Destination: "example-fn", LogGroup: "example-group", Tags: tags}
				return r.Validate(ctx)
			},
		},
		{
			name: "eventbridge-apidestination",
			validate: func() error {
				r := EventBridgeApiDestination{Name: "example-dest", Method: aws.String("GET"), Endpoint: aws.String("https://example.com"), InvocationRate: aws.Int32(1), ConnectionName: "example-conn", Tags: tags}
				return r.Validate(ctx)
			},
		},
		{
			name: "route53-record",
			validate: func() error {
				r := Route53Record{Name: "example.example.com", HostedZone: "example.com", Type: aws.String(RecordTypeA), Tags: tags}
				return r.Validate(ctx)
			},
		},
		{
			name: "sns-subscription",
			validate: func() error {
				r := SNSSubscription{Name: "example-sub", TopicName: "example-topic", EndpointName: "example-fn", Tags: tags}
				return r.Validate(ctx)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.validate()
			require.Error(t, err)

			vErr, ok := err.(*ValidationError)
			require.True(t, ok, "expected a ValidationError, got %T", err)
			assert.True(t, strings.Contains(strings.Join(vErr.Messages, "\n"), UnsupportedTagsMessage(tt.name)),
				"validation messages should carry the standard unsupported-tags message for %v: %v", tt.name, vErr.Messages)
		})
	}
}
