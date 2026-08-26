package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestDynamoDBTableArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		d := awsw.DynamoDB{}
		ctx := context.Background()
		identifier := "arn:aws:dynamodb:us-east-1:123456789012:table/my-table"
		arn, err := d.TableArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		d := awsw.DynamoDB{}
		ctx := context.Background()
		identifier := "invalid::my-table"
		_, err := d.TableArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
