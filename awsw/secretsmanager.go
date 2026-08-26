package awsw

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/pkg/errors"
)

type SecretsManager struct {
	*secretsmanager.Client
}

func NewSecretsManager(ctx context.Context, providerName string) SecretsManager {
	return SecretsManager{client.SecretsManager(ctx, providerName)}
}

// SecretArnForIdentifier resolves a SecretsManager Secret ARN from an identifier (ARN or Name).
func (s SecretsManager) SecretArnForIdentifier(ctx context.Context, identifier string) (*string, error) {
	resource, provider := ParseIdentifier(identifier)
	if strings.HasPrefix(resource, "arn:") {
		return &resource, nil
	}

	smService := s
	if provider != "" {
		if _, err := providers.Get(provider); err != nil {
			return nil, fmt.Errorf("provider %q not found: %w", provider, err)
		}
		smService = NewSecretsManager(ctx, provider)
	}

	// Direct lookup by Name/ID
	out, err := smService.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(resource),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe secret %q: %w", resource, err)
	}

	return out.ARN, nil
}

// GetValue returns the value of the secret specified by arn/name
func (s SecretsManager) GetValue(ctx context.Context, name string) (*string, error) {

	out, err := s.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(name),
	})

	if err != nil {
		return nil, err
	}

	if out.SecretString == nil {
		return nil, errors.New("secret value is nil")
	}

	return out.SecretString, nil
}

// GetValuebySecretId returns the value for a secretId, where the secredId can be defined as
//
// secretId >        only the secretId as the friendly name of the secret  & the secret value would be returned
// secretId:key >    when supplied in this format, the first segement before the ':' is considered the secret friendly name,
//
//	while the trailing part is used to fetch the value of the field in the json value by key name
func (s SecretsManager) GetValueBySecretId(ctx context.Context, secretId string) (*string, error) {

	//check if json-key, version-stage, version-id exists
	jsonKey := ""
	colonIdx := strings.Index(secretId, ":")
	if colonIdx != -1 { //colon found...
		jsonKey = secretId[colonIdx+1:]
		secretId = secretId[:colonIdx]
	}

	if jsonKey == "" {
		return s.GetValue(ctx, secretId)
	} else {
		valMap := make(map[string]string)
		err := s.GetValueAsJson(ctx, secretId, &valMap)
		if err != nil {
			return nil, err
		}
		if val, ok := valMap[jsonKey]; !ok {
			return nil, errors.Errorf("no value for key %v found in secret %v", jsonKey, secretId)
		} else {
			return &val, nil
		}
	}
}

// GetValueAsJson returns the value of the secret specified by name unmarshalled into the
// supplied object, or returns an error if there was a problem finding, retrieving or unmarshalling
// the secret.
func (s SecretsManager) GetValueAsJson(ctx context.Context, name string, obj any) error {

	out, err := s.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(name),
	})

	if err != nil {
		return err
	}

	if out.SecretString == nil {
		return errors.New("secret value is nil")
	}

	jsonVal := *out.SecretString

	err = json.Unmarshal([]byte(jsonVal), obj)
	if err != nil {
		return err
	}

	return nil
}
