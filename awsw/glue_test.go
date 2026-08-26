package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestGlueCatalogArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		g := awsw.Glue{}
		ctx := context.Background()
		identifier := "arn:aws:glue:us-east-1:123456789012:catalog"
		arn, err := g.CatalogArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		g := awsw.Glue{}
		ctx := context.Background()
		identifier := "invalid::my-catalog"
		_, err := g.CatalogArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})

	t.Run("Returns error for invalid provider when catalog shortcut", func(t *testing.T) {
		g := awsw.Glue{}
		ctx := context.Background()
		identifier := "invalid::catalog"
		_, err := g.CatalogArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}
