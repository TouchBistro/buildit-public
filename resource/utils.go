package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/pkg/errors"
)

// formatFailures takes a list of ECS failures and formats them into a single string
// that can be used as part of an error message.
func formatFailures(failures []ecstypes.Failure) string {
	var sb strings.Builder
	for i, f := range failures {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if f.Arn != nil {
			sb.WriteString(*f.Arn)
			sb.WriteString(": ")
		}
		if f.Reason != nil {
			sb.WriteString(*f.Reason)
			sb.WriteString(": ")
		}
		if f.Detail != nil {
			sb.WriteString(*f.Detail)
		} else {
			sb.WriteString("unknown failure")
		}
	}
	return sb.String()
}

// ParseName parses n and returns the provider name and the resource name.
// If there is no provider name an empty string will be returned.
//
// Resource names may optionally be prefixed with a provider with the form
// `provider/resource`.
func ParseName(n string) (string, string) {
	i := strings.IndexRune(n, '/')
	if i == -1 {
		// No provider
		return client.MainProvider, n
	}
	return n[:i], n[i+1:]
}

// SplitNameQualifier converts the name of qualified resources, e.g. lambda functions
// or state macines name:qualifier where qualifier is a version or alias name, into
// 2 values, names & optional qualifiers if present
func SplitNameQualifier(fullyQualifiedName string) (string, *string) {
	var name string
	var qualifier *string
	sections := strings.Split(fullyQualifiedName, ":")
	name = sections[0]
	if len(sections) > 1 {
		qualifier = util.ToStringPtr(sections[1])
	}
	return name, qualifier
}

// TODO eventually move to awsw/securets_manager
// secretArnFromName returns the full arn for the secret for the specified secret id (friendly name)
// the id parameter can be provided in the following formats
// secredId : only the secret id is specified and it's searched in the `main` account
// providerName/secretId : the secret id is searched in the AWS account referenced by the `providerName`
// [providerName/]secretId:secretProperty : the trailing string after the first colon ":" is used as the
// json-key[:version-stage[:version-id]] and returned as-is
func secretArnFromName(ctx context.Context, rctx Context, id string) (string, error) {
	smClient := client.SecretsManager(ctx, rctx.ProviderName)
	index := strings.Index(id, "/")

	secretID := id
	if index != -1 {
		providerName := id[:index]
		secretID = id[index+1:]
		smClient = client.SecretsManager(ctx, providerName)
	}

	//check if json-key, version-stage, version-id exists
	jsonKey := ""
	colonIdx := strings.Index(secretID, ":")
	if colonIdx != -1 {
		jsonKey = secretID[colonIdx:]
		secretID = secretID[:colonIdx]
	}

	out, err := smClient.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		return "", errors.Wrapf(err, "cannot describe secret id %v", id)
	}

	return fmt.Sprintf("%v%v", *out.ARN, jsonKey), nil
}
