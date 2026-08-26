package client

import (
	"context"
	"time"

	// _ "github.com/TouchBistro/awesome"
	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/util"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/TouchBistro/awesome/clients/_acm"
	"github.com/TouchBistro/awesome/clients/_applicationautoscaling"
	"github.com/TouchBistro/awesome/clients/_autoscaling"
	"github.com/TouchBistro/awesome/clients/_bedrock"
	"github.com/TouchBistro/awesome/clients/_cloudfront"
	"github.com/TouchBistro/awesome/clients/_cloudwatch"
	"github.com/TouchBistro/awesome/clients/_cloudwatchlogs"
	"github.com/TouchBistro/awesome/clients/_dynamodb"
	"github.com/TouchBistro/awesome/clients/_ec2"
	"github.com/TouchBistro/awesome/clients/_ecs"
	"github.com/TouchBistro/awesome/clients/_efs"
	"github.com/TouchBistro/awesome/clients/_elasticloadbalancingv2"
	"github.com/TouchBistro/awesome/clients/_eventbridge"
	"github.com/TouchBistro/awesome/clients/_firehose"
	"github.com/TouchBistro/awesome/clients/_glue"
	"github.com/TouchBistro/awesome/clients/_iam"
	"github.com/TouchBistro/awesome/clients/_kafka"
	"github.com/TouchBistro/awesome/clients/_kafkaconnect"
	"github.com/TouchBistro/awesome/clients/_kms"
	"github.com/TouchBistro/awesome/clients/_lambda"
	"github.com/TouchBistro/awesome/clients/_rds"
	"github.com/TouchBistro/awesome/clients/_resourcegroupstaggingapi"
	"github.com/TouchBistro/awesome/clients/_route53"
	"github.com/TouchBistro/awesome/clients/_s3"
	"github.com/TouchBistro/awesome/clients/_secretsmanager"
	"github.com/TouchBistro/awesome/clients/_servicediscovery"
	"github.com/TouchBistro/awesome/clients/_sfn"
	"github.com/TouchBistro/awesome/clients/_sns"
	"github.com/TouchBistro/awesome/clients/_sqs"
	"github.com/TouchBistro/awesome/clients/_sts"
	"github.com/TouchBistro/awesome/clients/_wafv2"

	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/firehose"
	"github.com/aws/aws-sdk-go-v2/service/glue"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kafkaconnect"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/wafv2"
)

const (
	// DefaultProvider is the aws clients provider for the IAM user's account
	DefaultProvider = "default"

	// MainProvider is the aws clients provider for the main account, or defaut if no main
	MainProvider = "main"
)

const (
	ProviderTypeRole    string = "role"
	ProviderTypeEnv     string = "env"
	ProviderTypeShared  string = "shared"
	ProviderTypeDefault string = "default"
)

// AwsProvider encapsulates the AWS clients.
type AwsProvider struct {
	Name                   string `yaml:"-"`
	ProviderType           string `yaml:"type"` // env | shared | default | role
	AccountId              string `yaml:"accountId"`
	RoleName               string `yaml:"roleName"`
	RoleArn                string `yaml:"roleArn"`
	ConfigFile             string `yaml:"configFile"`
	CredsFile              string `yaml:"credsFile"`
	Profile                string `yaml:"profile"`
	AccessKeyIdEnvVarName  string `yaml:"accessKeyIdEnvVar"`
	SecretAccessKeyVarName string `yaml:"secretAccessKeyEnvVar"`
	SessionTokenVarName    string `yaml:"sessionTokenEnvVar"`
	Region                 string `yaml:"region"`
}

// ToCredsProvider converts the AWSProvider configuration to an `awesome` CredsProvider type
// or returns a non-nil error
//
// the `providerType` defines what type of CredsProvider is configured.
//   - `default`  DefaultCredsProvider
//   - `env`      EnvironmentCredsProvider
//   - `shared`   SharedCredsProvider
//   - `role`     AssumeRoleCredsProvider
//
// If the `providerType` is `role` the `baseProvider` name is used to fetch the base provider
// (credentials) to assume that role.
//
// If the `providerType` is `role` & a non-nil `base` provider name is supplied, then an assumeRole
// operation is attempted using an existing provider with that name. If the baseProvider does
// not exist, an error is returned.
//
// If the `providerType` is `role` & a nil `base` provider name is supplied, then a DefaultCredsProvider is
// initialized & saved as `default`) first, before attemping to use it to assume-role.
func (r *AwsProvider) ToCredsProvider(ctx context.Context, baseProviderName *string) (*providers.CredsProvider, error) {
	var err error
	var def providers.CredsProvider
	var options []providers.CredsProviderOptionsFunc

	// always validate
	options = append(options, providers.ValidateProvider())

	region := "us-east-1" // default
	if len(r.Region) != 0 {
		region = r.Region
		options = append(options, providers.WithRegion(r.Region))
	}

	switch r.ProviderType {

	case ProviderTypeEnv:

		log.WithFields(log.Fields{"provider": r.Name, "region": region}).Info("initializing provider from environment variables")

		if len(r.AccessKeyIdEnvVarName) != 0 {
			options = append(options, providers.WithAccessKeyIdFrom(r.AccessKeyIdEnvVarName))
		}
		if len(r.SecretAccessKeyVarName) != 0 {
			options = append(options, providers.WithSecretAccessKeyFrom(r.SecretAccessKeyVarName))
		}
		if len(r.SessionTokenVarName) != 0 {
			options = append(options, providers.WithSessionTokenFrom(r.SessionTokenVarName))
		}
		def, err = providers.NewEnvironmentCredsProvider(ctx, r.Name, options...)

	case ProviderTypeShared:
		log.WithFields(log.Fields{"provider": r.Name, "region": region}).Info("initializing provider from shared credentials")

		if len(r.ConfigFile) != 0 {
			options = append(options, providers.WithConfigFile(r.ConfigFile))
		}
		if len(r.CredsFile) != 0 {
			options = append(options, providers.WithCredentialsFile(r.CredsFile))
		}
		if len(r.Profile) != 0 {
			options = append(options, providers.WithConfigProfile(r.Profile))
		}

		def, err = providers.NewSharedConfigCredsProvider(ctx, r.Name, options...)

	case ProviderTypeRole:

		// if no baseProviderName supplied, attempt to create a base provider named "default".
		// this will override any provider named `default`
		// also set the baseProviderName arg value = `default`
		if baseProviderName == nil {
			providerName := DefaultProvider
			baseProviderName = &providerName
			log.Debugf("no base provider supplied, setting up base provider `default` using default credentials chain first")
			def, err = providers.NewDefaultCredsProvider(ctx, providerName, options...)
			if err != nil {
				return nil, err
			}
		}

		// get the base provider to assume role; at this point, it should exist
		baseProvider, err := providers.Get(*baseProviderName)
		if err != nil {
			return nil, err
		}

		if len(r.RoleArn) != 0 {
			log.WithFields(log.Fields{"provider": r.Name, "base": *baseProviderName, "arn": r.RoleArn, "region": region}).Info("initializing provider using assume role")
			def, err = providers.NewAssumeRoleCredsProvider(ctx, r.Name,
				providers.WithBaseCredsProvider(baseProvider),
				providers.WithRoleArn(r.RoleArn),
				providers.WithRegion(region),
				providers.ValidateProvider())
			if err != nil {
				return nil, errors.Wrapf(err, "error assuming role %v for %v provider", r.RoleArn, r.Name)
			}
		} else if len(r.AccountId) != 0 && len(r.RoleName) != 0 {
			log.WithFields(log.Fields{"provider": r.Name, "base": *baseProviderName, "accountId": r.AccountId, "role": r.RoleName, "region": region}).Info("initializing provider using assume role")
			def, err = providers.NewAssumeRoleCredsProvider(ctx, r.Name,
				providers.WithBaseCredsProvider(baseProvider),
				providers.WithAccountId(r.AccountId),
				providers.WithRoleName(r.RoleName),
				providers.WithRegion(region),
				providers.ValidateProvider())
			if err != nil {
				return nil, errors.Wrapf(err, "error assuming role %v in account %v for %v provider", r.RoleName, r.RoleArn, r.Name)
			}
		} else {
			return nil, errors.Errorf("not enough parameters supplied for configuring role provider, RoleArn or AccountId & RoleName required")
		}

	default:
		log.WithField("name", "default").Info("initializing  provider using default credentials chain")
		def, err = providers.NewDefaultCredsProvider(ctx, r.Name, options...)
	}

	return &def, err
}

// Merge the other provider into this, for now, only accountID & RoleName values
// are merged, if they are set to non-zero (non-default) values
func (r *AwsProvider) Merge(other AwsProvider) {
	if len(other.ProviderType) != 0 {
		r.ProviderType = other.ProviderType
	}

	if len(other.AccountId) != 0 {
		r.AccountId = other.AccountId
	}

	if len(other.RoleName) != 0 {
		r.RoleName = other.RoleName
	}

	if len(other.RoleArn) != 0 {
		r.RoleName = other.RoleArn
	}

	if len(other.ConfigFile) != 0 {
		r.RoleName = other.ConfigFile
	}

	if len(other.CredsFile) != 0 {
		r.RoleName = other.CredsFile
	}

	if len(other.Profile) != 0 {
		r.RoleName = other.Profile
	}

	if len(other.AccessKeyIdEnvVarName) != 0 {
		r.RoleName = other.AccessKeyIdEnvVarName
	}

	if len(other.SecretAccessKeyVarName) != 0 {
		r.RoleName = other.SecretAccessKeyVarName
	}
}

// InitAwsProviders uses the supplied buildit RoleClientProviders map & initializes all named
// providers using the aws-ccp-go (github.com/TouchBistro/awesome)
//
// buildit uses 2 reserved named providers:
//
// default: this provider uses the credentials from the context that buildit is running in. this is
//
//	usually the AWS default credentials chain, unless the optional command line parameters
//	account-id & role-name are supplied. In that case, it assumes the role using the default
//	creds.
//
// main:    this provider is the same as `default`, unless it's explicitly defined in the
//
//	buildit config yaml. This provider is used to provision resources defined in that config
//	or search for any resources that are referenced by resource definitions.
//
// all other providers defined in the yaml config are used for searching references during provision.
// these references are defined using the <provider_name>/<resource_name> syntax.
//
// First parameter is the map of configs for provider configuration, <provider name> -> cfg
// Optional: when `defaultAccountId` & `defaultRoleName` are supplied (via command-line arguments)
// then the `default` provider is updated to use an AssumeRoleProvider for the corresponding role.
func InitAwsProviders(ctx context.Context, defaultProvider AwsProvider, otherProviders map[string]*AwsProvider) error {
	// build default provider
	defaultProvider.Name = DefaultProvider              // ensure name is set & correct
	_, err := defaultProvider.ToCredsProvider(ctx, nil) // since base provider is nil, a new provider type=`default` & named=`default` will be created first
	if err != nil {
		return errors.Wrapf(err, "error initializing provider: default")
	}

	// buildit main provider is the same as default, unless defined in the RoleClientProvider map
	if _, ok := otherProviders[MainProvider]; !ok {
		_, err := providers.Clone(DefaultProvider, MainProvider)
		if err != nil {
			return errors.Wrapf(err, "error intializing provider: %v", MainProvider)
		}
	}

	// loop through the config map & initialize all providers
	for name, config := range otherProviders {
		config.Name = name // set provider name
		_, err = config.ToCredsProvider(ctx, util.ToStringPtr(DefaultProvider))
		if err != nil {
			return errors.Wrapf(err, "error initializing provider: %v", name)
		}
	}

	return nil
}

func customRetry(retries int, backoff int) *retry.Standard {
	return retry.NewStandard(func(o *retry.StandardOptions) {
		o.MaxAttempts = retries
		o.MaxBackoff = time.Duration(backoff) * time.Second
	})
}

// mustGetProvider retrieves a provider by name or exits with a useful error message if not found
func mustGetProvider(name string) providers.CredsProvider {
	p, err := providers.Get(name)
	if err != nil {
		log.Fatalf("AWS provider %q not found. Please ensure it is defined in the providers section of your buildit configuration.", name)
	}
	return p
}

// Wrapper functions to returning a service client for default or a named provider
// By design all of these functions panic in case of downstream errors

// ACM returns the AWS ACM API client for the main provider.
func ACM(ctx context.Context, providerName string) *acm.Client {
	return _acm.Must(mustGetProvider(providerName), func(o *acm.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// ACMGlobal returns an AWS ACM API client for the named provider, pinned to us-east-1:
// certificates served by CloudFront must live in us-east-1 regardless of the provider's
// configured region. Built per call instead of via _acm.Must because the shared client
// cache is keyed by provider only — a cached region pin would leak into (or be lost to)
// the regional ACM accessor above.
func ACMGlobal(ctx context.Context, providerName string) *acm.Client {
	return acm.NewFromConfig(mustGetProvider(providerName).Config(), func(o *acm.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
		o.Region = "us-east-1"
	})
}

// ApplicationAutoScaling returns an AWS ApplicationAutoScaling API client for the main provider.
func ApplicationAutoScaling(ctx context.Context, providerName string) *applicationautoscaling.Client {
	return _applicationautoscaling.Must(mustGetProvider(providerName), func(o *applicationautoscaling.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// Autoscaling returns an AWS Autoscaling API Client for the main provider.
func Autoscaling(ctx context.Context, providerName string) *autoscaling.Client {
	return _autoscaling.Must(mustGetProvider(providerName), func(o *autoscaling.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// Bedrock returns the AWS Bedrock API client for the main provider.
func Bedrock(ctx context.Context, providerName string) *bedrock.Client {
	return _bedrock.Must(mustGetProvider(providerName), func(o *bedrock.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// Cloudfront returns the AWS Cloudfront API client for the main provider.
func Cloudfront(ctx context.Context, providerName string) *cloudfront.Client {
	return _cloudfront.Must(mustGetProvider(providerName), func(o *cloudfront.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// CloudWatchLogs returns the AWS CloudWatchLogs API client for the main provider.
func CloudWatchLogs(ctx context.Context, providerName string) *cloudwatchlogs.Client {
	return _cloudwatchlogs.Must(mustGetProvider(providerName), func(o *cloudwatchlogs.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// CloudWatch returns the AWS CloudWatchLogs API client for the main provider.
func CloudWatch(ctx context.Context, providerName string) *cloudwatch.Client {
	return _cloudwatch.Must(mustGetProvider(providerName), func(o *cloudwatch.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// EventBridge returns the AWS EventBridge API client for the main provider.
func EventBridge(ctx context.Context, providerName string) *eventbridge.Client {
	return _eventbridge.Must(mustGetProvider(providerName), func(o *eventbridge.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// Firehose returns the AWS Kinesis Data Firehose API client for the main provider.
func Firehose(ctx context.Context, providerName string) *firehose.Client {
	return _firehose.Must(mustGetProvider(providerName), func(o *firehose.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// Glue returns the AWS Glue API client for the main provider.
func Glue(ctx context.Context, providerName string) *glue.Client {
	return _glue.Must(mustGetProvider(providerName), func(o *glue.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// EC2 returns an AWS EC2 API client for the main provider.
func EC2(ctx context.Context, providerName string) *ec2.Client {
	return _ec2.Must(mustGetProvider(providerName), func(o *ec2.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// ECS returns an AWS ECS API client for the main provider.
func ECS(ctx context.Context, providerName string) *ecs.Client {
	return _ecs.Must(mustGetProvider(providerName), func(o *ecs.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// ELB returns an AWS ELBv2 API client for the main provider.
func ELB(ctx context.Context, providerName string) *elbv2.Client {
	return _elasticloadbalancingv2.Must(mustGetProvider(providerName), func(o *elbv2.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// IAM returns an AWS IAM API client for the main provider.
func IAM(ctx context.Context, providerName string) *iam.Client {
	return _iam.Must(mustGetProvider(providerName), func(o *iam.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

func Lambda(ctx context.Context, providerName string) *lambda.Client {
	return _lambda.Must(mustGetProvider(providerName), func(o *lambda.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// RDS returns an AWS RDS API client for the main provider.
func RDS(ctx context.Context, providerName string) *rds.Client {
	return _rds.Must(mustGetProvider(providerName), func(o *rds.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// Route53 returns an AWS Route53 API client
func Route53(ctx context.Context, providerName string) *route53.Client {
	return _route53.Must(mustGetProvider(providerName), func(o *route53.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// S3 returns an AWS S3 API client
func S3(ctx context.Context, providerName string) *s3.Client {
	return _s3.Must(mustGetProvider(providerName), func(o *s3.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// ServiceDiscovery returns an AWS ServiceDiscovery API client for the main provider.
func ServiceDiscovery(ctx context.Context, providerName string) *servicediscovery.Client {
	return _servicediscovery.Must(mustGetProvider(providerName), func(o *servicediscovery.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// STS returns an AWS STS API client for the main provider.
func STS(ctx context.Context, providerName string) *sts.Client {
	return _sts.Must(mustGetProvider(providerName), func(o *sts.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// SecretsManager returns an AWS SecretsManager API client for the main provider.
func SecretsManager(ctx context.Context, providerName string) *secretsmanager.Client {
	return _secretsmanager.Must(mustGetProvider(providerName), func(o *secretsmanager.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// SFN returns an AWS SFN (state function) API client for the main provider.
func SFN(ctx context.Context, providerName string) *sfn.Client {
	return _sfn.Must(mustGetProvider(providerName), func(o *sfn.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// SQS returns the AWS SQS API client for the main provider.
func SQS(ctx context.Context, providerName string) *sqs.Client {
	return _sqs.Must(mustGetProvider(providerName), func(o *sqs.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// SNS returns the AWS SNS API client for the main provider.
func SNS(ctx context.Context, providerName string) *sns.Client {
	return _sns.Must(mustGetProvider(providerName), func(o *sns.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// SNS returns the AWS SNS API client for the main provider.
func EFS(ctx context.Context, providerName string) *efs.Client {
	return _efs.Must(mustGetProvider(providerName), func(o *efs.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// KMS returns the AWS KMS API client for the supplied provider.
func KMS(ctx context.Context, providerName string) *kms.Client {
	return _kms.Must(mustGetProvider(providerName), func(o *kms.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// MSK returns the AWS MSK API client for the main provider.
func MSK(ctx context.Context, providerName string) *kafkaconnect.Client {
	return _kafkaconnect.Must(mustGetProvider(providerName), func(o *kafkaconnect.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// Kafka returns the AWS MSK API client for the main provider.
func Kafka(ctx context.Context, providerName string) *kafka.Client {
	return _kafka.Must(mustGetProvider(providerName), func(o *kafka.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

func ResourceGroupsTaggingAPI(ctx context.Context, providerName string) *resourcegroupstaggingapi.Client {
	return _resourcegroupstaggingapi.Must(mustGetProvider(providerName), func(o *resourcegroupstaggingapi.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// DynamoDB returns the AWS DynamoDB API client for the main provider.
func DynamoDB(ctx context.Context, providerName string) *dynamodb.Client {
	return _dynamodb.Must(mustGetProvider(providerName), func(o *dynamodb.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
	})
}

// WAFV2Global returns the AWS WAFv2 API client for the named provider, pinned to
// us-east-1: buildit only uses WAFv2 for CloudFront-scoped (global) web ACLs, and AWS
// serves the CLOUDFRONT scope exclusively from us-east-1 regardless of the provider's
// configured region. A regional WAFv2 use case must add a separate accessor (the client
// is cached per provider, so the region pin here would leak into it).
func WAFV2Global(ctx context.Context, providerName string) *wafv2.Client {
	return _wafv2.Must(mustGetProvider(providerName), func(o *wafv2.Options) {
		o.Retryer = customRetry(ctx.Value(util.RETRY).(int), ctx.Value(util.BACKOFF).(int))
		o.Region = "us-east-1"
	})
}
