package resource

import "context"

// NormalizableResource is a Resource whose definition is prepared before use:
// defaults filled in, inputs sanitized, global tags merged, names resolved.
//
// Normalize mutates the receiver, so every implementation uses a pointer
// receiver. Only *T satisfies this interface; a generic builder must therefore
// hold resources by pointer until Normalize has run.
type NormalizableResource interface {
	Resource
	Normalize(context.Context)
}

// ValidatableResource is a Resource that checks its own business rules after
// normalization and reports every failure at once as a ValidationError.
//
// Validate does not mutate the receiver and uses a value receiver, like
// Compare, Apply and Destroy. Both T and *T satisfy this interface.
type ValidatableResource interface {
	Resource
	Validate(context.Context) error
}

// Every top-level resource type registered in config's resourcesConfig must
// satisfy both lifecycle interfaces. The pointer form is asserted because the
// pointer method set contains the value methods too, so a single line catches
// a Normalize that was given a value receiver or a Validate with the wrong
// signature at build time rather than when config runs.
//
// Nested sub-resources (LBListener, LBRule, EventBridgeTarget,
// ApplicationAutoScaling, EFSAccessPoint, EFSMountTarget) take an extra
// resource Context in their lifecycle methods and are deliberately absent:
// they are not graph vertices. lambda and firehose assert their own types.
var (
	_ NormalizableResource = (*ACMCertificate)(nil)
	_ ValidatableResource  = (*ACMCertificate)(nil)
	_ NormalizableResource = (*BedrockApplicationInferenceProfile)(nil)
	_ ValidatableResource  = (*BedrockApplicationInferenceProfile)(nil)
	_ NormalizableResource = (*CloudfrontDistribution)(nil)
	_ ValidatableResource  = (*CloudfrontDistribution)(nil)
	_ NormalizableResource = (*CloudfrontFunction)(nil)
	_ ValidatableResource  = (*CloudfrontFunction)(nil)
	_ NormalizableResource = (*CloudfrontVpcOrigin)(nil)
	_ ValidatableResource  = (*CloudfrontVpcOrigin)(nil)
	_ NormalizableResource = (*CWLogGroup)(nil)
	_ ValidatableResource  = (*CWLogGroup)(nil)
	_ NormalizableResource = (*CWMetricAlarm)(nil)
	_ ValidatableResource  = (*CWMetricAlarm)(nil)
	_ NormalizableResource = (*CWSubscriptionFilter)(nil)
	_ ValidatableResource  = (*CWSubscriptionFilter)(nil)
	_ NormalizableResource = (*DynamoDB)(nil)
	_ ValidatableResource  = (*DynamoDB)(nil)
	_ NormalizableResource = (*ECSService)(nil)
	_ ValidatableResource  = (*ECSService)(nil)
	_ NormalizableResource = (*EFSFileSystem)(nil)
	_ ValidatableResource  = (*EFSFileSystem)(nil)
	_ NormalizableResource = (*EventBridgeApiConnection)(nil)
	_ ValidatableResource  = (*EventBridgeApiConnection)(nil)
	_ NormalizableResource = (*EventBridgeApiDestination)(nil)
	_ ValidatableResource  = (*EventBridgeApiDestination)(nil)
	_ NormalizableResource = (*EventBridgeRule)(nil)
	_ ValidatableResource  = (*EventBridgeRule)(nil)
	_ NormalizableResource = (*IAMPolicy)(nil)
	_ ValidatableResource  = (*IAMPolicy)(nil)
	_ NormalizableResource = (*IAMRole)(nil)
	_ ValidatableResource  = (*IAMRole)(nil)
	_ NormalizableResource = (*LBTargetGroup)(nil)
	_ ValidatableResource  = (*LBTargetGroup)(nil)
	_ NormalizableResource = (*LoadBalancer)(nil)
	_ ValidatableResource  = (*LoadBalancer)(nil)
	_ NormalizableResource = (*MSKConnector)(nil)
	_ ValidatableResource  = (*MSKConnector)(nil)
	_ NormalizableResource = (*MSKPlugin)(nil)
	_ ValidatableResource  = (*MSKPlugin)(nil)
	_ NormalizableResource = (*MSKWorkerConfiguration)(nil)
	_ ValidatableResource  = (*MSKWorkerConfiguration)(nil)
	_ NormalizableResource = (*Route53Record)(nil)
	_ ValidatableResource  = (*Route53Record)(nil)
	_ NormalizableResource = (*S3Bucket)(nil)
	_ ValidatableResource  = (*S3Bucket)(nil)
	_ NormalizableResource = (*SDService)(nil)
	_ ValidatableResource  = (*SDService)(nil)
	_ NormalizableResource = (*SecurityGroup)(nil)
	_ ValidatableResource  = (*SecurityGroup)(nil)
	_ NormalizableResource = (*SNSSubscription)(nil)
	_ ValidatableResource  = (*SNSSubscription)(nil)
	_ NormalizableResource = (*SQSQueue)(nil)
	_ ValidatableResource  = (*SQSQueue)(nil)
	_ NormalizableResource = (*StandaloneTask)(nil)
	_ ValidatableResource  = (*StandaloneTask)(nil)
	_ NormalizableResource = (*StateMachine)(nil)
	_ ValidatableResource  = (*StateMachine)(nil)
	_ NormalizableResource = (*TaskDef)(nil)
	_ ValidatableResource  = (*TaskDef)(nil)
)
