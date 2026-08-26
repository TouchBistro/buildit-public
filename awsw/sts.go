package awsw

import (
	"context"

	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type STS struct {
	*sts.Client
}

// NewSTS creates a new instance of STS wrapper
func NewSTS(ctx context.Context, providerName string) STS {
	return STS{client.STS(ctx, providerName)}
}

// GetCallerIdentity returns the caller identity for the current provider
func (s STS) GetCallerIdentity(ctx context.Context) (*sts.GetCallerIdentityOutput, error) {
	return s.Client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
}

// GetAccountID returns the account ID for the current provider
func (s STS) GetAccountID(ctx context.Context) (string, error) {
	out, err := s.GetCallerIdentity(ctx)
	if err != nil {
		return "", err
	}
	return aws.ToString(out.Account), nil
}
