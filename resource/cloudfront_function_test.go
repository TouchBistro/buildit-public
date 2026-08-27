package resource

import (
	"context"
	"strings"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/aws/aws-sdk-go-v2/aws"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

const testFnCode = `function handler(event) { return event.request; }`

// testFn returns a normalized baseline function resource.
func testFn() *CloudfrontFunction {
	r := &CloudfrontFunction{
		Name:    "test-fn",
		Comment: aws.String("test function"),
		Code:    testFnCode,
	}
	r.Normalize(context.Background())
	return r
}

// existingFn returns an AWS-side function matching testFn exactly.
func existingFn() *awsw.ExistingFunction {
	return &awsw.ExistingFunction{
		ARN:    "arn:aws:cloudfront::111122223333:function/test-fn",
		ETag:   "E123",
		Status: "UNASSOCIATED",
		Config: cftypes.FunctionConfig{
			Comment: aws.String("test function"),
			Runtime: cftypes.FunctionRuntimeCloudfrontJs20,
		},
		Code: []byte(testFnCode),
	}
}

func TestCloudfrontFunction_Normalize(t *testing.T) {
	t.Run("defaults runtime to cloudfront-js-2.0", func(t *testing.T) {
		r := &CloudfrontFunction{Name: "fn", Code: testFnCode}
		r.Normalize(context.Background())
		assert.Equal(t, CloudfrontFunctionRuntimeJS20, aws.ToString(r.Runtime))
	})

	t.Run("preserves explicit runtime", func(t *testing.T) {
		r := &CloudfrontFunction{Name: "fn", Code: testFnCode, Runtime: aws.String(CloudfrontFunctionRuntimeJS10)}
		r.Normalize(context.Background())
		assert.Equal(t, CloudfrontFunctionRuntimeJS10, aws.ToString(r.Runtime))
	})

	t.Run("merges global tags without overriding resource tags", func(t *testing.T) {
		r := &CloudfrontFunction{
			Name: "fn", Code: testFnCode,
			Tags:       map[string]string{"team": "example-team"},
			GlobalTags: map[string]string{"team": "global-team", "env": "example"},
		}
		r.Normalize(context.Background())
		assert.Equal(t, "example-team", r.Tags["team"])
		assert.Equal(t, "example", r.Tags["env"])
	})

	t.Run("nil tags map is initialized", func(t *testing.T) {
		r := &CloudfrontFunction{Name: "fn", Code: testFnCode, GlobalTags: map[string]string{"env": "example"}}
		r.Normalize(context.Background())
		assert.Equal(t, "example", r.Tags["env"])
	})
}

func TestCloudfrontFunction_Validate(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(r *CloudfrontFunction)
		errText string
	}{
		{
			name:   "valid",
			mutate: func(r *CloudfrontFunction) {},
		},
		{
			name:    "missing code",
			mutate:  func(r *CloudfrontFunction) { r.Code = "" },
			errText: "code is required",
		},
		{
			name:    "code over 10KB",
			mutate:  func(r *CloudfrontFunction) { r.Code = strings.Repeat("a", 10*1024+1) },
			errText: "the CloudFront limit",
		},
		{
			name:    "invalid runtime",
			mutate:  func(r *CloudfrontFunction) { r.Runtime = aws.String("nodejs20.x") },
			errText: "runtime must be one of",
		},
		{
			name:    "invalid name characters",
			mutate:  func(r *CloudfrontFunction) { r.Name = "bad name!" },
			errText: "must match",
		},
		{
			name:    "name too long",
			mutate:  func(r *CloudfrontFunction) { r.Name = strings.Repeat("a", 65) },
			errText: "must match",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := testFn()
			tc.mutate(r)
			err := r.Validate(context.Background())
			if tc.errText == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.errText)
		})
	}
}

func TestCloudfrontFunction_ComputeDiff(t *testing.T) {
	liveInSync := []byte(testFnCode)

	t.Run("no changes returns nil", func(t *testing.T) {
		assert.Nil(t, testFn().computeDiff(context.Background(), existingFn(), liveInSync))
	})

	t.Run("nil comment equals empty comment", func(t *testing.T) {
		r := testFn()
		r.Comment = nil
		existing := existingFn()
		existing.Config.Comment = aws.String("")
		assert.Nil(t, r.computeDiff(context.Background(), existing, liveInSync))
	})

	t.Run("code change", func(t *testing.T) {
		r := testFn()
		r.Code = "function handler(event) { return event.response; }"
		diff := r.computeDiff(context.Background(), existingFn(), liveInSync)
		require.NotNil(t, diff)
		assert.True(t, diff.configChanged)
		assert.False(t, diff.publishOnly)
		require.Len(t, diff.Messages, 1)
		assert.Contains(t, diff.Messages[0], "code: changed")
	})

	t.Run("comment change", func(t *testing.T) {
		r := testFn()
		r.Comment = aws.String("new comment")
		diff := r.computeDiff(context.Background(), existingFn(), liveInSync)
		require.NotNil(t, diff)
		assert.True(t, diff.configChanged)
		assert.Contains(t, diff.Messages[0], "comment: test function -> new comment")
	})

	t.Run("runtime change", func(t *testing.T) {
		r := testFn()
		r.Runtime = aws.String(CloudfrontFunctionRuntimeJS10)
		diff := r.computeDiff(context.Background(), existingFn(), liveInSync)
		require.NotNil(t, diff)
		assert.True(t, diff.configChanged)
		assert.Contains(t, diff.Messages[0], "runtime: cloudfront-js-2.0 -> cloudfront-js-1.0")
	})

	t.Run("never published", func(t *testing.T) {
		diff := testFn().computeDiff(context.Background(), existingFn(), nil)
		require.NotNil(t, diff)
		assert.False(t, diff.configChanged)
		assert.True(t, diff.publishOnly)
		assert.Contains(t, diff.Messages[0], "never been published")
	})

	t.Run("live stage stale", func(t *testing.T) {
		diff := testFn().computeDiff(context.Background(), existingFn(), []byte("old code"))
		require.NotNil(t, diff)
		assert.False(t, diff.configChanged)
		assert.True(t, diff.publishOnly)
		assert.Contains(t, diff.Messages[0], "out of date")
	})

	t.Run("code change wins over stale live", func(t *testing.T) {
		r := testFn()
		r.Code = "new code"
		diff := r.computeDiff(context.Background(), existingFn(), []byte("old code"))
		require.NotNil(t, diff)
		assert.True(t, diff.configChanged)
		assert.False(t, diff.publishOnly)
	})

	t.Run("tags-only change does not force a republish", func(t *testing.T) {
		r := testFn()
		r.Tags = map[string]string{"env": "example"}
		diff := r.computeDiff(context.Background(), existingFn(), liveInSync)
		require.NotNil(t, diff)
		assert.True(t, diff.tagsDiff)
		assert.False(t, diff.configChanged)
		assert.False(t, diff.publishOnly)
		assert.Equal(t, map[string]string{"env": "example"}, diff.tagDiff.Upserts())
		require.Len(t, diff.Messages, 1)
		assert.Contains(t, diff.Messages[0], "tag env")
	})

	t.Run("removed tag is detected", func(t *testing.T) {
		r := testFn()
		existing := existingFn()
		existing.Tags = map[string]string{"env": "example"}
		diff := r.computeDiff(context.Background(), existing, liveInSync)
		require.NotNil(t, diff)
		assert.True(t, diff.tagsDiff)
		assert.Equal(t, []string{"env"}, diff.tagDiff.DeletedKeys())
	})

	t.Run("matching tags produce no diff", func(t *testing.T) {
		r := testFn()
		r.Tags = map[string]string{"env": "example"}
		existing := existingFn()
		existing.Tags = map[string]string{"env": "example"}
		assert.Nil(t, r.computeDiff(context.Background(), existing, liveInSync))
	})

	t.Run("diff carries etag, arn, and existing resource", func(t *testing.T) {
		r := testFn()
		r.Code = "new code"
		diff := r.computeDiff(context.Background(), existingFn(), liveInSync)
		require.NotNil(t, diff)
		assert.Equal(t, "E123", diff.etag)
		assert.Equal(t, "arn:aws:cloudfront::111122223333:function/test-fn", diff.arn)
		assert.NotNil(t, diff.AWSResource())
	})
}

// TestCloudfrontFunction_YAMLUnmarshal verifies the YAML tags map a config (as it would
// appear under resources.cloudfront-function) onto the struct, and that Normalize +
// Validate accept it.
func TestCloudfrontFunction_YAMLUnmarshal(t *testing.T) {
	const doc = `
my-fn:
  comment: "adds security headers"
  runtime: cloudfront-js-1.0
  code: |
    function handler(event) {
      return event.request;
    }
`

	var parsed map[string]CloudfrontFunction
	require.NoError(t, yaml.Unmarshal([]byte(doc), &parsed))

	r, ok := parsed["my-fn"]
	require.True(t, ok)

	assert.Equal(t, "adds security headers", aws.ToString(r.Comment))
	assert.Equal(t, CloudfrontFunctionRuntimeJS10, aws.ToString(r.Runtime))
	assert.Contains(t, r.Code, "return event.request;")

	r.Name = "my-fn"
	r.Normalize(context.Background())
	assert.NoError(t, r.Validate(context.Background()))
}
