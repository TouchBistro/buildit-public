package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	applicationautoscalingtypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

const (
	AlarmActionEc2          = "ec2"
	AlarmActionSns          = "sns"
	AlarmActionAutoscaling  = "autoscaling"
	AlarmActionSsm          = "ssm"
	AlarmActionSsmIncidents = "ssm-incidents"
)

// CWMetricAlarm represents a CloudWatch Metric Alarm
// The following attributes/features are not support at the momemt:
// - Percentils or ExtendedStatistics
// - Alarms based on a MetricsDataQuery
// - Anomaly Detection Alarms
// Refence: put ECS autoscaling https://docs.aws.amazon.com/autoscaling/application/userguide/create-step-scaling-policy-cli.html
type CWMetricAlarm struct {
	BaseResource `yaml:",inline"`
	Name                    string            `yaml:"-"`
	Description             string            `yaml:"description"`
	MetricName              string            `yaml:"metricName"`              // name for the metric associated
	Namespace               string            `yaml:"metricNamespace"`         // namespace for the metric associated
	Statistic               string            `yaml:"statistic"`               // statistics used Average, Sum, Minimum, Maximum; percetiles not supported)
	Period                  int32             `yaml:"period"`                  // evaluation time, 10, 30 or multiples of 60
	EvaluationPeriods       int32             `yaml:"evaluationPeriods"`       // number of periods over with data is compared to the threshold
	Threshold               float64           `yaml:"threshold"`               // threshold value for static alarms
	ComparisonOperator      string            `yaml:"comparisonOperator"`      // allowed: LT | LE | GT | GE
	DatapointsToAlarm       int32             `yaml:"datapointsToAlarm"`       // datapoints to breach for alarm
	Dimension               map[string]string `yaml:"dimensions"`              // the metrics dimensions
	ActionsEnabled          *bool             `yaml:"actionsEnabled"`          // are action enabled or disabled
	AlarmActions            map[string]string `yaml:"alarmActions"`            // actions to take when state is IN ALARM
	OKActions               map[string]string `yaml:"okActions"`               // actions to take when state is OK
	InsufficientDataActions map[string]string `yaml:"insufficientDataActions"` // actions to take when there is insufficient data for the metric
	TreatMissingData        string            `yaml:"treatMissingData"`        // allowed: breaching | notBreaching | ignore | missing
	Unit                    string            `yaml:"unit"`                    // metrics unit or percent
	Tags                    map[string]string `yaml:"tags"`                    // Resource level tags for the Cloudwatch metric alarm
	GlobalTags              map[string]string `yaml:"-"`
	DependsOn               []Key             `yaml:"dependsOn"`
}

// Key returns the unique key for the resource for this buildit context
func (c CWMetricAlarm) Key() Key {
	return NewKey(c.Context.ProviderName, c.Identifier())
}

// Identifier returns name of the cloudwatch log group
func (c CWMetricAlarm) Identifier() string {
	return c.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (c *CWMetricAlarm) Normalize(ctx context.Context) {

	if c.Statistic == "" {
		c.Statistic = string(cloudwatchtypes.StatisticSum) // default is SUM
	}

	if c.ActionsEnabled == nil {
		c.ActionsEnabled = aws.Bool(true) // default is true
	}

	//uppercase the comparator
	c.ComparisonOperator = strings.ToUpper(c.ComparisonOperator)

	//capitalize first letter
	switch strings.ToLower(c.Statistic) {
	case strings.ToLower(string(cloudwatchtypes.StatisticAverage)):
		c.Statistic = string(cloudwatchtypes.StatisticAverage)

	case strings.ToLower(string(cloudwatchtypes.StatisticMaximum)):
		c.Statistic = string(cloudwatchtypes.StatisticMaximum)

	case strings.ToLower(string(cloudwatchtypes.StatisticMinimum)):
		c.Statistic = string(cloudwatchtypes.StatisticMinimum)

	case strings.ToLower(string(cloudwatchtypes.StatisticSum)):
		c.Statistic = string(cloudwatchtypes.StatisticSum)

	case strings.ToLower(string(cloudwatchtypes.StatisticSampleCount)):
		c.Statistic = string(cloudwatchtypes.StatisticSampleCount)
	}

	// tags
	if c.Tags == nil {
		c.Tags = make(map[string]string)
	}
	// merge global tags into resource level tags
	ResourceTags(c.Tags).Merge(c.GlobalTags)

}

// Validate checks that the input provided is correct
func (c CWMetricAlarm) Validate(ctx context.Context) error {

	var errMessages []string

	switch strings.ToUpper(c.ComparisonOperator) {
	case "LT", "LE", "GT", "GE":
	default:
		msg := "invalid value for comparison operator, can be one of LE, LT, GE, GT"
		errMessages = append(errMessages, msg)
	}

	// statistic
	switch cloudwatchtypes.Statistic(c.Statistic) {
	case cloudwatchtypes.StatisticSum,
		cloudwatchtypes.StatisticMaximum,
		cloudwatchtypes.StatisticMinimum,
		cloudwatchtypes.StatisticAverage:
	default:
		msg := "invalid value for statistic, can be one of Average, Minimum, Maximum, Sum"
		errMessages = append(errMessages, msg)
	}

	if c.DatapointsToAlarm > c.EvaluationPeriods {
		msg := "invalid value for datapointsToAlarm, must be less than or equal to evaluationPeriods"
		errMessages = append(errMessages, msg)
	}

	// treat missing data
	switch c.TreatMissingData {
	case "breaching", "notBreaching", "ignore", "missing":
	default:
		msg := fmt.Sprint("invalid value for treat missing data, can be one of breaching", "notBreaching", "ignore", "missing")
		errMessages = append(errMessages, msg)
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: c.Identifier(),
		ResourceType:       "CloudWatch Alarm",
		Messages:           errMessages,
	}
}

// Apply creates a new cloudwatch log group and any log metric filters
func (c CWMetricAlarm) Apply(ctx context.Context) error {
	log.Debugf("creating cloudwatch metric alarm %v", c.Identifier())

	diffs, err := c.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", c.Identifier()).Info("cloudwatch metric alarm already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = c.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update cloudwatch metric alarm %v", c.Identifier())
		}
		return nil
	}
	return c.apply(ctx)
}

// Destroy removes the cloudwatch metric alarm
func (c CWMetricAlarm) Destroy(ctx context.Context) error {
	log.Debugf("deleting cloudwatch alarm %v", c.Identifier())
	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding existing cloudwatch alarm %v", c.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": c.Name,
		}).Info("cloudwatch metric alarm does not exist, nothing to destroy, skipping")
		return nil
	}

	cwClient := client.CloudWatch(ctx, c.Context.ProviderName)
	_, err = cwClient.DeleteAlarms(ctx, &cloudwatch.DeleteAlarmsInput{
		AlarmNames: []string{c.Name},
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting cloudwatch metric alarm %v", c.Name)
	}

	log.WithFields(log.Fields{
		"Name": c.Name,
	}).Info(color.Red("cloudwatch metric alarm deleted"))
	return nil
}

// CWMetricAlarmDiff represents diffs for the CWMetric alarm.
// since MetricAlarms are always upserts, all we need to capture
// if there is any diff found, so we don't need an additional fields
type CWMetricAlarmDiff struct {
	BaseResourceDiff
}

// Compare fetches the existing object from AWS & compares it with this definition,
// and returns a `ReesourceDiff` type with the differences,
func (c CWMetricAlarm) Compare(ctx context.Context) (ResourceDiff, error) {
	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", c.Identifier())
	}

	diffs := &CWMetricAlarmDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "cloudwatch log group does not exist")
		return diffs, nil
	}

	diffs.Resource = existing
	log.Debugf("updating cloudwatch log group for %v", c.Identifier())
	found := false

	// description
	if c.Description != util.CoalesceComparable(existing.AlarmDescription, "") {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm description is not the same")
	}
	// metric
	if c.MetricName != util.CoalesceComparable(existing.MetricName, "") {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm metric name is not the same")
	}

	// namespace
	if c.Namespace != util.CoalesceComparable(existing.Namespace, "") {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm metric namespace is not the same")
	}

	// statistics
	if c.Statistic != string(existing.Statistic) {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm statistic is not the same")
	}

	// period
	if c.Period != util.CoalesceComparable(existing.Period, 0) {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm metric period value is not the same")
	}

	// evaluation periods
	if c.EvaluationPeriods != util.CoalesceComparable(existing.EvaluationPeriods, 0) {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm metric evaluation periods value is not the same")
	}

	// threshold
	if c.Threshold != util.CoalesceComparable(existing.Threshold, 0) {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm threshold value is not the same")
	}

	// comparison operator
	comparisonOper := c.toCloudwatchComparisonOperator()
	if comparisonOper != existing.ComparisonOperator {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm comparison operation is not the same")
	}

	// datapoints to alarm
	if c.DatapointsToAlarm != util.CoalesceComparable(existing.DatapointsToAlarm, 0) {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm datapoints to alarm value is not the same ")
	}

	// dimension
	dim, err := c.alarmDimensiontoValue(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error determining metric dimensions for alarm %v and metric specified %v/%v", c.Name, c.Namespace, c.MetricName)
	}
	if len(dim) != len(existing.Dimensions) {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm dimensions are not the same")
	} else {
		for _, d := range existing.Dimensions {
			if v, ok := dim[*d.Name]; !ok {
				found = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("alarm dimension %v value will be removed", *d.Name))
			} else {
				if v != *d.Value {
					found = true
					diffs.Messages = append(diffs.Messages, "alarm dimension %v value is not the same")
				}
			}
		}
	}

	// alarm actions
	alarmActions, okActions, insufficientDataActions, err := c.toAlarmActions(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error converting alarm action supplied to a value arn")
	}

	if util.DiffStringSlices(alarmActions, existing.AlarmActions) {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm actions are not the same")
	}

	if util.DiffStringSlices(okActions, existing.OKActions) {
		found = true
		diffs.Messages = append(diffs.Messages, "ok actions are not the same")
	}

	if util.DiffStringSlices(insufficientDataActions, existing.InsufficientDataActions) {
		found = true
		diffs.Messages = append(diffs.Messages, "insufficient data actions are not the same")
	}

	if c.TreatMissingData != util.CoalesceComparable(existing.TreatMissingData, "") {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm treat missing data value is not the same")
	}

	if c.Unit != string(existing.Unit) {
		found = true
		diffs.Messages = append(diffs.Messages, "alarm unit value is not the same")
	}

	if !found {
		return nil, nil
	}

	return diffs, nil
}

// apply creates a new cloudwatch log group and any log metric filters
func (c CWMetricAlarm) apply(ctx context.Context) error {

	log.Debugf("creating cloudwatch alarm %v", c.Name)
	comparisonOper := c.toCloudwatchComparisonOperator()

	//TODO: metric dimension
	var dimensions []cloudwatchtypes.Dimension
	dim, err := c.alarmDimensiontoValue(ctx)
	if err != nil {
		return errors.Wrapf(err, "error determining metric dimensions for alarm %v and metric specified %v %v", c.Name, c.Namespace, c.MetricName)
	}
	for key, val := range dim {
		dimensions = append(dimensions, cloudwatchtypes.Dimension{
			Name:  aws.String(key),
			Value: aws.String(val),
		})
	}

	// tags
	var tags []cloudwatchtypes.Tag
	for k, v := range c.Tags {
		tags = append(tags, cloudwatchtypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	// alarm actions
	alarmActions, okActions, insufficientDataActions, err := c.toAlarmActions(ctx)
	if err != nil {
		return errors.Wrapf(err, "error converting alarm action supplied to a value arn")
	}

	cwClient := client.CloudWatch(ctx, c.Context.ProviderName)
	_, err = cwClient.PutMetricAlarm(ctx, &cloudwatch.PutMetricAlarmInput{
		AlarmName:               aws.String(c.Name),
		AlarmDescription:        aws.String(c.Description),
		ComparisonOperator:      comparisonOper,
		ActionsEnabled:          c.ActionsEnabled,
		Period:                  aws.Int32(c.Period),
		EvaluationPeriods:       aws.Int32(c.EvaluationPeriods),
		Threshold:               aws.Float64(c.Threshold),
		DatapointsToAlarm:       aws.Int32(c.DatapointsToAlarm),
		MetricName:              aws.String(c.MetricName),
		Namespace:               aws.String(c.Namespace),
		Statistic:               cloudwatchtypes.Statistic(c.Statistic),
		Dimensions:              dimensions,
		AlarmActions:            alarmActions,
		OKActions:               okActions,
		InsufficientDataActions: insufficientDataActions,
		TreatMissingData:        aws.String(c.TreatMissingData),
		Tags:                    tags,
	})

	if err != nil {
		return errors.Wrapf(err, "error creating cloudwatch metric alarm %v", c.Name)
	}

	log.WithFields(log.Fields{
		"Name": c.Name,
	}).Info(color.Green("cloudwatch metric alarm upserted"))

	return nil
}

// applyDiffs applies the changes to the resource from the diffs
func (c CWMetricAlarm) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	return c.apply(ctx)
}

// fetchExisting returns the existing cloudwatch alarm details if found
func (c CWMetricAlarm) fetchExisting(ctx context.Context) (*cloudwatchtypes.MetricAlarm, error) {

	cwClient := client.CloudWatch(ctx, c.Context.ProviderName)

	descOut, err := cwClient.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{
		AlarmNames: []string{c.Identifier()},
		AlarmTypes: []cloudwatchtypes.AlarmType{cloudwatchtypes.AlarmTypeMetricAlarm}, //only metric alarms supported
	})

	if err != nil {
		return nil, errors.Wrapf(err, "error describing cloudwatch alarm %v", c.Identifier())
	}

	if len(descOut.MetricAlarms) == 0 {
		return nil, nil
	}

	return &descOut.MetricAlarms[0], nil
}

// alarmDimensiontoValue receives a metric namespace, metric name and dimension key/value & returns the accepted value for that
// alarm dimension based on the metric namespace & dimension key e.g namespace: AWS/ApplicationELB & metric RequestCountPerTarget can
// have a dimension key/value of TargetGroup/<target-group-name>; the return value would be the last 2 segments of the target group ARN
//
// example ref: https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-cloudwatch-metrics.html
//
// since there are 100s of metrics and a bunch of valid dimension comibnations, we will implement them here on a need basisc
// for now the following  metric & dimensions are implemented.
//
// Namespace              Metric         Dimensions
// AWS/ApplicationELB     *              LoadBalancer, TargetGroup
func (c CWMetricAlarm) alarmDimensiontoValue(ctx context.Context) (map[string]string, error) {

	returnDimensionMap := make(map[string]string)

	if len(c.Dimension) == 0 {
		return returnDimensionMap, nil
	}

	switch c.Namespace {

	case "AWS/ApplicationELB":
		//all metrics under AWS/ApplicationELB are supported
		for key, value := range c.Dimension {
			var newVal string
			var err error
			switch key {
			case "LoadBalancer":
				//fine LB specified by the value & return dimension name for lb
				newVal, err = loadBalancerToDimensionValue(ctx, c.Context.ProviderName, value)
				if err != nil {
					return nil, errors.Wrapf(err, "loadbalancer name %v cannot be converted to a dimension value", value)
				}
			case "TargetGroup":
				newVal, err = targetGroupToDimensionValue(ctx, c.Context.ProviderName, value)
				if err != nil {
					return nil, errors.Wrapf(err, "target group name %v cannot be converted to a dimension value", value)
				}
			case "AvailabilityZone": //TODO
				return nil, errors.Errorf("invalid dimension specified for metric %v/%v", c.Namespace, c.MetricName)
			default:
				return nil, errors.Errorf("invalid dimension specified for metric %v/%v", c.Namespace, c.MetricName)
			}
			returnDimensionMap[key] = newVal
		}

	//Other supported metric namespaces with dimensions to be added here in future
	default:
		return nil, errors.Errorf("dimensions for metric namespace %v is not supported", c.Namespace)
	}

	return returnDimensionMap, nil
}

// targetGroupToDimensionValue finds the metric dimension name for a target group for the
// supplied target group name, else an error
func targetGroupToDimensionValue(ctx context.Context, providerName string, name string) (string, error) {

	tg, err := targetGroupFromName(ctx, providerName, name)
	if err != nil {
		return "", errors.Wrap(err, "error finding target group")
	}

	arn := *tg.TargetGroupArn
	return arn[strings.Index(arn, "targetgroup"):], nil
}

// toCloudwatchComparisonOperator converts ths supplied string comparison operation to cloudwatch types
// comparison operation
func (c CWMetricAlarm) toCloudwatchComparisonOperator() cloudwatchtypes.ComparisonOperator {
	var comparisonOper cloudwatchtypes.ComparisonOperator
	switch c.ComparisonOperator {
	case "LE":
		comparisonOper = cloudwatchtypes.ComparisonOperatorLessThanOrEqualToThreshold
	case "LT":
		comparisonOper = cloudwatchtypes.ComparisonOperatorLessThanThreshold
	case "GE":
		comparisonOper = cloudwatchtypes.ComparisonOperatorGreaterThanOrEqualToThreshold
	case "GT":
		comparisonOper = cloudwatchtypes.ComparisonOperatorGreaterThanThreshold
	}
	return comparisonOper
}

// toAlarmActions converrts the supplied actions to cloudwatch action arns []strings
func (c CWMetricAlarm) toAlarmActions(ctx context.Context) ([]string, []string, []string, error) {
	// alarm actions
	var alarmActions []string
	for key, value := range c.AlarmActions {
		arn, err := alarmActiontoArn(ctx, c.Context.ProviderName, key, value)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "error converting alarm action key-value (%v,%v) to a value arn", key, value)
		}
		alarmActions = append(alarmActions, arn)
	}

	// ok actions
	var okActions []string
	for key, value := range c.OKActions {
		arn, err := alarmActiontoArn(ctx, c.Context.ProviderName, key, value)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "error converting alarm action key-value (%v,%v) to a value arn", key, value)
		}
		okActions = append(okActions, arn)
	}

	// insufficient data actions
	var insufficientDataActions []string
	for key, value := range c.InsufficientDataActions {
		arn, err := alarmActiontoArn(ctx, c.Context.ProviderName, key, value)
		if err != nil {
			return nil, nil, nil, errors.Wrapf(err, "error converting alarm action key-value (%v,%v) to a value arn", key, value)
		}
		insufficientDataActions = append(insufficientDataActions, arn)
	}
	return alarmActions, okActions, insufficientDataActions, nil
}

// alarmActiontoArn receives a key /value alarm action from the metric *Actions section
// and returns a valid Cloudwatch metric alarm action ARN based on the provided key.
// A cloudwatch alarm action can be of the following types,
// ec2:           key=recover -> returns arn:aws:automate:region:ec2:recove
// ec2:           key=reboot -> returns arn:aws:automate:region:ec2:reboot
// sns:           key <sns-topic-name> -> returns SNS topics arn e.g arn:aws:sns:region:account-id:sns-topic-name
// autoscaling:   <autoscaling-policy-name> -> returns autoscaling policy arn only ECS autoscaling targets supported
// ssm:           <opt_item> : TODO: Not supported today
// ssm-incidents: TODO: Not supported today
func alarmActiontoArn(ctx context.Context, providerName string, key, value string) (string, error) {

	switch key {
	case AlarmActionEc2: // "ec2"
		switch value {
		case "recover":
			return "arn:aws:automate:region:ec2:recover", nil
		case "reboot":
			return "arn:aws:automate:region:ec2:reboot", nil
		default:
			return "", errors.Errorf("invalid alarm key %v & value %v specified", key, value)
		}

	case AlarmActionSns: // "sns"
		if arn.IsARN(value) {
			return value, nil //for now we expect an valid arn
		}
		return "", errors.Errorf("an in-valid SNS topic arn supplied for alarm action key %v", key)

	case AlarmActionAutoscaling: // "autoscaling"
		aasClient := client.ApplicationAutoScaling(ctx, providerName)
		descOut, err := aasClient.DescribeScalingPolicies(ctx, &applicationautoscaling.DescribeScalingPoliciesInput{
			ServiceNamespace: applicationautoscalingtypes.ServiceNamespaceEcs,
			PolicyNames:      []string{value},
		})
		if err != nil {
			return "", errors.Wrapf(err, "error listing applicaiton autoscaling policy value speicifed for alarm action key %v, and value %v", key, value)
		}
		if len(descOut.ScalingPolicies) == 0 {
			return "", errors.Errorf("the autoscaling policy %v supplied for alarm action type %v, doesn't exist", value, key)
		}
		for _, policy := range descOut.ScalingPolicies {
			if value == *policy.PolicyName {
				return *policy.PolicyARN, nil
			}
		}
		return "", errors.Errorf("the autoscaling policy %v supplied for alarm action type %v, doesn't exist", value, key)

	case AlarmActionSsm, AlarmActionSsmIncidents:
		return "", errors.New("ssm and ssm-incident action types are not supported yet")

	default:
		return "", errors.Errorf("invalid alarm action key %v specified, allowed values are ec2, sns, autoscaling", key)
	}
}

// loadBalancerToDimensionValue find the metric dimension name for a load balancer for the
// supplied load balancer name, else an error
func loadBalancerToDimensionValue(ctx context.Context, providerName string, name string) (string, error) {
	elbClient := client.ELB(ctx, providerName)
	out, err := elbClient.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{
		Names: []string{name},
	})
	if err != nil {
		return "", errors.Wrapf(err, "error retreiving load balancer %v", name)
	}
	for _, lb := range out.LoadBalancers {
		if name == *lb.LoadBalancerName {
			arn := *lb.LoadBalancerArn
			return arn[strings.Index(arn, "app"):], nil
		}
	}
	return "", errors.Wrapf(err, "load balancer with name: %v not found", name)
}
