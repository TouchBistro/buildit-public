module github.com/TouchBistro/buildit

go 1.25.1

require (
	github.com/DataDog/datadog-go/v5 v5.2.0
	github.com/TouchBistro/awesome v0.0.9
	github.com/TouchBistro/goutils v0.4.0
	github.com/aws/aws-sdk-go-v2/service/bedrock v1.59.0
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.67.0
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.50.5
	github.com/aws/aws-sdk-go-v2/service/ecs v1.41.1
	github.com/aws/aws-sdk-go-v2/service/efs v1.34.2
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.30.1
	github.com/aws/aws-sdk-go-v2/service/kafka v1.39.2
	github.com/aws/aws-sdk-go-v2/service/kafkaconnect v1.23.2
	github.com/aws/aws-sdk-go-v2/service/kms v1.53.4
	github.com/aws/aws-sdk-go-v2/service/lambda v1.77.6
	github.com/aws/aws-sdk-go-v2/service/s3 v1.51.4
	github.com/aws/aws-sdk-go-v2/service/sfn v1.26.2
	github.com/aws/aws-sdk-go-v2/service/sns v1.31.3
	github.com/aws/aws-sdk-go-v2/service/wafv2 v1.60.1
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.9.0
	github.com/spf13/cobra v1.6.1
	github.com/spf13/viper v1.20.1
	github.com/stretchr/testify v1.10.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.11.9 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.2.1 // indirect
	github.com/pelletier/go-toml/v2 v2.2.3 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sagikazarmark/locafero v0.7.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.9.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// AWS SDK modules
require (
	github.com/aws/aws-sdk-go-v2 v1.43.0
	github.com/aws/aws-sdk-go-v2/config v1.27.16 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/acm v1.25.1
	github.com/aws/aws-sdk-go-v2/service/applicationautoscaling v1.27.1
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.40.1
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.36.1
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.34.1
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.162.0
	github.com/aws/aws-sdk-go-v2/service/eventbridge v1.30.1
	github.com/aws/aws-sdk-go-v2/service/firehose v1.37.4
	github.com/aws/aws-sdk-go-v2/service/glue v1.137.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.31.1
	github.com/aws/aws-sdk-go-v2/service/rds v1.74.0
	github.com/aws/aws-sdk-go-v2/service/route53 v1.40.1
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.28.1
	github.com/aws/aws-sdk-go-v2/service/servicediscovery v1.29.1
	github.com/aws/aws-sdk-go-v2/service/sqs v1.31.1
	github.com/aws/aws-sdk-go-v2/service/sts v1.28.10
)

require (
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.1 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.31 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.3.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.11.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.17.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi v1.26.6
	github.com/aws/aws-sdk-go-v2/service/sso v1.20.9 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.24.3 // indirect
	github.com/aws/smithy-go v1.27.3 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	golang.org/x/tools v0.21.1-0.20240508182429-e35e4ccd0d2d // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)
