package client

import (
	"context"
	"fmt"
	"sync"
	"time"

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

// buildit uses two reserved provider names:
//
// default: the credentials of the context buildit runs in. The CLI builds it from the
// --default-provider-type family of flags before the config is parsed (InitDefaultProvider).
// A program embedding buildit registers its own with the awesome providers package, or
// lets EnsureDefaultProvider build one from the AWS default credentials chain.
//
// main: provisions the resources of a config and resolves the references they make. buildit
// never writes it to the registry: Provider resolves `main` to a registered provider of that
// name (declared in a config file, or registered by the program), else to `default`.
//
// Every other provider is used to look up references written as <provider>::<name>.

// Provider resolves name against the awesome providers registry, the only source of
// providers buildit uses: `main` is the registered `main` if there is one, else `default`;
// every other name must be registered as is.
func Provider(name string) (providers.CredsProvider, error) {
	if name == MainProvider {
		if p, err := providers.Get(MainProvider); err == nil {
			return p, nil
		}
		p, err := providers.Get(DefaultProvider)
		if err != nil {
			return nil, errors.Wrapf(err, "AWS provider %q not found: no %q is registered and there is no %q to fall back to; register one with the awesome providers package before building the config", MainProvider, MainProvider, DefaultProvider)
		}
		return p, nil
	}
	p, err := providers.Get(name)
	if err != nil {
		return nil, errors.Wrapf(err, "AWS provider %q not found: declare it in the providers section of the buildit config, or register it with the awesome providers package before building the config", name)
	}
	return p, nil
}

// InitDefaultProvider builds the reserved `default` provider from a CLI-style AwsProvider
// config (the --default-provider-type family of flags), replacing any registered `default`.
// The CLI calls it before the config is parsed.
func InitDefaultProvider(ctx context.Context, cfg AwsProvider) error {
	cfg.Name = DefaultProvider                               // ensure name is set & correct
	if _, err := cfg.ToCredsProvider(ctx, nil); err != nil { // nil base: a provider named `default` is created first
		return errors.Wrapf(err, "error initializing provider: %v", DefaultProvider)
	}
	return nil
}

// EnsureDefaultProvider leaves a registered `default` provider as it is and otherwise
// builds one the way the CLI does with no flags: the AWS default credentials chain in
// us-east-1, validated with STS. It is what a program embedding buildit gets when it
// registers no `default` of its own.
func EnsureDefaultProvider(ctx context.Context) error {
	if _, err := providers.Get(DefaultProvider); err == nil {
		return nil
	}
	log.Debug("no `default` provider registered, building one from the AWS default credentials chain")
	return InitDefaultProvider(ctx, AwsProvider{ProviderType: ProviderTypeDefault})
}

// CheckProviderRegistered verifies that p is the provider registered under its own name
// in the awesome registry, which is how every service client accessor in this package
// finds it. A provider built with providers.New*CredsProvider is registered on
// construction, and a later one built under the same name replaces it.
func CheckProviderRegistered(p providers.CredsProvider) error {
	if p == nil {
		return errors.New("nil provider supplied")
	}
	registered, err := providers.Get(p.Key())
	if err != nil {
		return errors.Wrapf(err, "provider %q is not registered; build it with the awesome providers package (providers.New*CredsProvider)", p.Key())
	}
	if registered != p {
		return errors.Errorf("provider %q is not the one registered under that name; a provider built later with the same name replaced it", p.Key())
	}
	return nil
}

// configProviders are the names InitNamedProviders has registered in this process, so a
// later config declaring the same provider again is not reported as replacing someone
// else's. It grows for the life of the process on purpose: the awesome registry has no
// unregister, so a name it holds stays registered too.
var configProviders = struct {
	sync.Mutex
	names map[string]struct{}
}{names: make(map[string]struct{})}

// InitNamedProviders initializes every provider declared in the `providers:` section of a
// config, <provider name> -> cfg, assuming roles from the `default` provider where needed.
// A declared provider replaces a registered one of the same name; that is logged as a
// warning unless the registered one came from a config too.
func InitNamedProviders(ctx context.Context, cfgs map[string]*AwsProvider) error {
	configProviders.Lock()
	defer configProviders.Unlock()
	for name, cfg := range cfgs {
		if _, registered := configProviders.names[name]; !registered {
			if _, err := providers.Get(name); err == nil {
				log.WithField("provider", name).Warn("provider declared in the config replaces a registered provider of the same name")
			}
		}
		cfg.Name = name // set provider name
		if _, err := cfg.ToCredsProvider(ctx, util.ToStringPtr(DefaultProvider)); err != nil {
			return errors.Wrapf(err, "error initializing provider: %v", name)
		}
		configProviders.names[name] = struct{}{}
	}
	return nil
}

const (
	// DefaultRetries and DefaultBackoff apply when the context carries no util.RETRY /
	// util.BACKOFF value, so a library consumer that never set them still gets the CLI's
	// defaults instead of a panic.
	DefaultRetries = 3
	DefaultBackoff = 20
)

// retryerFor builds the retryer for a service client from the retry values on ctx,
// falling back to the defaults above.
func retryerFor(ctx context.Context) *retry.Standard {
	return customRetry(
		ctxInt(ctx, util.RETRY, DefaultRetries),
		ctxInt(ctx, util.BACKOFF, DefaultBackoff),
	)
}

// ctxInt returns the int stored under key, or fallback when the key is absent. A value
// that is present but not an int is a caller bug (ContextWithRetryPolicy is the typed
// way to set these), so it is reported rather than silently replaced.
func ctxInt(ctx context.Context, key any, fallback int) int {
	v := ctx.Value(key)
	if v == nil {
		return fallback
	}
	n, ok := v.(int)
	if !ok {
		log.WithFields(log.Fields{"key": key, "type": fmt.Sprintf("%T", v)}).
			Warnf("Ignoring context retry value: expected int, using default %d", fallback)
		return fallback
	}
	return n
}

func customRetry(retries int, backoff int) *retry.Standard {
	return retry.NewStandard(func(o *retry.StandardOptions) {
		o.MaxAttempts = retries
		o.MaxBackoff = time.Duration(backoff) * time.Second
	})
}

// Service client accessors. Each returns the cached client for the named provider, or an
// error when the provider is unknown. The retryer comes from the retry values on ctx.

// ACM returns the AWS ACM API client for the main provider.
func ACM(ctx context.Context, providerName string) (*acm.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _acm.Client(p, func(o *acm.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// ACMGlobal returns an AWS ACM API client for the named provider, pinned to us-east-1:
// certificates served by CloudFront must live in us-east-1 regardless of the provider's
// configured region. Built per call instead of via _acm.Must because the shared client
// cache is keyed by provider only — a cached region pin would leak into (or be lost to)
// the regional ACM accessor above.
func ACMGlobal(ctx context.Context, providerName string) (*acm.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return acm.NewFromConfig(p.Config(), func(o *acm.Options) {
		o.Retryer = retryerFor(ctx)
		o.Region = "us-east-1"
	}), nil
}

// ApplicationAutoScaling returns an AWS ApplicationAutoScaling API client for the main provider.
func ApplicationAutoScaling(ctx context.Context, providerName string) (*applicationautoscaling.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _applicationautoscaling.Client(p, func(o *applicationautoscaling.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// Autoscaling returns an AWS Autoscaling API Client for the main provider.
func Autoscaling(ctx context.Context, providerName string) (*autoscaling.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _autoscaling.Client(p, func(o *autoscaling.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// Bedrock returns the AWS Bedrock API client for the main provider.
func Bedrock(ctx context.Context, providerName string) (*bedrock.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _bedrock.Client(p, func(o *bedrock.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// Cloudfront returns the AWS Cloudfront API client for the main provider.
func Cloudfront(ctx context.Context, providerName string) (*cloudfront.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _cloudfront.Client(p, func(o *cloudfront.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// CloudWatchLogs returns the AWS CloudWatchLogs API client for the main provider.
func CloudWatchLogs(ctx context.Context, providerName string) (*cloudwatchlogs.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _cloudwatchlogs.Client(p, func(o *cloudwatchlogs.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// CloudWatch returns the AWS CloudWatchLogs API client for the main provider.
func CloudWatch(ctx context.Context, providerName string) (*cloudwatch.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _cloudwatch.Client(p, func(o *cloudwatch.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// EventBridge returns the AWS EventBridge API client for the main provider.
func EventBridge(ctx context.Context, providerName string) (*eventbridge.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _eventbridge.Client(p, func(o *eventbridge.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// Firehose returns the AWS Kinesis Data Firehose API client for the main provider.
func Firehose(ctx context.Context, providerName string) (*firehose.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _firehose.Client(p, func(o *firehose.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// Glue returns the AWS Glue API client for the main provider.
func Glue(ctx context.Context, providerName string) (*glue.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _glue.Client(p, func(o *glue.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// EC2 returns an AWS EC2 API client for the main provider.
func EC2(ctx context.Context, providerName string) (*ec2.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _ec2.Client(p, func(o *ec2.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// ECS returns an AWS ECS API client for the main provider.
func ECS(ctx context.Context, providerName string) (*ecs.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _ecs.Client(p, func(o *ecs.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// ELB returns an AWS ELBv2 API client for the main provider.
func ELB(ctx context.Context, providerName string) (*elbv2.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _elasticloadbalancingv2.Client(p, func(o *elbv2.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// IAM returns an AWS IAM API client for the main provider.
func IAM(ctx context.Context, providerName string) (*iam.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _iam.Client(p, func(o *iam.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

func Lambda(ctx context.Context, providerName string) (*lambda.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _lambda.Client(p, func(o *lambda.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// RDS returns an AWS RDS API client for the main provider.
func RDS(ctx context.Context, providerName string) (*rds.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _rds.Client(p, func(o *rds.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// Route53 returns an AWS Route53 API client
func Route53(ctx context.Context, providerName string) (*route53.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _route53.Client(p, func(o *route53.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// S3 returns an AWS S3 API client
func S3(ctx context.Context, providerName string) (*s3.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _s3.Client(p, func(o *s3.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// ServiceDiscovery returns an AWS ServiceDiscovery API client for the main provider.
func ServiceDiscovery(ctx context.Context, providerName string) (*servicediscovery.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _servicediscovery.Client(p, func(o *servicediscovery.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// STS returns an AWS STS API client for the main provider.
func STS(ctx context.Context, providerName string) (*sts.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _sts.Client(p, func(o *sts.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// SecretsManager returns an AWS SecretsManager API client for the main provider.
func SecretsManager(ctx context.Context, providerName string) (*secretsmanager.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _secretsmanager.Client(p, func(o *secretsmanager.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// SFN returns an AWS SFN (state function) API client for the main provider.
func SFN(ctx context.Context, providerName string) (*sfn.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _sfn.Client(p, func(o *sfn.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// SQS returns the AWS SQS API client for the main provider.
func SQS(ctx context.Context, providerName string) (*sqs.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _sqs.Client(p, func(o *sqs.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// SNS returns the AWS SNS API client for the main provider.
func SNS(ctx context.Context, providerName string) (*sns.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _sns.Client(p, func(o *sns.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// SNS returns the AWS SNS API client for the main provider.
func EFS(ctx context.Context, providerName string) (*efs.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _efs.Client(p, func(o *efs.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// KMS returns the AWS KMS API client for the supplied provider.
func KMS(ctx context.Context, providerName string) (*kms.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _kms.Client(p, func(o *kms.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// MSK returns the AWS MSK API client for the main provider.
func MSK(ctx context.Context, providerName string) (*kafkaconnect.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _kafkaconnect.Client(p, func(o *kafkaconnect.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// Kafka returns the AWS MSK API client for the main provider.
func Kafka(ctx context.Context, providerName string) (*kafka.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _kafka.Client(p, func(o *kafka.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

func ResourceGroupsTaggingAPI(ctx context.Context, providerName string) (*resourcegroupstaggingapi.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _resourcegroupstaggingapi.Client(p, func(o *resourcegroupstaggingapi.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// DynamoDB returns the AWS DynamoDB API client for the main provider.
func DynamoDB(ctx context.Context, providerName string) (*dynamodb.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _dynamodb.Client(p, func(o *dynamodb.Options) {
		o.Retryer = retryerFor(ctx)
	})
}

// WAFV2Global returns the AWS WAFv2 API client for the named provider, pinned to
// us-east-1: buildit only uses WAFv2 for CloudFront-scoped (global) web ACLs, and AWS
// serves the CLOUDFRONT scope exclusively from us-east-1 regardless of the provider's
// configured region. A regional WAFv2 use case must add a separate accessor (the client
// is cached per provider, so the region pin here would leak into it).
func WAFV2Global(ctx context.Context, providerName string) (*wafv2.Client, error) {
	p, err := Provider(providerName)
	if err != nil {
		return nil, err
	}
	return _wafv2.Client(p, func(o *wafv2.Options) {
		o.Retryer = retryerFor(ctx)
		o.Region = "us-east-1"
	})
}
