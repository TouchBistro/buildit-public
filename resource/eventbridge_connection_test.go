package resource

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validConnectionParameters() *EventBridgeConnectionParameters {
	return &EventBridgeConnectionParameters{
		ApiKeyName:  aws.String("Authorization"),
		ApiKeyValue: aws.String("example-secret:api_key"),
	}
}

func TestEventBridgeApiConnection_Validate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		connection EventBridgeApiConnection
		wantErr    bool
	}{
		{
			name: "valid connection",
			connection: EventBridgeApiConnection{
				Name:                 "example-connection",
				ConnectionParameters: validConnectionParameters(),
			},
			wantErr: false,
		},
		{
			name: "missing connection parameters",
			connection: EventBridgeApiConnection{
				Name: "example-connection",
			},
			wantErr: true,
		},
		{
			name: "tags are rejected: AWS does not support tagging connections",
			connection: EventBridgeApiConnection{
				Name:                 "example-connection",
				ConnectionParameters: validConnectionParameters(),
				Tags:                 map[string]string{"team": "example-team"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.connection.Validate(ctx)
			if !tt.wantErr {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

func TestEventBridgeApiConnection_Validate_TagsErrorMessage(t *testing.T) {
	connection := EventBridgeApiConnection{
		Name:                 "example-connection",
		ConnectionParameters: validConnectionParameters(),
		Tags:                 map[string]string{"team": "example-team"},
	}

	err := connection.Validate(context.Background())
	require.Error(t, err)

	vErr, ok := err.(*ValidationError)
	require.True(t, ok, "expected a ValidationError, got %T", err)
	assert.True(t, strings.Contains(strings.Join(vErr.Messages, "\n"), "tags are not supported"),
		"validation messages should say tags are unsupported: %v", vErr.Messages)
}
