package resource

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
	"github.com/stretchr/testify/assert"
)

func TestBedrockApplicationInferenceProfile_Validate(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		profile BedrockApplicationInferenceProfile
		wantErr bool
	}{
		{
			name: "valid foundation model arn",
			profile: BedrockApplicationInferenceProfile{
				Name:        "my-profile",
				ModelSource: "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-3-haiku-20240307-v1:0",
			},
			wantErr: false,
		},
		{
			name: "valid system-defined inference profile arn",
			profile: BedrockApplicationInferenceProfile{
				Name:        "my-profile",
				ModelSource: "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3-haiku-20240307-v1:0",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			profile: BedrockApplicationInferenceProfile{
				ModelSource: "arn:aws:bedrock:us-east-1::foundation-model/x",
			},
			wantErr: true,
		},
		{
			name: "missing modelSource",
			profile: BedrockApplicationInferenceProfile{
				Name: "my-profile",
			},
			wantErr: true,
		},
		{
			name: "non-arn modelSource",
			profile: BedrockApplicationInferenceProfile{
				Name:        "my-profile",
				ModelSource: "anthropic.claude-3-haiku-20240307-v1:0",
			},
			wantErr: true,
		},
		{
			name: "non-source bedrock arn (guardrail) rejected",
			profile: BedrockApplicationInferenceProfile{
				Name:        "my-profile",
				ModelSource: "arn:aws:bedrock:us-east-1::guardrail/abc123",
			},
			wantErr: true,
		},
		{
			name: "valid gov partition foundation model",
			profile: BedrockApplicationInferenceProfile{
				Name:        "my-profile",
				ModelSource: "arn:aws-us-gov:bedrock:us-gov-west-1::foundation-model/anthropic.claude-3-haiku-20240307-v1:0",
			},
			wantErr: false,
		},
		{
			name: "valid cn partition inference profile",
			profile: BedrockApplicationInferenceProfile{
				Name:        "my-profile",
				ModelSource: "arn:aws-cn:bedrock:cn-north-1:123456789012:inference-profile/cn.anthropic.claude-3-haiku-20240307-v1:0",
			},
			wantErr: false,
		},
		{
			name: "non-bedrock service arn rejected",
			profile: BedrockApplicationInferenceProfile{
				Name:        "my-profile",
				ModelSource: "arn:aws:sagemaker:us-east-1::foundation-model/x",
			},
			wantErr: true,
		},
		{
			name: "name with invalid characters rejected",
			profile: BedrockApplicationInferenceProfile{
				Name:        "my.profile",
				ModelSource: "arn:aws:bedrock:us-east-1::foundation-model/x",
			},
			wantErr: true,
		},
		{
			name: "name starting with separator rejected",
			profile: BedrockApplicationInferenceProfile{
				Name:        "-my-profile",
				ModelSource: "arn:aws:bedrock:us-east-1::foundation-model/x",
			},
			wantErr: true,
		},
		{
			name: "name exceeding 64 characters rejected",
			profile: BedrockApplicationInferenceProfile{
				Name:        strings.Repeat("a", 65),
				ModelSource: "arn:aws:bedrock:us-east-1::foundation-model/x",
			},
			wantErr: true,
		},
		{
			name: "name at 64 character boundary accepted",
			profile: BedrockApplicationInferenceProfile{
				Name:        strings.Repeat("a", 64),
				ModelSource: "arn:aws:bedrock:us-east-1::foundation-model/x",
			},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.profile.Validate(ctx)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBedrockApplicationInferenceProfile_Normalize_MergesGlobalTags(t *testing.T) {
	ctx := context.Background()
	p := &BedrockApplicationInferenceProfile{
		Name:        "my-profile",
		ModelSource: "arn:aws:bedrock:us-east-1::foundation-model/x",
		Tags:        map[string]string{"team": "platform"},
		GlobalTags:  map[string]string{"env": "prod", "team": "shared"},
	}
	p.Normalize(ctx)

	// existing tag preserved, missing global tag added
	assert.Equal(t, "platform", p.Tags["team"])
	assert.Equal(t, "prod", p.Tags["env"])
}

func TestBedrockApplicationInferenceProfile_Normalize_NilTags(t *testing.T) {
	ctx := context.Background()
	p := &BedrockApplicationInferenceProfile{
		Name:        "my-profile",
		ModelSource: "arn:aws:bedrock:us-east-1::foundation-model/x",
		GlobalTags:  map[string]string{"env": "prod"},
	}
	p.Normalize(ctx)

	assert.NotNil(t, p.Tags)
	assert.Equal(t, "prod", p.Tags["env"])
}

func TestBedrockApplicationInferenceProfile_ComputeDiff(t *testing.T) {
	ctx := context.Background()
	const (
		profileArn = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/abc123"
		haikuArn   = "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-3-haiku-20240307-v1:0"
		sonnetArn  = "arn:aws:bedrock:us-east-1::foundation-model/anthropic.claude-3-5-sonnet-20240620-v1:0"
		sysProfile = "arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-3-haiku-20240307-v1:0"
	)

	activeProfile := func(desc string, modelArn string) *bedrocktypes.InferenceProfileSummary {
		return &bedrocktypes.InferenceProfileSummary{
			InferenceProfileArn: aws.String(profileArn),
			Description:         aws.String(desc),
			Status:              bedrocktypes.InferenceProfileStatusActive,
			Models: []bedrocktypes.InferenceProfileModel{
				{ModelArn: aws.String(modelArn)},
			},
		}
	}

	t.Run("does not exist", func(t *testing.T) {
		r := BedrockApplicationInferenceProfile{Name: "my-profile", ModelSource: haikuArn}
		d := r.computeDiff(ctx, nil, nil)
		assert.NotNil(t, d)
		assert.Nil(t, d.AWSResource())
		assert.False(t, d.hasMutableChanges())
		assert.Contains(t, d.Differences()[0], "does not exist")
	})

	t.Run("non-Active status sets statusDiff and skips field comparison", func(t *testing.T) {
		// AWS SDK only exposes Active as a typed const; other statuses are string
		// values the API may return (CREATING/FAILED/DELETING).
		const creatingStatus bedrocktypes.InferenceProfileStatus = "CREATING"
		existing := &bedrocktypes.InferenceProfileSummary{
			InferenceProfileArn: aws.String(profileArn),
			Description:         aws.String("anything"),
			Status:              creatingStatus,
		}
		r := BedrockApplicationInferenceProfile{
			Name:        "my-profile",
			Description: "different-desc",
			ModelSource: haikuArn,
			Tags:        map[string]string{"env": "prod"},
		}
		d := r.computeDiff(ctx, existing, map[string]string{"env": "staging"})
		assert.NotNil(t, d)
		assert.True(t, d.statusDiff)
		assert.Equal(t, creatingStatus, d.status)
		assert.False(t, d.descriptionDiff, "field comparison must be skipped for non-Active")
		assert.False(t, d.tagsDiff, "tag comparison must be skipped for non-Active")
		assert.False(t, d.hasMutableChanges())
		assert.NotNil(t, d.AWSResource())
	})

	t.Run("foundation-model drift detected", func(t *testing.T) {
		existing := activeProfile("same", sonnetArn)
		r := BedrockApplicationInferenceProfile{
			Name:        "my-profile",
			Description: "same",
			ModelSource: haikuArn,
		}
		d := r.computeDiff(ctx, existing, nil)
		assert.NotNil(t, d)
		assert.True(t, d.modelSourceDiff)
		assert.False(t, d.hasMutableChanges(), "modelSource is immutable")
	})

	t.Run("system-defined inference-profile source never flags drift", func(t *testing.T) {
		// AWS returns the resolved foundation-model ARN in Models[], not the system profile ARN.
		existing := activeProfile("same", haikuArn)
		r := BedrockApplicationInferenceProfile{
			Name:        "my-profile",
			Description: "same",
			ModelSource: sysProfile,
		}
		d := r.computeDiff(ctx, existing, nil)
		assert.Nil(t, d, "no diff expected for system-defined source by design")
	})

	t.Run("description drift flagged but not mutable", func(t *testing.T) {
		existing := activeProfile("old", haikuArn)
		r := BedrockApplicationInferenceProfile{
			Name:        "my-profile",
			Description: "new",
			ModelSource: haikuArn,
		}
		d := r.computeDiff(ctx, existing, nil)
		assert.NotNil(t, d)
		assert.True(t, d.descriptionDiff)
		assert.False(t, d.hasMutableChanges())
	})

	t.Run("tags-only diff is mutable", func(t *testing.T) {
		existing := activeProfile("same", haikuArn)
		r := BedrockApplicationInferenceProfile{
			Name:        "my-profile",
			Description: "same",
			ModelSource: haikuArn,
			Tags:        map[string]string{"env": "prod", "team": "platform"},
		}
		awsTags := map[string]string{"env": "staging"} // env changed, team added
		d := r.computeDiff(ctx, existing, awsTags)
		assert.NotNil(t, d)
		assert.True(t, d.tagsDiff)
		assert.True(t, d.hasMutableChanges())
		assert.Equal(t, "prod", d.tagDiff.Upserts()["env"])
		assert.Equal(t, "platform", d.tagDiff.Upserts()["team"])
	})

	t.Run("no diff when everything matches", func(t *testing.T) {
		existing := activeProfile("same", haikuArn)
		r := BedrockApplicationInferenceProfile{
			Name:        "my-profile",
			Description: "same",
			ModelSource: haikuArn,
			Tags:        map[string]string{},
		}
		d := r.computeDiff(ctx, existing, map[string]string{})
		assert.Nil(t, d)
	})

	t.Run("immutable-only diff is not mutable", func(t *testing.T) {
		existing := activeProfile("old", sonnetArn)
		r := BedrockApplicationInferenceProfile{
			Name:        "my-profile",
			Description: "new",
			ModelSource: haikuArn,
		}
		d := r.computeDiff(ctx, existing, nil)
		assert.NotNil(t, d)
		assert.True(t, d.descriptionDiff)
		assert.True(t, d.modelSourceDiff)
		assert.False(t, d.tagsDiff)
		assert.False(t, d.hasMutableChanges())
	})
}
