package awsw_test

import (
	"context"
	"testing"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/stretchr/testify/assert"
)

func TestEC2VpcArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		e := awsw.EC2{}
		ctx := context.Background()
		identifier := "arn:aws:ec2:us-east-1:123456789012:vpc/vpc-12345678"
		arn, err := e.VpcArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})

	t.Run("Returns error for invalid provider", func(t *testing.T) {
		e := awsw.EC2{}
		ctx := context.Background()
		identifier := "invalid::vpc-12345678"
		_, err := e.VpcArnForIdentifier(ctx, identifier)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider \"invalid\" not found")
	})
}

func TestEC2SubnetArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		e := awsw.EC2{}
		ctx := context.Background()
		identifier := "arn:aws:ec2:us-east-1:123456789012:subnet/subnet-12345678"
		arn, err := e.SubnetArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})
}

func TestEC2SecurityGroupArnForIdentifier(t *testing.T) {
	t.Run("Resolves direct ARN", func(t *testing.T) {
		e := awsw.EC2{}
		ctx := context.Background()
		identifier := "arn:aws:ec2:us-east-1:123456789012:security-group/sg-12345678"
		arn, err := e.SecurityGroupArnForIdentifier(ctx, identifier)
		assert.NoError(t, err)
		assert.Equal(t, identifier, *arn)
	})
}
