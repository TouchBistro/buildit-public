package config

import (
	"strings"

	"github.com/TouchBistro/awesome/providers"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/resource/firehose"
	"github.com/TouchBistro/buildit/resource/lambda"
	"github.com/pkg/errors"
)

// The Add methods below are the Go-side way to populate a ResourceSet, one pair per
// resource type (TestResourceSetAddMethodsCoverEveryType keeps them in step with the
// struct). AddX stores the resource under the name it would have in buildit.yml, which may
// carry a provider prefix (`provider::name`); AddXWithProvider takes the provider as an
// awesome CredsProvider instead, which must be the one registered under its name, since
// that is where the resource's service clients will look it up. Both reject a name already
// present for that type: a silently replaced resource would drop out of plan/apply/destroy
// (DEVOPS-8902).

// addResource stores r under key in the map for one resource type, allocating the map on
// first use.
func addResource[T any](m *map[string]T, typeName, key string, r T) error {
	if *m == nil {
		*m = make(map[string]T)
	}
	if _, exists := (*m)[key]; exists {
		return errors.Errorf("%s %q is already defined", typeName, key)
	}
	(*m)[key] = r
	return nil
}

// providerKey returns the `provider::name` key a resource is stored under, after checking
// that p is the provider registered under its name.
func (rs *ResourceSet) providerKey(p providers.CredsProvider, name string) (string, error) {
	if err := client.CheckProviderRegistered(p); err != nil {
		return "", errors.Wrapf(err, "resource %q", name)
	}
	if strings.Contains(name, resource.DefaultKeyParseDelim) {
		return "", errors.Errorf("resource name %q must not contain %q when a provider is supplied", name, resource.DefaultKeyParseDelim)
	}
	return resource.NewKey(p.Key(), name).String(), nil
}

// AddBedrockApplicationInferenceProfile adds a `bedrock-application-inference-profile` resource under name.
func (rs *ResourceSet) AddBedrockApplicationInferenceProfile(name string, r resource.BedrockApplicationInferenceProfile) error {
	return addResource(&rs.BedrockApplicationInferenceProfile, "bedrock-application-inference-profile", name, r)
}

// AddBedrockApplicationInferenceProfileWithProvider adds a `bedrock-application-inference-profile` resource under name, provisioned by p.
func (rs *ResourceSet) AddBedrockApplicationInferenceProfileWithProvider(p providers.CredsProvider, name string, r resource.BedrockApplicationInferenceProfile) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.BedrockApplicationInferenceProfile, "bedrock-application-inference-profile", key, r)
}

// AddCertificate adds a `certificate` resource under name.
func (rs *ResourceSet) AddCertificate(name string, r resource.ACMCertificate) error {
	return addResource(&rs.Certificate, "certificate", name, r)
}

// AddCertificateWithProvider adds a `certificate` resource under name, provisioned by p.
func (rs *ResourceSet) AddCertificateWithProvider(p providers.CredsProvider, name string, r resource.ACMCertificate) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.Certificate, "certificate", key, r)
}

// AddCloudfrontDistribution adds a `cloudfront-distribution` resource under name.
func (rs *ResourceSet) AddCloudfrontDistribution(name string, r resource.CloudfrontDistribution) error {
	return addResource(&rs.CloudfrontDistribution, "cloudfront-distribution", name, r)
}

// AddCloudfrontDistributionWithProvider adds a `cloudfront-distribution` resource under name, provisioned by p.
func (rs *ResourceSet) AddCloudfrontDistributionWithProvider(p providers.CredsProvider, name string, r resource.CloudfrontDistribution) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.CloudfrontDistribution, "cloudfront-distribution", key, r)
}

// AddCloudfrontFunction adds a `cloudfront-function` resource under name.
func (rs *ResourceSet) AddCloudfrontFunction(name string, r resource.CloudfrontFunction) error {
	return addResource(&rs.CloudfrontFunction, "cloudfront-function", name, r)
}

// AddCloudfrontFunctionWithProvider adds a `cloudfront-function` resource under name, provisioned by p.
func (rs *ResourceSet) AddCloudfrontFunctionWithProvider(p providers.CredsProvider, name string, r resource.CloudfrontFunction) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.CloudfrontFunction, "cloudfront-function", key, r)
}

// AddCloudfrontVpcOrigin adds a `cloudfront-vpc-origin` resource under name.
func (rs *ResourceSet) AddCloudfrontVpcOrigin(name string, r resource.CloudfrontVpcOrigin) error {
	return addResource(&rs.CloudfrontVpcOrigin, "cloudfront-vpc-origin", name, r)
}

// AddCloudfrontVpcOriginWithProvider adds a `cloudfront-vpc-origin` resource under name, provisioned by p.
func (rs *ResourceSet) AddCloudfrontVpcOriginWithProvider(p providers.CredsProvider, name string, r resource.CloudfrontVpcOrigin) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.CloudfrontVpcOrigin, "cloudfront-vpc-origin", key, r)
}

// AddCWLogGroup adds a `cloudwatch-loggroup` resource under name.
func (rs *ResourceSet) AddCWLogGroup(name string, r resource.CWLogGroup) error {
	return addResource(&rs.CWLogGroup, "cloudwatch-loggroup", name, r)
}

// AddCWLogGroupWithProvider adds a `cloudwatch-loggroup` resource under name, provisioned by p.
func (rs *ResourceSet) AddCWLogGroupWithProvider(p providers.CredsProvider, name string, r resource.CWLogGroup) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.CWLogGroup, "cloudwatch-loggroup", key, r)
}

// AddCWMetricAlarm adds a `cloudwatch-metricalarm` resource under name.
func (rs *ResourceSet) AddCWMetricAlarm(name string, r resource.CWMetricAlarm) error {
	return addResource(&rs.CWMetricAlarm, "cloudwatch-metricalarm", name, r)
}

// AddCWMetricAlarmWithProvider adds a `cloudwatch-metricalarm` resource under name, provisioned by p.
func (rs *ResourceSet) AddCWMetricAlarmWithProvider(p providers.CredsProvider, name string, r resource.CWMetricAlarm) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.CWMetricAlarm, "cloudwatch-metricalarm", key, r)
}

// AddCWSubscriptionFilter adds a `cloudwatch-subscriptionfilter` resource under name.
func (rs *ResourceSet) AddCWSubscriptionFilter(name string, r resource.CWSubscriptionFilter) error {
	return addResource(&rs.CWSubscriptionFilter, "cloudwatch-subscriptionfilter", name, r)
}

// AddCWSubscriptionFilterWithProvider adds a `cloudwatch-subscriptionfilter` resource under name, provisioned by p.
func (rs *ResourceSet) AddCWSubscriptionFilterWithProvider(p providers.CredsProvider, name string, r resource.CWSubscriptionFilter) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.CWSubscriptionFilter, "cloudwatch-subscriptionfilter", key, r)
}

// AddDynamoDB adds a `dynamodb-table` resource under name.
func (rs *ResourceSet) AddDynamoDB(name string, r resource.DynamoDB) error {
	return addResource(&rs.DynamoDB, "dynamodb-table", name, r)
}

// AddDynamoDBWithProvider adds a `dynamodb-table` resource under name, provisioned by p.
func (rs *ResourceSet) AddDynamoDBWithProvider(p providers.CredsProvider, name string, r resource.DynamoDB) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.DynamoDB, "dynamodb-table", key, r)
}

// AddECSService adds a `ecs-service` resource under name.
func (rs *ResourceSet) AddECSService(name string, r resource.ECSService) error {
	return addResource(&rs.ECSService, "ecs-service", name, r)
}

// AddECSServiceWithProvider adds a `ecs-service` resource under name, provisioned by p.
func (rs *ResourceSet) AddECSServiceWithProvider(p providers.CredsProvider, name string, r resource.ECSService) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.ECSService, "ecs-service", key, r)
}

// AddEFSFileSystem adds a `efs-filesystem` resource under name.
func (rs *ResourceSet) AddEFSFileSystem(name string, r resource.EFSFileSystem) error {
	return addResource(&rs.EFSFileSystem, "efs-filesystem", name, r)
}

// AddEFSFileSystemWithProvider adds a `efs-filesystem` resource under name, provisioned by p.
func (rs *ResourceSet) AddEFSFileSystemWithProvider(p providers.CredsProvider, name string, r resource.EFSFileSystem) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.EFSFileSystem, "efs-filesystem", key, r)
}

// AddEventBridgeApiDestination adds a `eventbridge-apidestination` resource under name.
func (rs *ResourceSet) AddEventBridgeApiDestination(name string, r resource.EventBridgeApiDestination) error {
	return addResource(&rs.EventBridgeApiDestination, "eventbridge-apidestination", name, r)
}

// AddEventBridgeApiDestinationWithProvider adds a `eventbridge-apidestination` resource under name, provisioned by p.
func (rs *ResourceSet) AddEventBridgeApiDestinationWithProvider(p providers.CredsProvider, name string, r resource.EventBridgeApiDestination) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.EventBridgeApiDestination, "eventbridge-apidestination", key, r)
}

// AddEventBridgeRule adds a `eventbridge-rule` resource under name.
func (rs *ResourceSet) AddEventBridgeRule(name string, r resource.EventBridgeRule) error {
	return addResource(&rs.EventBridgeRule, "eventbridge-rule", name, r)
}

// AddEventBridgeRuleWithProvider adds a `eventbridge-rule` resource under name, provisioned by p.
func (rs *ResourceSet) AddEventBridgeRuleWithProvider(p providers.CredsProvider, name string, r resource.EventBridgeRule) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.EventBridgeRule, "eventbridge-rule", key, r)
}

// AddEventBridgeConnection adds a `eventbridge-connection` resource under name.
func (rs *ResourceSet) AddEventBridgeConnection(name string, r resource.EventBridgeApiConnection) error {
	return addResource(&rs.EventBridgeConnection, "eventbridge-connection", name, r)
}

// AddEventBridgeConnectionWithProvider adds a `eventbridge-connection` resource under name, provisioned by p.
func (rs *ResourceSet) AddEventBridgeConnectionWithProvider(p providers.CredsProvider, name string, r resource.EventBridgeApiConnection) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.EventBridgeConnection, "eventbridge-connection", key, r)
}

// AddFirehoseDeliveryStream adds a `firehose-delivery-stream` resource under name.
func (rs *ResourceSet) AddFirehoseDeliveryStream(name string, r firehose.FirehoseDeliveryStream) error {
	return addResource(&rs.FirehoseDeliveryStream, "firehose-delivery-stream", name, r)
}

// AddFirehoseDeliveryStreamWithProvider adds a `firehose-delivery-stream` resource under name, provisioned by p.
func (rs *ResourceSet) AddFirehoseDeliveryStreamWithProvider(p providers.CredsProvider, name string, r firehose.FirehoseDeliveryStream) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.FirehoseDeliveryStream, "firehose-delivery-stream", key, r)
}

// AddIAMPolicy adds a `iam-policy` resource under name.
func (rs *ResourceSet) AddIAMPolicy(name string, r resource.IAMPolicy) error {
	return addResource(&rs.IAMPolicy, "iam-policy", name, r)
}

// AddIAMPolicyWithProvider adds a `iam-policy` resource under name, provisioned by p.
func (rs *ResourceSet) AddIAMPolicyWithProvider(p providers.CredsProvider, name string, r resource.IAMPolicy) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.IAMPolicy, "iam-policy", key, r)
}

// AddIAMRole adds a `iam-role` resource under name.
func (rs *ResourceSet) AddIAMRole(name string, r resource.IAMRole) error {
	return addResource(&rs.IAMRole, "iam-role", name, r)
}

// AddIAMRoleWithProvider adds a `iam-role` resource under name, provisioned by p.
func (rs *ResourceSet) AddIAMRoleWithProvider(p providers.CredsProvider, name string, r resource.IAMRole) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.IAMRole, "iam-role", key, r)
}

// AddLambdaFn adds a `lambda-function` resource under name.
func (rs *ResourceSet) AddLambdaFn(name string, r lambda.Function) error {
	return addResource(&rs.LambdaFn, "lambda-function", name, r)
}

// AddLambdaFnWithProvider adds a `lambda-function` resource under name, provisioned by p.
func (rs *ResourceSet) AddLambdaFnWithProvider(p providers.CredsProvider, name string, r lambda.Function) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.LambdaFn, "lambda-function", key, r)
}

// AddLambdaLayer adds a `lambda-layer` resource under name.
func (rs *ResourceSet) AddLambdaLayer(name string, r lambda.Layer) error {
	return addResource(&rs.LambdaLayer, "lambda-layer", name, r)
}

// AddLambdaLayerWithProvider adds a `lambda-layer` resource under name, provisioned by p.
func (rs *ResourceSet) AddLambdaLayerWithProvider(p providers.CredsProvider, name string, r lambda.Layer) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.LambdaLayer, "lambda-layer", key, r)
}

// AddLBTargetGroup adds a `lb-targetgroup` resource under name.
func (rs *ResourceSet) AddLBTargetGroup(name string, r resource.LBTargetGroup) error {
	return addResource(&rs.LBTargetGroup, "lb-targetgroup", name, r)
}

// AddLBTargetGroupWithProvider adds a `lb-targetgroup` resource under name, provisioned by p.
func (rs *ResourceSet) AddLBTargetGroupWithProvider(p providers.CredsProvider, name string, r resource.LBTargetGroup) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.LBTargetGroup, "lb-targetgroup", key, r)
}

// AddLoadBalancer adds a `load-balancer` resource under name.
func (rs *ResourceSet) AddLoadBalancer(name string, r resource.LoadBalancer) error {
	return addResource(&rs.LoadBalancer, "load-balancer", name, r)
}

// AddLoadBalancerWithProvider adds a `load-balancer` resource under name, provisioned by p.
func (rs *ResourceSet) AddLoadBalancerWithProvider(p providers.CredsProvider, name string, r resource.LoadBalancer) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.LoadBalancer, "load-balancer", key, r)
}

// AddMSKConnector adds a `msk-connector` resource under name.
func (rs *ResourceSet) AddMSKConnector(name string, r resource.MSKConnector) error {
	return addResource(&rs.MSKConnector, "msk-connector", name, r)
}

// AddMSKConnectorWithProvider adds a `msk-connector` resource under name, provisioned by p.
func (rs *ResourceSet) AddMSKConnectorWithProvider(p providers.CredsProvider, name string, r resource.MSKConnector) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.MSKConnector, "msk-connector", key, r)
}

// AddMSKPlugin adds a `msk-plugin` resource under name.
func (rs *ResourceSet) AddMSKPlugin(name string, r resource.MSKPlugin) error {
	return addResource(&rs.MSKPlugin, "msk-plugin", name, r)
}

// AddMSKPluginWithProvider adds a `msk-plugin` resource under name, provisioned by p.
func (rs *ResourceSet) AddMSKPluginWithProvider(p providers.CredsProvider, name string, r resource.MSKPlugin) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.MSKPlugin, "msk-plugin", key, r)
}

// AddMSKWorkerConfiguration adds a `msk-worker-configuration` resource under name.
func (rs *ResourceSet) AddMSKWorkerConfiguration(name string, r resource.MSKWorkerConfiguration) error {
	return addResource(&rs.MSKWorkerConfiguration, "msk-worker-configuration", name, r)
}

// AddMSKWorkerConfigurationWithProvider adds a `msk-worker-configuration` resource under name, provisioned by p.
func (rs *ResourceSet) AddMSKWorkerConfigurationWithProvider(p providers.CredsProvider, name string, r resource.MSKWorkerConfiguration) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.MSKWorkerConfiguration, "msk-worker-configuration", key, r)
}

// AddRoute53Record adds a `route53-record` resource under name.
func (rs *ResourceSet) AddRoute53Record(name string, r resource.Route53Record) error {
	return addResource(&rs.Route53Record, "route53-record", name, r)
}

// AddRoute53RecordWithProvider adds a `route53-record` resource under name, provisioned by p.
func (rs *ResourceSet) AddRoute53RecordWithProvider(p providers.CredsProvider, name string, r resource.Route53Record) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.Route53Record, "route53-record", key, r)
}

// AddS3Bucket adds a `s3-bucket` resource under name.
func (rs *ResourceSet) AddS3Bucket(name string, r resource.S3Bucket) error {
	return addResource(&rs.S3Bucket, "s3-bucket", name, r)
}

// AddS3BucketWithProvider adds a `s3-bucket` resource under name, provisioned by p.
func (rs *ResourceSet) AddS3BucketWithProvider(p providers.CredsProvider, name string, r resource.S3Bucket) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.S3Bucket, "s3-bucket", key, r)
}

// AddSDService adds a `sd-service` resource under name.
func (rs *ResourceSet) AddSDService(name string, r resource.SDService) error {
	return addResource(&rs.SDService, "sd-service", name, r)
}

// AddSDServiceWithProvider adds a `sd-service` resource under name, provisioned by p.
func (rs *ResourceSet) AddSDServiceWithProvider(p providers.CredsProvider, name string, r resource.SDService) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.SDService, "sd-service", key, r)
}

// AddSecurityGroup adds a `security-group` resource under name.
func (rs *ResourceSet) AddSecurityGroup(name string, r resource.SecurityGroup) error {
	return addResource(&rs.SecurityGroup, "security-group", name, r)
}

// AddSecurityGroupWithProvider adds a `security-group` resource under name, provisioned by p.
func (rs *ResourceSet) AddSecurityGroupWithProvider(p providers.CredsProvider, name string, r resource.SecurityGroup) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.SecurityGroup, "security-group", key, r)
}

// AddSQSQueue adds a `sqs-queue` resource under name.
func (rs *ResourceSet) AddSQSQueue(name string, r resource.SQSQueue) error {
	return addResource(&rs.SQSQueue, "sqs-queue", name, r)
}

// AddSQSQueueWithProvider adds a `sqs-queue` resource under name, provisioned by p.
func (rs *ResourceSet) AddSQSQueueWithProvider(p providers.CredsProvider, name string, r resource.SQSQueue) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.SQSQueue, "sqs-queue", key, r)
}

// AddSNSSubscription adds a `sns-subscription` resource under name.
func (rs *ResourceSet) AddSNSSubscription(name string, r resource.SNSSubscription) error {
	return addResource(&rs.SNSSubscription, "sns-subscription", name, r)
}

// AddSNSSubscriptionWithProvider adds a `sns-subscription` resource under name, provisioned by p.
func (rs *ResourceSet) AddSNSSubscriptionWithProvider(p providers.CredsProvider, name string, r resource.SNSSubscription) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.SNSSubscription, "sns-subscription", key, r)
}

// AddStandaloneTask adds a `standalone-task` resource under name.
func (rs *ResourceSet) AddStandaloneTask(name string, r resource.StandaloneTask) error {
	return addResource(&rs.StandaloneTask, "standalone-task", name, r)
}

// AddStandaloneTaskWithProvider adds a `standalone-task` resource under name, provisioned by p.
func (rs *ResourceSet) AddStandaloneTaskWithProvider(p providers.CredsProvider, name string, r resource.StandaloneTask) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.StandaloneTask, "standalone-task", key, r)
}

// AddStateMachine adds a `state-machine` resource under name.
func (rs *ResourceSet) AddStateMachine(name string, r resource.StateMachine) error {
	return addResource(&rs.StateMachine, "state-machine", name, r)
}

// AddStateMachineWithProvider adds a `state-machine` resource under name, provisioned by p.
func (rs *ResourceSet) AddStateMachineWithProvider(p providers.CredsProvider, name string, r resource.StateMachine) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.StateMachine, "state-machine", key, r)
}

// AddTaskDef adds a `taskdef` resource under name.
func (rs *ResourceSet) AddTaskDef(name string, r resource.TaskDef) error {
	return addResource(&rs.TaskDef, "taskdef", name, r)
}

// AddTaskDefWithProvider adds a `taskdef` resource under name, provisioned by p.
func (rs *ResourceSet) AddTaskDefWithProvider(p providers.CredsProvider, name string, r resource.TaskDef) error {
	key, err := rs.providerKey(p, name)
	if err != nil {
		return err
	}
	return addResource(&rs.TaskDef, "taskdef", key, r)
}
