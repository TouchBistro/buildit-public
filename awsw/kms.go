package awsw

import (
	"context"

	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/pkg/errors"
)

type KMS struct {
	*kms.Client
}

// NewKMS creates a new instance of the KMS wrapper.
func NewKMS(ctx context.Context, providerName string) KMS {
	return KMS{client.KMS(ctx, providerName)}
}

// ResolveKeyArn resolves a KMS key reference (alias, key id, or ARN) to its canonical key ARN.
// DescribeKey accepts any of those forms, so this gives a single comparable value regardless of
// how the key was supplied. An empty reference returns an empty string.
func (k KMS) ResolveKeyArn(ctx context.Context, keyRef string) (string, error) {
	if keyRef == "" {
		return "", nil
	}
	out, err := k.DescribeKey(ctx, &kms.DescribeKeyInput{
		KeyId: aws.String(keyRef),
	})
	if err != nil {
		return "", errors.Wrapf(err, "failed to describe kms key %q", keyRef)
	}
	if out.KeyMetadata == nil {
		return "", errors.Errorf("kms key %q returned no metadata", keyRef)
	}
	return aws.ToString(out.KeyMetadata.Arn), nil
}
