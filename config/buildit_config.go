package config

import (
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/resource/firehose"
	"github.com/TouchBistro/buildit/resource/lambda"
)

type resourcesConfig struct {
	// comparable resources
	BedrockApplicationInferenceProfile map[string]resource.BedrockApplicationInferenceProfile `yaml:"bedrock-application-inference-profile"`
	Certificate                        map[string]resource.ACMCertificate                     `yaml:"certificate"`
	CloudfrontDistribution             map[string]resource.CloudfrontDistribution             `yaml:"cloudfront-distribution"`
	CloudfrontFunction                 map[string]resource.CloudfrontFunction                 `yaml:"cloudfront-function"`
	CloudfrontVpcOrigin                map[string]resource.CloudfrontVpcOrigin                `yaml:"cloudfront-vpc-origin"`
	CWLogGroup                         map[string]resource.CWLogGroup                         `yaml:"cloudwatch-loggroup"`
	CWMetricAlarm                      map[string]resource.CWMetricAlarm                      `yaml:"cloudwatch-metricalarm"`
	CWSubscriptionFilter               map[string]resource.CWSubscriptionFilter               `yaml:"cloudwatch-subscriptionfilter"`
	DynamoDB                           map[string]resource.DynamoDB                           `yaml:"dynamodb-table"`
	ECSService                         map[string]resource.ECSService                         `yaml:"ecs-service"`
	EFSFileSystem                      map[string]resource.EFSFileSystem                      `yaml:"efs-filesystem"`
	EventBridgeApiDestination          map[string]resource.EventBridgeApiDestination          `yaml:"eventbridge-apidestination"`
	EventBridgeRule                    map[string]resource.EventBridgeRule                    `yaml:"eventbridge-rule"`
	EventBridgeConnection              map[string]resource.EventBridgeApiConnection           `yaml:"eventbridge-connection"`
	FirehoseDeliveryStream             map[string]firehose.FirehoseDeliveryStream             `yaml:"firehose-delivery-stream"`
	IAMPolicy                          map[string]resource.IAMPolicy                          `yaml:"iam-policy"`
	IAMRole                            map[string]resource.IAMRole                            `yaml:"iam-role"`
	LambdaFn                           map[string]lambda.Function                             `yaml:"lambda-function"`
	LambdaLayer                        map[string]lambda.Layer                                `yaml:"lambda-layer"`
	LBTargetGroup                      map[string]resource.LBTargetGroup                      `yaml:"lb-targetgroup"`
	LoadBalancer                       map[string]resource.LoadBalancer                       `yaml:"load-balancer"`
	MSKConnector                       map[string]resource.MSKConnector                       `yaml:"msk-connector"`
	MSKPlugin                          map[string]resource.MSKPlugin                          `yaml:"msk-plugin"`
	MSKWorkerConfiguration             map[string]resource.MSKWorkerConfiguration             `yaml:"msk-worker-configuration"`
	Route53Record                      map[string]resource.Route53Record                      `yaml:"route53-record"`
	S3Bucket                           map[string]resource.S3Bucket                           `yaml:"s3-bucket"`
	SDService                          map[string]resource.SDService                          `yaml:"sd-service"`
	SecurityGroup                      map[string]resource.SecurityGroup                      `yaml:"security-group"`
	SQSQueue                           map[string]resource.SQSQueue                           `yaml:"sqs-queue"`
	SNSSubscription                    map[string]resource.SNSSubscription                    `yaml:"sns-subscription"`
	StandaloneTask                     map[string]resource.StandaloneTask                     `yaml:"standalone-task"`
	StateMachine                       map[string]resource.StateMachine                       `yaml:"state-machine"`
	TaskDef                            map[string]resource.TaskDef                            `yaml:"taskdef"`
}

type builditConfig struct {
	Providers  map[string]*client.AwsProvider `yaml:"providers"`
	Resources  resourcesConfig                `yaml:"resources"`
	GlobalTags map[string]string              `yaml:"globalTags"`
	SHA        string                         `yaml:"-"`
	// Source is the file this config was read from, so validation can tell the user
	// which of several merged files to go and fix.
	Source string `yaml:"-"`
}
