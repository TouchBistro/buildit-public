package awsw_test

import (
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestParseIdentifier(t *testing.T) {
	tests := []struct {
		name             string
		identifier       string
		expectedResource string
		expectedProvider string
	}{
		{
			name:             "Direct ARN",
			identifier:       "arn:aws:s3:::my-bucket",
			expectedResource: "arn:aws:s3:::my-bucket",
			expectedProvider: "",
		},
		{
			name:             "Provider with :: delimiter",
			identifier:       "my-provider::my-bucket",
			expectedResource: "my-bucket",
			expectedProvider: "my-provider",
		},
		{
			name:             "Provider with / delimiter",
			identifier:       "my-provider/my-bucket",
			expectedResource: "my-bucket",
			expectedProvider: "my-provider",
		},
		{
			name:             "Priority :: over /",
			identifier:       "my-provider::other/bucket",
			expectedResource: "other/bucket",
			expectedProvider: "my-provider",
		},
		{
			name:             "No provider",
			identifier:       "my-bucket",
			expectedResource: "my-bucket",
			expectedProvider: "",
		},
		{
			name:             "Multiple delimiters",
			identifier:       "p1::p2/resource",
			expectedResource: "p2/resource",
			expectedProvider: "p1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resource, provider := awsw.ParseIdentifier(tt.identifier)
			assert.Equal(t, tt.expectedResource, resource)
			assert.Equal(t, tt.expectedProvider, provider)
		})
	}
}
