package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestFirehoseDeliveryStreamArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		f := awsw.Firehose{}
		ctx := context.Background()
		identifier := "arn:aws:firehose:us-east-1:123456789012:deliverystream/my-stream"
		arn, err := f.DeliveryStreamArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		f := awsw.Firehose{}
		ctx := context.Background()
		identifier := "invalid::my-stream"
		_, err := f.DeliveryStreamArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})

	t.Run("Provider prefix with ARN resource passes through", func(t *testing.T) {
		f := awsw.Firehose{}
		ctx := context.Background()
		streamArn := "arn:aws:firehose:us-east-1:123456789012:deliverystream/my-stream"
		identifier := "staging::" + streamArn
		result, err := f.DeliveryStreamArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, streamArn, *result)
	})
}
