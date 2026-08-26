package resource

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	eventbridgetypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

var awsPlaceholderRegex = regexp.MustCompile(`<[A-Za-z][A-Za-z0-9_-]*>`)

type EventBridgeTargetType string

const (
	EventBridgeTarget_EcsRunTask     EventBridgeTargetType = "ecs"
	EventBridgeTarget_ApiDestination EventBridgeTargetType = "api"
	EventBridgeTarget_StepFunction   EventBridgeTargetType = "sfn"
	EventBridgeTarget_Lambda         EventBridgeTargetType = "lambda"
	EventBridgeTarget_Firehose       EventBridgeTargetType = "firehose"
)

func EventBridgeTargetTypeValues() []string {
	return []string{
		string(EventBridgeTarget_EcsRunTask),
		string(EventBridgeTarget_ApiDestination),
		string(EventBridgeTarget_StepFunction),
		string(EventBridgeTarget_Lambda),
		string(EventBridgeTarget_Firehose),
	}
}

// ToEventBridgeTargetTypePtr converts to a *EventBridgeTargetType from *string
func ToEventBridgeTargetTypePtr(val *string) *EventBridgeTargetType {

	if val == nil {
		def := EventBridgeTarget_EcsRunTask
		return &def
	}

	value := EventBridgeTargetType(strings.ToLower(*val))
	return &value
}

func (e EventBridgeTargetType) String() string {
	return string(e)
}

type EventBridgeGenericInput struct {
	Value    *string           `yaml:"value"`
	JsonPath *string           `yaml:"jsonPath"`
	Template *string           `yaml:"template"`
	PathsMap map[string]string `yaml:"pathsMap"`
}

// EventBridgeTarget represents an EventBridge rule target; only ECS task supported for now
// the target definintion is done inline witht he EventBridgeRule
type EventBridgeTarget struct {
	ID             string                   `yaml:"id"`
	Role           string                   `yaml:"role"`
	TargetResource string                   `yaml:"targetResource"` // supply resource name for the correspondign type, ecs service name, api destination name, step fn name, lambda fn name[:alias|version]
	TargetType     *EventBridgeTargetType   `yaml:"type"`           // indicates the type of target, currently supported are ecs runTask & api Destination; default is ecs runTask
	ECSTask        *EventBridgeEcsTarget    `yaml:"ecsTask"`        // additional config when type = ecs
	APIDestination *EventBridgeApiTarget    `yaml:"httpParameters"` // additional config when type = api
	GenericInput   *EventBridgeGenericInput `yaml:"input"`          // raw eventbridge target input configuraiton; for some target types, it's configured from the respective parameters, e.g. ecs & api
	EventBusName   string                   `yaml:"-"`              // set by EventBridgeRule, for internal use
	RuleName       string                   `yaml:"-"`              // set by EventBridgeRule, for internal use
	GlobalTags     map[string]string        `yaml:"-"`              // set by EventBridgeRule, for internal use
	Tags           map[string]string        `yaml:"tags"`
}

// EventBridgeEcsTarget represents the parameters for an ECS task eventbridge rule target
type EventBridgeEcsTarget struct {
	TaskCount         *int32                        `yaml:"taskCount"`
	TaskDefName       *string                       `yaml:"taskDefName"`
	LaunchType        *string                       `yaml:"launchType"`        // EC2 | Fargate | CAP are supported, CAP is default, using default capacity provider strategy
	CapacityProviders []ECSCapacityProviderStrategy `yaml:"capacityProviders"` // when CAP is used, specify a custom capacity provider strantegy; else cluster default is used
	PlatformVersion   *string                       `yaml:"platformVersion"`   // when launchType = Fargate, specify the platform version to use for this task
	AssignPublicIP    string                        `yaml:"assignPublicIp"`    // enabled only when awsvpc + public subnet is used
	Subnets           []string                      `yaml:"subnetNames"`
	SecurityGroups    []string                      `yaml:"securityGroupNames"`
	Overrides         *TaskOverrides                `yaml:"overrides"` // task overrides are a special case for ecs runTask type tagets & it's converted to target invocation `input`
}

// EventBridgeApiTarget represents the parameters for an API Destination eventbridge rule targe
type EventBridgeApiTarget struct {
	HeaderParameters      map[string]string `yaml:"headerParameters"`
	PathParameters        []string          `yaml:"pathParameters"`
	QueryStringParameters map[string]string `yaml:"queryStringParameters"`
	Payload               *string           `yaml:"payload"`
}

// Identifier returns name of the eventbridge target ID
func (t EventBridgeTarget) Identifier() string {
	return t.ID
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (t *EventBridgeTarget) Normalize(ctx context.Context) {

	// target type
	if t.TargetType == nil {
		t.TargetType = ToEventBridgeTargetTypePtr(nil)
	} else {
		t.TargetType = ToEventBridgeTargetTypePtr(aws.String(string(*t.TargetType)))
	}

	if *t.TargetType == EventBridgeTarget_EcsRunTask {
		if t.ECSTask != nil {
			if t.ECSTask.LaunchType == nil {
				t.ECSTask.LaunchType = aws.String(CAP)
			}
			t.ECSTask.LaunchType = aws.String(strings.ToUpper(*t.ECSTask.LaunchType))

			if t.ECSTask.TaskCount == nil {
				t.ECSTask.TaskCount = aws.Int32(1) // set default to 1
			}
		}
	}
}

// Validate checks that the input provided is correct
func (t EventBridgeTarget) Validate(ctx context.Context, rctx Context) error {
	var errMessages []string

	switch *t.TargetType {
	case EventBridgeTarget_ApiDestination, EventBridgeTarget_EcsRunTask, EventBridgeTarget_StepFunction, EventBridgeTarget_Lambda, EventBridgeTarget_Firehose:
	default:
		errMessages = append(errMessages, fmt.Sprintf("invalid target type supplied, supported types are: %v", EventBridgeTargetTypeValues()))
	}

	if len(t.TargetResource) == 0 {
		errMessages = append(errMessages, "targetResource must be specified: the delivery stream name for firehose, function name for lambda, state machine name for sfn, cluster name for ecs, or destination name for api")
	}

	switch *t.TargetType {
	case EventBridgeTarget_EcsRunTask:

		if t.ECSTask == nil {
			errMessages = append(errMessages, "ecsTask must be provided for the target")
		} else {
			task := t.ECSTask

			switch *task.LaunchType {
			case CAP, Fargate, EC2:
			default:
				//invalid launch type
				msg := fmt.Sprintf("invalid launch type %v, must be `CAP`, 'EC2' or 'FARGATE'", task.LaunchType)
				errMessages = append(errMessages, msg)
			}

			if task.AssignPublicIP != "" {
				if task.AssignPublicIP != string(ecstypes.AssignPublicIpEnabled) && task.AssignPublicIP != string(ecstypes.AssignPublicIpDisabled) {
					msg := fmt.Sprintf("invalid value for assignPublicIp %q, must be 'ENABLED' or 'DISABLED'", task.AssignPublicIP)
					errMessages = append(errMessages, msg)
				}
			}
		}
	case EventBridgeTarget_ApiDestination:
		numPathParms := 0
		if t.APIDestination != nil {
			numPathParms = len(t.APIDestination.PathParameters)
		}
		endpoint, _ := awsw.NewEventBridge(ctx, rctx.ProviderName).ApiDestinationEndpointForName(ctx, t.TargetResource)
		if endpoint != nil {
			wildcards := strings.Count(*endpoint, "*")
			if wildcards != numPathParms {
				msg := fmt.Sprintf("number of path parameters (%v) must be equal to the target api destination wildcards (%v)", numPathParms, wildcards)
				errMessages = append(errMessages, msg)
			}
		}
	case EventBridgeTarget_StepFunction, EventBridgeTarget_Lambda, EventBridgeTarget_Firehose:
		// generic input supplied
		if t.GenericInput != nil {
			var inputCount int32
			if t.GenericInput.Value != nil {
				inputCount++
			}
			if t.GenericInput.JsonPath != nil {
				inputCount++
			}
			if t.GenericInput.Template != nil || t.GenericInput.PathsMap != nil {
				inputCount++
			}

			if inputCount > 1 {
				msg := "only one of input, json path or input transformer can be supplied"
				errMessages = append(errMessages, msg)
			}

			if t.GenericInput.PathsMap != nil && t.GenericInput.Template == nil {
				errMessages = append(errMessages, "input transformer pathsMap requires a template to be set")
			}

			if t.GenericInput.Template != nil && t.GenericInput.PathsMap == nil {
				if awsPlaceholderRegex.MatchString(*t.GenericInput.Template) {
					errMessages = append(errMessages, "input transformer template contains placeholders but no pathsMap is defined")
				}
			}
		}
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: t.Identifier(),
		ResourceType:       "EventBridgeTarget",
		Messages:           errMessages,
	}
}

// equals reutrns true if the supplied target is the same as this, else false
func (t EventBridgeTarget) equals(ctx context.Context, rctx Context, existing *eventbridgetypes.Target) (bool, []string, error) {

	equal := true
	messages := make([]string, 0)

	var arn *string
	var err error

	targetResourceIsSame := true

	// target arn/resource

	switch *t.TargetType {
	case EventBridgeTarget_EcsRunTask:
		arn, err = awsw.NewECS(ctx, rctx.ProviderName).EcsClusterArnForName(ctx, t.TargetResource)
	case EventBridgeTarget_ApiDestination:
		arn, err = awsw.NewEventBridge(ctx, rctx.ProviderName).ApiDestinationArnForName(ctx, t.TargetResource)
	case EventBridgeTarget_StepFunction:
		name, qualifier := SplitNameQualifier(t.TargetResource)
		arn, err = awsw.NewSFN(ctx, rctx.ProviderName).StateMachineArnForNameAndQualifer(ctx, name, qualifier)
	case EventBridgeTarget_Lambda:
		name, qualifier := SplitNameQualifier(t.TargetResource)
		arn, err = awsw.NewLambda(ctx, rctx.ProviderName).LambdaArnForNameAndQualifier(ctx, name, qualifier)
	case EventBridgeTarget_Firehose:
		arn, err = awsw.NewFirehose(ctx, rctx.ProviderName).DeliveryStreamArnForIdentifier(ctx, t.TargetResource)
	}

	if err != nil {
		return false, nil, err
	}

	switch {
	case arn == nil:
		equal = false
		messages = append(messages, fmt.Sprintf("target resource for target %q could not be resolved or does not exist", t.Identifier()))
		targetResourceIsSame = false
	case existing.Arn == nil || *arn != *existing.Arn:
		equal = false
		messages = append(messages, fmt.Sprintf("target resource for target %q will be updated: %v -> %v",
			t.Identifier(), util.Coalesce(existing.Arn, "<none>"), *arn))
		targetResourceIsSame = false
	}

	// ID
	// TODO: this diff doesn't make sense, it wont happen since if the IDs are different we will never be comparing them
	if t.ID != *existing.Id {
		equal = false
		messages = append(messages, fmt.Sprintf("existing target Id '%v' is not the same as defined '%v'", t.Identifier(), t.ID))
	}

	// role
	roleArn, err := awsw.NewIAM(ctx, rctx.ProviderName).RoleArnForName(ctx, t.Role)
	if err != nil {
		return false, nil, err
	}

	if util.Coalesce(roleArn, "") != util.Coalesce(existing.RoleArn, "") {
		equal = false
		messages = append(messages, fmt.Sprintf("ecs task role for target %q will be updated %v -> %v'",
			t.Identifier(), util.Coalesce(existing.RoleArn, ""), util.Coalesce(roleArn, "")))
	}

	// parameters
	if targetResourceIsSame {
		switch *t.TargetType {
		case EventBridgeTarget_EcsRunTask: // ecs
			eq, msgs, err := t.equalsEcsParameters(ctx, rctx, existing)
			if err != nil {
				return false, nil, err
			}
			if !eq {
				equal = false
				messages = append(messages, msgs...)
			}
		case EventBridgeTarget_ApiDestination: // api destination
			eq, msgs, err := t.equalsHttpParameters(existing)
			if err != nil {
				return false, nil, err
			}
			if !eq {
				equal = false
				messages = append(messages, msgs...)
			}
		case EventBridgeTarget_StepFunction, EventBridgeTarget_Lambda, EventBridgeTarget_Firehose: // step function, lambda, or firehose
			// TODO

			existingInputIsNil := existing.Input == nil && existing.InputTransformer == nil && existing.InputPath == nil
			if t.GenericInput == nil && !existingInputIsNil {
				equal = false
				messages = append(messages, fmt.Sprintf("input configuration for target %q configuration will be deleted", t.Identifier()))
			} else if t.GenericInput != nil && existingInputIsNil {
				equal = false
				messages = append(messages, fmt.Sprintf("input configuration for target %q configuration will be added", t.Identifier()))
			} else if t.GenericInput != nil && !existingInputIsNil {
				tinput := t.GenericInput
				if util.Coalesce(tinput.Value, "") != util.Coalesce(existing.Input, "") {
					equal = false
					messages = append(messages, fmt.Sprintf("rule input value for target %q will be updated %v -> %v",
						t.Identifier(), util.Coalesce(existing.Input, ""), util.Coalesce(tinput.Value, "")))
				}
				if util.Coalesce(tinput.JsonPath, "") != util.Coalesce(existing.InputPath, "") {
					equal = false
					messages = append(messages, fmt.Sprintf("rule input json path for target %q will be updated %v -> %v",
						t.Identifier(), util.Coalesce(existing.InputPath, ""), util.Coalesce(tinput.JsonPath, "")))
				}

				var existingTemplate *string
				var existingMap map[string]string

				if existing.InputTransformer != nil {
					existingTemplate = existing.InputTransformer.InputTemplate
					existingMap = existing.InputTransformer.InputPathsMap
				}

				if strings.TrimSpace(util.Coalesce(tinput.Template, "")) != strings.TrimSpace(util.Coalesce(existingTemplate, "")) {
					equal = false
					messages = append(messages, fmt.Sprintf("rule input value for target %q will be updated %v\n -> \n%v",
						t.Identifier(), util.Coalesce(existingTemplate, ""), util.Coalesce(tinput.Template, "")))
				} else if !util.StringMap(tinput.PathsMap).Equals(existingMap) {
					equal = false
					messages = append(messages, fmt.Sprintf("rule input paths map for target %q will be updated, %v -> %v",
						t.Identifier(), existingMap, tinput.PathsMap))
				}
			}
		}
	}

	if equal {
		return true, nil, nil
	} else {
		return false, messages, nil
	}
}

// equalsEcsParameters compares this resource spec's ecs parameters with the supplied existing ecs parameters,
// and returns a true|false value when the resources are equal|not equal, a list of diffs or a non-nil error
// if there equals comparison fails
func (t EventBridgeTarget) equalsEcsParameters(ctx context.Context, rctx Context, existing *eventbridgetypes.Target) (bool, []string, error) {

	equal := true
	var messages []string

	if t.ECSTask != nil && (existing == nil || existing.EcsParameters == nil) {
		equal = false
		messages = append(messages, fmt.Sprintf("no ecs task parameters found for the existing target '%v'", t.Identifier()))
	}

	input := existing.Input
	if input != nil {
		var awsOverrides TaskOverrides
		err := awsOverrides.parseJson(*input)
		if err != nil {
			return false, nil, err
		}
		if eq, msgs := awsOverrides.equals(t.ECSTask.Overrides); !eq {
			equal = false
			messages = append(messages, fmt.Sprintf("ecs task overrides do not match for target '%v'", t.Identifier()))
			messages = append(messages, msgs...)
		}
	} else if t.ECSTask.Overrides != nil {
		//if aws overrides are nil but defined...
		equal = false
		messages = append(messages, fmt.Sprintf("ecs task overrides do not match for target '%v'", t.Identifier()))
	}

	if t.ECSTask == nil && existing.EcsParameters != nil ||
		t.ECSTask != nil && existing.EcsParameters == nil {
		equal = false
		messages = append(messages, fmt.Sprintf("ecs task parameters do not match for target '%v'", t.Identifier()))
	}

	ecsParms := t.ECSTask
	existingEcsParms := existing.EcsParameters

	taskDef, err := taskDefByName(ctx, rctx, *t.ECSTask.TaskDefName)
	if err != nil {
		return false, nil, err
	}

	// if defined taskdef name doesn't specify revision, strip it from arn before comparing
	taskDefArn := *taskDef.TaskDefinitionArn
	if !strings.Contains(*t.ECSTask.TaskDefName, ":") {
		i := strings.LastIndex(*taskDef.TaskDefinitionArn, ":")
		taskDefArn = (*taskDef.TaskDefinitionArn)[:i]
	}

	if taskDefArn != *existingEcsParms.TaskDefinitionArn {
		equal = false

		messages = append(messages,
			fmt.Sprintf("ecs task definition does not match for target '%v' %v -> %v",
				t.Identifier(),
				(*existingEcsParms.TaskDefinitionArn)[strings.LastIndex(*existingEcsParms.TaskDefinitionArn, "/")+1:],
				taskDefArn[strings.LastIndex(taskDefArn, "/")+1:]))
	}

	//launchType/CAP
	if *ecsParms.LaunchType == CAP {
		//if new launchType = CAP, old must be CAP too, so launchType should be nil
		if existingEcsParms.LaunchType != "" {
			equal = false
			messages = append(messages, fmt.Sprintf("ecs task capacity provider parameters do not match for target '%v'", t.Identifier()))
		}
		newIsDefaultStrategy := len(ecsParms.CapacityProviders) == 0
		oldWasDefaultStrategy := len(existingEcsParms.CapacityProviderStrategy) == 0
		if oldWasDefaultStrategy && !newIsDefaultStrategy || !oldWasDefaultStrategy && newIsDefaultStrategy {
			equal = false
			messages = append(messages, fmt.Sprintf("ecs task capacity provider parameters do not match for target '%v'", t.Identifier()))
		}
		if !oldWasDefaultStrategy && !newIsDefaultStrategy {
			if diffEventbridgeCapacityProviderStrategies(existingEcsParms.CapacityProviderStrategy, ecsParms.CapacityProviders) {
				equal = false
				messages = append(messages, fmt.Sprintf("ecs task capacity provider parameters do not match for target '%v'", t.Identifier()))
			}
		}
	} else {
		if *ecsParms.LaunchType != string(existingEcsParms.LaunchType) {
			equal = false
			messages = append(messages, fmt.Sprintf("ecs task launch types do not match for target '%v'", t.Identifier()))
		}
	}

	if ecsParms.TaskCount != nil && *ecsParms.TaskCount != *existingEcsParms.TaskCount {
		equal = false
		messages = append(messages, fmt.Sprintf("ecs task count does not match for target '%v'", t.Identifier()))
	}

	//awsvpc networking config
	existingNetworkConfig := existingEcsParms.NetworkConfiguration != nil &&
		existingEcsParms.NetworkConfiguration.AwsvpcConfiguration != nil

	if !existingNetworkConfig {
		if ecsParms.AssignPublicIP != "" || len(ecsParms.Subnets) > 0 || len(ecsParms.SecurityGroups) > 0 {
			equal = false
			messages = append(messages, fmt.Sprintf("ecs task network configurations do not match for target '%v'", t.Identifier()))
		}
	} else {
		existingAwsvpcConfig := existingEcsParms.NetworkConfiguration.AwsvpcConfiguration

		//assignedPublicIP
		if ecsParms.AssignPublicIP == "" && existingAwsvpcConfig.AssignPublicIp != "" ||
			ecsParms.AssignPublicIP != "" && existingAwsvpcConfig.AssignPublicIp == "" {
			equal = false
			messages = append(messages, fmt.Sprintf("ecs task assign public ip do not match for target '%v'", t.Identifier()))
		}

		if ecsParms.AssignPublicIP != "" && existingAwsvpcConfig.AssignPublicIp != "" {
			if ecsParms.AssignPublicIP != string(existingAwsvpcConfig.AssignPublicIp) {
				equal = false
				messages = append(messages, fmt.Sprintf("ecs task assign public ip value do not match for target '%v'", t.Identifier()))
			}
		}

		//check subnet counts
		if len(ecsParms.Subnets) != len(existingAwsvpcConfig.Subnets) {
			equal = false
			messages = append(messages, fmt.Sprintf("ecs task subnets not match for target '%v'", t.Identifier()))
		}

		//check security group counts
		if len(ecsParms.SecurityGroups) != len(existingAwsvpcConfig.SecurityGroups) {
			equal = false
			messages = append(messages, fmt.Sprintf("ecs task security groups not match for target '%v'", t.Identifier()))
		}

		//match subnets
		subnetIDs, err := awsw.NewEC2(ctx, rctx.ProviderName).SubnetIdsByNames(ctx, ecsParms.Subnets)
		if err != nil {
			return false, nil, err
		}

		if !util.SliceElementsEqual(subnetIDs, existingAwsvpcConfig.Subnets) {
			equal = false
			messages = append(messages, fmt.Sprintf("ecs task subnets not match for target '%v'", t.Identifier()))
		}

		//match security groups
		securityGroupIDs, err := awsw.NewEC2(ctx, rctx.ProviderName).SecurityGroupIdsByNames(ctx, nil, ecsParms.SecurityGroups)
		if err != nil {
			return false, nil, err
		}

		if !util.SliceElementsEqual(securityGroupIDs, existingAwsvpcConfig.SecurityGroups) {
			equal = false
			messages = append(messages, fmt.Sprintf("ecs task security groups not match for target '%v'", t.Identifier()))
		}
	}

	if equal {
		return true, nil, nil
	} else {
		return false, messages, nil
	}
}

// equalsHttpParameters compares this resource spec's http parameters with the supplied existing http parameters,
// and returns a true|false value when the resources are equal|not equal, a list of diffs or a non-nil error
// if there equals comparison fails
func (t EventBridgeTarget) equalsHttpParameters(existing *eventbridgetypes.Target) (bool, []string, error) {

	equal := true
	var messages []string

	if t.APIDestination != nil && (existing == nil || existing.HttpParameters == nil) {
		equal = false
		messages = append(messages, fmt.Sprintf("no api destination parameters found for the existing target '%v'", t.Identifier()))
	} else if t.APIDestination == nil && (existing != nil && existing.HttpParameters != nil) {
		equal = false
		messages = append(messages, fmt.Sprintf("api destination parameters exist for existing target '%v'", t.Identifier()))
	} else if t.APIDestination != nil && (existing != nil && existing.HttpParameters != nil) {

		if !reflect.DeepEqual(t.APIDestination.HeaderParameters, existing.HttpParameters.HeaderParameters) {
			equal = false
			messages = append(messages, fmt.Sprintf("api destination header parameters are not the same '%v'", t.Identifier()))
		}

		if !reflect.DeepEqual(t.APIDestination.PathParameters, existing.HttpParameters.PathParameterValues) {
			equal = false
			messages = append(messages, fmt.Sprintf("api destination path parameters are not the same '%v'", t.Identifier()))
		}

		if !reflect.DeepEqual(t.APIDestination.QueryStringParameters, existing.HttpParameters.QueryStringParameters) {
			equal = false
			messages = append(messages, fmt.Sprintf("api destination query string parameters are not the same '%v'", t.Identifier()))
		}
	}

	// input json
	if util.Coalesce(t.APIDestination.Payload, "") != util.Coalesce(existing.Input, "") {
		equal = false
		messages = append(messages, fmt.Sprintf("api destination input json payloads are not the same '%v'", t.Identifier()))
	}

	if equal {
		return true, nil, nil
	} else {
		return false, messages, nil
	}
}

// fetchExisting returns the existing eventbridge target details if found
func (t EventBridgeTarget) fetchExisting(ctx context.Context, rctx Context) (*eventbridgetypes.Target, error) {

	ebrClient := client.EventBridge(ctx, rctx.ProviderName)
	out, err := ebrClient.ListTargetsByRule(ctx, &eventbridge.ListTargetsByRuleInput{
		EventBusName: aws.String(t.EventBusName),
		Rule:         aws.String(t.RuleName),
	})

	if err != nil {
		return nil, errors.Wrapf(err, "error retrieving target %v for eventbridge rule %v", t.ID, t.RuleName)
	}

	for _, target := range out.Targets {
		if t.ID == *target.Id {
			return &target, nil
		}
	}

	return nil, nil
}

// Apply creates a new eventbridge target for a rule
func (t EventBridgeTarget) Apply(ctx context.Context, rctx Context) error {

	//role
	roleArn, err := awsw.NewIAM(ctx, rctx.ProviderName).RoleArnForName(ctx, t.Role)
	if err != nil {
		return errors.Wrapf(err, "error fetching role for name %v", t.Role)
	}

	var targets []eventbridgetypes.Target
	switch *t.TargetType {
	case EventBridgeTarget_EcsRunTask:
		// when target is ECS task
		targetArn, ecsParams, err := t.getEcsTaskParameters(ctx, rctx)
		if err != nil {
			return errors.Wrapf(err, "errors when building ecs task parameters for eventbridge target %v", t.ID)
		}

		// for ecs run task target, the container overrides value marshalled as JSON is supplied
		// via the target input field
		var input *string
		if t.ECSTask.Overrides != nil {
			jsonStr, err := t.ECSTask.Overrides.toJsonString()
			if err != nil {
				return err
			}
			input = &jsonStr
		}

		targets = []eventbridgetypes.Target{{
			Arn:           targetArn,
			RetryPolicy:   nil,
			Id:            aws.String(t.ID),
			Input:         input,
			RoleArn:       roleArn,
			EcsParameters: ecsParams,
		}}

	case EventBridgeTarget_ApiDestination:

		targetArn, httpParams, err := t.getApiDestinationParameters(ctx, rctx)
		if err != nil {
			return errors.Wrapf(err, "errors when building http parameters for eventbridge target %v", t.ID)
		}

		// for API destination targets, the `input` parameter is used to send the request payload
		// payload for buildit api destination target is supplied using the `Payload` field
		var input *string
		if t.APIDestination != nil {
			input = t.APIDestination.Payload
		}

		targets = []eventbridgetypes.Target{{
			Arn:            targetArn,
			RetryPolicy:    nil,
			Id:             aws.String(t.ID),
			RoleArn:        roleArn,
			HttpParameters: httpParams,
			Input:          input,
		}}
	case EventBridgeTarget_StepFunction:

		name, qualifier := SplitNameQualifier(t.TargetResource)
		targetArn, err := awsw.NewSFN(ctx, rctx.ProviderName).StateMachineArnForNameAndQualifer(ctx, name, qualifier)
		if err != nil {
			return errors.Wrapf(err, "errors when resolving step function arn for eventbridge target %v", t.ID)
		}
		if targetArn == nil {
			return errors.Errorf("step function state machine %q not found", t.TargetResource)
		}

		input, path, transformer := t.getInputConfigFromGenericInput()

		targets = []eventbridgetypes.Target{{
			Arn:              targetArn,
			RetryPolicy:      nil,
			Id:               aws.String(t.ID),
			RoleArn:          roleArn,
			Input:            input,
			InputPath:        path,
			InputTransformer: transformer,
		}}
	case EventBridgeTarget_Lambda:

		name, qualifier := SplitNameQualifier(t.TargetResource)
		targetArn, err := awsw.NewLambda(ctx, rctx.ProviderName).LambdaArnForNameAndQualifier(ctx, name, qualifier)
		if err != nil {
			return errors.Wrapf(err, "errors when resolving lambda arn for eventbridge target %v", t.ID)
		}
		if targetArn == nil {
			return errors.Errorf("lambda function %q not found", t.TargetResource)
		}

		input, path, transformer := t.getInputConfigFromGenericInput()

		targets = []eventbridgetypes.Target{{
			Arn:              targetArn,
			RetryPolicy:      nil,
			Id:               aws.String(t.ID),
			RoleArn:          roleArn,
			Input:            input,
			InputPath:        path,
			InputTransformer: transformer,
		}}
	case EventBridgeTarget_Firehose:

		targetArn, err := awsw.NewFirehose(ctx, rctx.ProviderName).DeliveryStreamArnForIdentifier(ctx, t.TargetResource)
		if err != nil {
			return errors.Wrapf(err, "errors when resolving firehose delivery stream arn for eventbridge target %v", t.ID)
		}
		if targetArn == nil {
			return errors.Errorf("firehose delivery stream %q not found", t.TargetResource)
		}

		input, path, transformer := t.getInputConfigFromGenericInput()

		targets = []eventbridgetypes.Target{{
			Arn:              targetArn,
			RetryPolicy:      nil,
			Id:               aws.String(t.ID),
			RoleArn:          roleArn,
			Input:            input,
			InputPath:        path,
			InputTransformer: transformer,
		}}
	}

	// put targets
	ebrClient := client.EventBridge(ctx, rctx.ProviderName)
	_, err = ebrClient.PutTargets(ctx, &eventbridge.PutTargetsInput{
		EventBusName: aws.String(t.EventBusName),
		Rule:         aws.String(t.RuleName),
		Targets:      targets,
	})

	if err != nil {
		return errors.Wrapf(err, "error adding target %v to rule %v", t.ID, t.RuleName)
	}

	log.WithFields(log.Fields{
		"ID":   t.Identifier(),
		"Rule": t.RuleName,
	}).Info(color.Green("eventbridge target created"))

	return nil
}

// Destroy removes the eventbridge target
func (t EventBridgeTarget) Destroy(ctx context.Context, rctx Context) error {

	ebrClient := client.EventBridge(ctx, rctx.ProviderName)

	_, err := ebrClient.RemoveTargets(ctx, &eventbridge.RemoveTargetsInput{
		EventBusName: aws.String(t.EventBusName),
		Rule:         aws.String(t.RuleName),
		Ids:          []string{t.ID},
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting target %v for rule %v", t.Identifier(), t.RuleName)
	}

	log.WithFields(log.Fields{
		"ID":   t.Identifier(),
		"Rule": t.RuleName,
	}).Info(color.Red("eventbridge target deleted"))

	return nil
}

// getEcsTaskParameters returns the cluster arn (to be used as eventbridge target arn, the ecs
// task parameters or error)
func (t EventBridgeTarget) getEcsTaskParameters(ctx context.Context, rctx Context) (*string, *eventbridgetypes.EcsParameters, error) {
	if *t.TargetType == EventBridgeTarget_EcsRunTask && t.ECSTask != nil {

		//cluster arn
		arn, err := awsw.NewECS(ctx, rctx.ProviderName).EcsClusterArnForName(ctx, t.TargetResource)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error fetching cluster arn for name %v", t.TargetResource)
		}

		//taskDef
		taskDef, err := taskDefByName(ctx, rctx, *t.ECSTask.TaskDefName)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error fetching cluster arn for name %v", t.TargetResource)
		}

		//subnets
		subnets, err := awsw.NewEC2(ctx, rctx.ProviderName).SubnetIdsByNames(ctx, t.ECSTask.Subnets)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error fetching subnets for target %v", t.TargetResource)
		}

		//security groups
		securityGroups, err := awsw.NewEC2(ctx, rctx.ProviderName).SecurityGroupIdsByNames(ctx, nil, t.ECSTask.SecurityGroups)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error fetching security groups for target %v", t.TargetResource)
		}

		//launchType or CAP
		var launchType eventbridgetypes.LaunchType
		var capacityProviderStrategy []eventbridgetypes.CapacityProviderStrategyItem
		if *t.ECSTask.LaunchType == CAP {
			capacityProviderStrategy = make([]eventbridgetypes.CapacityProviderStrategyItem, len(t.ECSTask.CapacityProviders))
			for n, cp := range t.ECSTask.CapacityProviders {
				capacityProviderStrategy[n] = eventbridgetypes.CapacityProviderStrategyItem{
					Base:             cp.Base,
					Weight:           cp.Weight,
					CapacityProvider: aws.String(cp.CapacityProviderName),
				}
			}
		} else {
			launchType = eventbridgetypes.LaunchType(*t.ECSTask.LaunchType)
		}

		var platformVersion *string
		if *t.ECSTask.LaunchType == Fargate {
			platformVersion = t.ECSTask.PlatformVersion
		}

		//awsvpc network configuration
		var networkConfig *eventbridgetypes.NetworkConfiguration
		if taskDef.NetworkMode == ecstypes.NetworkModeAwsvpc {
			networkConfig = &eventbridgetypes.NetworkConfiguration{
				AwsvpcConfiguration: &eventbridgetypes.AwsVpcConfiguration{
					AssignPublicIp: eventbridgetypes.AssignPublicIp(t.ECSTask.AssignPublicIP),
					Subnets:        subnets,
					SecurityGroups: securityGroups,
				},
			}
		}

		//placement strategy
		// only to be used if the task is not using FARGATE
		//TODO: currently we force it  placement strategy to binpack(memory) & AZ-spread
		var placementStrategy []eventbridgetypes.PlacementStrategy
		usesFragate, err := t.usesFargate(ctx, rctx)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "error checking if task uses fargate or fargate-based capacity provider strategy")
		}
		if !usesFragate {
			placementStrategy = []eventbridgetypes.PlacementStrategy{
				{
					Field: aws.String("attribute:ecs.availability-zone"),
					Type:  eventbridgetypes.PlacementStrategyTypeSpread,
				},
				{
					Field: aws.String("memory"),
					Type:  eventbridgetypes.PlacementStrategyTypeBinpack,
				},
			}
		}

		//tags
		var tags []eventbridgetypes.Tag
		for k, v := range t.Tags {
			tags = append(tags, eventbridgetypes.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			})
		}

		taskDefArn := *taskDef.TaskDefinitionArn
		// strip the revision if not specify by TaskDefName
		if !strings.Contains(*t.ECSTask.TaskDefName, ":") {
			i := strings.LastIndex(*taskDef.TaskDefinitionArn, ":")
			taskDefArn = (*taskDef.TaskDefinitionArn)[:i]
		}

		return arn, &eventbridgetypes.EcsParameters{
			LaunchType:               launchType,
			PlatformVersion:          platformVersion,
			CapacityProviderStrategy: capacityProviderStrategy,
			PlacementStrategy:        placementStrategy,
			TaskCount:                t.ECSTask.TaskCount,
			TaskDefinitionArn:        &taskDefArn,
			NetworkConfiguration:     networkConfig,
			PropagateTags:            eventbridgetypes.PropagateTagsTaskDefinition,
			Tags:                     tags,
		}, nil
	}

	return nil, nil, nil
}

// getApiDestinationParameters returns the eventbridge_apidestination arn (to be used as eventbridge
// target arn, the httptarget parameters or error)
func (t EventBridgeTarget) getApiDestinationParameters(ctx context.Context, rctx Context) (*string, *eventbridgetypes.HttpParameters, error) {

	var httpParms = &eventbridgetypes.HttpParameters{}

	if *t.TargetType == EventBridgeTarget_ApiDestination {
		// arn for api destination
		arn, err := awsw.NewEventBridge(ctx, rctx.ProviderName).ApiDestinationArnForName(ctx, t.TargetResource)
		if err != nil {
			return nil, nil, err
		}

		if arn == nil {
			return nil, nil, errors.Errorf("cannot lookup api destination %v ", t.TargetResource)
		}

		if t.APIDestination != nil {
			if t.APIDestination.HeaderParameters != nil {
				httpParms.HeaderParameters = t.APIDestination.HeaderParameters
			}

			if t.APIDestination.QueryStringParameters != nil {
				httpParms.QueryStringParameters = t.APIDestination.QueryStringParameters
			}

			if t.APIDestination.PathParameters != nil {
				httpParms.PathParameterValues = t.APIDestination.PathParameters
			}
		}

		return arn, httpParms, nil
	}
	return nil, nil, nil
}

// getInputConfigFromGenericInput returns the input, input Path & input transforme parameters, or nils
// for the supplied GenericInput
func (t EventBridgeTarget) getInputConfigFromGenericInput() (input *string, path *string, transformer *eventbridgetypes.InputTransformer) {

	if t.GenericInput != nil {
		input = t.GenericInput.Value
		path = t.GenericInput.JsonPath
		if t.GenericInput.Template != nil {
			transformer = &eventbridgetypes.InputTransformer{
				InputTemplate: t.GenericInput.Template,
				InputPathsMap: t.GenericInput.PathsMap,
			}
		}
	}

	return input, path, transformer
}

// usesFargate returns true if the service is to be run with Fargate launch type OR a capacity provider strategy based
// on fargate (either explicitly or implicitly)
func (t EventBridgeTarget) usesFargate(ctx context.Context, rctx Context) (bool, error) {

	switch *t.ECSTask.LaunchType {
	case Fargate:
		return true, nil
	case EC2:
		return false, nil
	}

	//if explicit CAP strategy is supplied, check if it's FARGATE based...
	if len(t.ECSTask.CapacityProviders) > 0 {
		for _, cps := range t.ECSTask.CapacityProviders {
			if cps.CapacityProviderName == CAP_Fargate || cps.CapacityProviderName == CAP_FargateSpot {
				return true, nil
			}
		}
	} else {
		//if no explicity CAP strategy is supplied, check default capacity provider strategy of the cluster...
		ecsClient := client.ECS(ctx, rctx.ProviderName)
		out, err := ecsClient.DescribeClusters(ctx, &ecs.DescribeClustersInput{
			Clusters: []string{t.TargetResource},
		})

		if err != nil {
			return false, errors.Wrapf(err, "error fetching cluter details for %v", t.TargetResource)
		}

		for _, cluster := range out.Clusters {
			if *cluster.ClusterName == t.TargetResource {
				defCapStrategy := cluster.DefaultCapacityProviderStrategy
				for _, cp := range defCapStrategy {
					if *cp.CapacityProvider == CAP_Fargate || *cp.CapacityProvider == CAP_FargateSpot {
						return true, nil
					}
				}
			}
		}
	}

	return false, nil
}

// compares 2 eventbridge capacity providers
func diffEventbridgeCapacityProviderStrategies(left []eventbridgetypes.CapacityProviderStrategyItem,
	right []ECSCapacityProviderStrategy) bool {

	if len(left) != len(right) {
		return true
	}
	rMap := make(map[string]ECSCapacityProviderStrategy)
	for _, rVal := range right {
		rMap[rVal.CapacityProviderName] = rVal
	}
	for _, lVal := range left {
		rVal, ok := rMap[*lVal.CapacityProvider]
		if !ok {
			return true
		}
		if *lVal.CapacityProvider != rVal.CapacityProviderName ||
			lVal.Base != rVal.Base || lVal.Weight != rVal.Weight {
			return true
		}
		delete(rMap, *lVal.CapacityProvider)
	}
	return false
}
