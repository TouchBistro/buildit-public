package awsw

import (
	"context"

	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Lambda struct {
	*lambda.Client
}

// NewLambda creates a new instance of Lambda wrapper
func NewLambda(ctx context.Context, providerName string) Lambda {
	return Lambda{client.Lambda(ctx, providerName)}
}

// FunctionArnForIdentifier resolves a Lambda Function ARN from an identifier (ARN or Name).
func (l Lambda) FunctionArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	lambdaService := l
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		lambdaService = NewLambda(ctx, provider)
	}

	// Direct lookup by Name
	out, err := lambdaService.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(resource),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get function %q: %w", resource, err)
	}

	return out.Configuration.FunctionArn, nil
}

// AddResourceTags tags the Lambda resource with the supplied tag keys/value, returns error if the
// operation fails
func (l Lambda) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		_, err := l.TagResource(ctx, &lambda.TagResourceInput{
			Resource: aws.String(arn),
			Tags:     tags,
		})

		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"Arn": arn,
		}).Infof("%v tags added to lambda resource", len(tags))
	}

	return nil
}

// DeleteResourceTags removes the supplied tag keys from the lambda resource, or returns an error
// if the oepration fails
func (l Lambda) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		_, err := l.UntagResource(ctx, &lambda.UntagResourceInput{
			Resource: aws.String(arn),
			TagKeys:  keys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"Arn": arn,
		}).Infof("%v tags deleted from resource", len(tags))
	}
	return nil
}

// LambdaArnForNameAndQualifier returns the qualified lambda arn for the supplied name and (optional version or alias) qualifier
func (l Lambda) LambdaArnForNameAndQualifier(ctx context.Context, name string, qualifier *string) (*string, error) {

	// fetch the lambda func
	out, err := l.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(name),
		Qualifier:    qualifier,
	})

	if err != nil {
		// if lambda not found, then return nil obj
		var rnfe *types.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil, nil
		}
		// for other err's return err
		return nil, err
	}
	return out.Configuration.FunctionArn, nil
}

// LayerArnForNameAndQualifier returns the lambda layer arn && layer vesion arn for the supplied name and (optional version)
func (l Lambda) LayerArnForNameAndVersion(ctx context.Context, name string, version *int64) (*string, *string, error) {

	// fetch the lambda func
	out, err := l.GetLayerVersion(ctx, &lambda.GetLayerVersionInput{
		LayerName:     aws.String(name),
		VersionNumber: version,
	})

	if err != nil {
		// if lambda not found, then return nil obj
		var rnfe *types.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil, nil, nil
		}
		// for other err's return err
		return nil, nil, err
	}
	return out.LayerArn, out.LayerVersionArn, nil
}
