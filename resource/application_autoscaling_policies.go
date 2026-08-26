package resource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	aas "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	aastypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	schedulingTimeFormat string = "2006-01-02 15:04:05"
)

// ApplicationAutoScalingStepAdjustment defines a step adjustment for step-scaling policy
type ApplicationAutoScalingStepAdjustment struct {
	IntervalLowerBound *float64 `yaml:"lowerBound"`
	IntervalUpperBound *float64 `yaml:"upperBound"`
	Value              int32    `yaml:"value"`
}

// ApplicationAutoScalingCustomPolicyMetric defines the metric for the custom target-tracking policy
type ApplicationAutoScalingCustomPolicyMetric struct {
	Name       string            `yaml:"name"`       //cloudwatch metric name
	Namespace  string            `yaml:"namespace"`  //cloudwatch metric namespace
	Statistic  string            `yaml:"statistic"`  //Average, Sum, Minimum, Maximum
	Unit       *string           `yaml:"unit"`       //where the metric defines a unit, empty/nil when no unit specified
	Dimensions map[string]string `yaml:"dimensions"` //the dimensions for that metric
}

// ApplicationAutoScalingPolicy defines an autoscaling policy parameters
type ApplicationAutoScalingPolicy struct {
	ResourceId            string                                    `yaml:"-"`
	ScalableDimension     string                                    `yaml:"-"`
	ServiceNamespace      string                                    `yaml:"-"`
	Name                  string                                    `yaml:"policyName"`
	Type                  string                                    `yaml:"policyType"`           //target-tracking, step-scaling, scheduled
	CoolDownSeconds       int32                                     `yaml:"coolDown"`             //scaleIn, scaleOut thresholds for target and step scaling
	DisableScaleIn        bool                                      `yaml:"disableScaleIn"`       // disable scale in
	TargetMetricName      string                                    `yaml:"targetMetricName"`     // cpu, memory, request-count, custom
	TargetMetricResource  string                                    `yaml:"targetMetricResource"` // target group name when request-count metric is used
	TargetMetricValue     float64                                   `yaml:"targetMetricValue"`    // threshold for the target metric to scale
	TargetCustomMetric    *ApplicationAutoScalingCustomPolicyMetric `yaml:"targetCustomMetric"`   // when targetMetricName=custom; specify the custom metric
	ScheduleStartTime     string                                    `yaml:"scheduleStartTimeUTC"` // schedule start time for scheculed
	ScheduleEndTime       string                                    `yaml:"scheduleEndTimeUTC"`   // schedule end time for scheduled
	ScheduleCron          string                                    `yaml:"scheduleCronUTC"`      // schedule cron expression for repetition
	ScheduleMinCapacity   int32                                     `yaml:"scheduleMin"`          // min capacity to set during schedule
	ScheduleMaxCapacity   int32                                     `yaml:"scheduleMax"`          // max capacity to set during schedule
	StepMetricAggregation string                                    `yaml:"stepMetricAggregation"`
	StepAdjustmentType    string                                    `yaml:"stepAdjustmentType"`
	StepMinAdjustment     *int32                                    `yaml:"stepMinAdjustment"` // minimum adjustment magnitude, when adjustment type used in percentChangeInCapacity
	StepAdjustments       []ApplicationAutoScalingStepAdjustment    `yaml:"stepAdjustments"`
}

// AASPolicyDiff struct is a container for keeping ApplicationAutoScalingPolicy diffs
type AASPolicyDiff struct {
	BaseResourceDiff

	diff                       bool
	invalidPolicyTypeDiff      bool
	targetTrackingDetailsDiff  bool
	stepScalingDetailsDiff     bool
	scheduledActionDetailsDiff bool
}

// Compares this policy with the policy or scheduld action that exists in AWS & returns
// a diff or a non-nil error
func (p *ApplicationAutoScalingPolicy) Compare(ctx context.Context, rctx Context, existing interface{}) (ResourceDiff, error) {

	var diffs = &AASPolicyDiff{}

	switch p.Type {

	// TargetTracking Policy Compare
	case TargetTracking:
		if ttPolicy, ok := existing.(aastypes.ScalingPolicy); !ok {
			diffs.diff = true
			diffs.invalidPolicyTypeDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("existing policy %v type is different", p.Name))
		} else {
			if ttPolicy.PolicyType != aastypes.PolicyTypeTargetTrackingScaling {
				diffs.diff = true
				diffs.invalidPolicyTypeDiff = true
				diffs.Messages = append(diffs.Messages, "existing policy %v type is different")
			}
			if ttPolicy.TargetTrackingScalingPolicyConfiguration != nil {
				cfg := ttPolicy.TargetTrackingScalingPolicyConfiguration
				if cfg.DisableScaleIn != nil && *cfg.DisableScaleIn != p.DisableScaleIn ||
					cfg.ScaleInCooldown != nil && *cfg.ScaleInCooldown != p.CoolDownSeconds ||
					cfg.ScaleOutCooldown != nil && *cfg.ScaleOutCooldown != p.CoolDownSeconds ||
					cfg.TargetValue != nil && *cfg.TargetValue != p.TargetMetricValue {
					diffs.diff = true
					diffs.targetTrackingDetailsDiff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v configuration is different", p.Name))
				}
				//Custom Metric Target
				if p.TargetMetricName == AutoScalingMetricCustom {
					if ttPolicy.TargetTrackingScalingPolicyConfiguration.CustomizedMetricSpecification == nil {
						diffs.diff = true
						diffs.targetTrackingDetailsDiff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v, target-tracking existing custom metric configuration is different", p.Name))
					} else {
						customMetricSpec, err := p.toCustomizedMetricSpecifiction(ctx)
						if err != nil {
							return nil, err
						}
						awsCustomMetricSpec := ttPolicy.TargetTrackingScalingPolicyConfiguration.CustomizedMetricSpecification
						if util.Coalesce(customMetricSpec.MetricName, "") != util.Coalesce(awsCustomMetricSpec.MetricName, "") ||
							util.Coalesce(customMetricSpec.Namespace, "") != util.Coalesce(awsCustomMetricSpec.Namespace, "") ||
							util.Coalesce(customMetricSpec.Unit, "") != util.Coalesce(awsCustomMetricSpec.Unit, "") ||
							!metricDimensionEquals(customMetricSpec.Dimensions, awsCustomMetricSpec.Dimensions) ||
							string(customMetricSpec.Statistic) != string(awsCustomMetricSpec.Statistic) {
							diffs.diff = true
							diffs.targetTrackingDetailsDiff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v, target-tracking custom metric configuration is different", p.Name))
						}
					}
				} else {
					// Predefined Metric Target
					if ttPolicy.TargetTrackingScalingPolicyConfiguration.PredefinedMetricSpecification == nil {
						diffs.diff = true
						diffs.targetTrackingDetailsDiff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v, target-tracking prefdefined metric configuration is different", p.Name))
					} else {
						predefinedMetricSpec, err := p.toPredefinedMetricSpecifiction(ctx, rctx)
						if err != nil {
							return nil, err
						}
						awsPredefinedMetricSpec := ttPolicy.TargetTrackingScalingPolicyConfiguration.PredefinedMetricSpecification
						if string(predefinedMetricSpec.PredefinedMetricType) != string(awsPredefinedMetricSpec.PredefinedMetricType) ||
							util.Coalesce(predefinedMetricSpec.ResourceLabel, "") != util.Coalesce(awsPredefinedMetricSpec.ResourceLabel, "") {
							diffs.diff = true
							diffs.targetTrackingDetailsDiff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v, target-tracking predefined metric configuration is different", p.Name))
						}
					}
				}
			} else {
				// case when taregt tracking policy config is nil,
				diffs.diff = true
				diffs.invalidPolicyTypeDiff = true
				diffs.targetTrackingDetailsDiff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("existing policy %v type is different", p.Name))
			}
		}

		// if diffs founds
		if diffs.diff {
			return diffs, nil
		}

		return nil, nil

	case StepScaling:

		if ssPolicy, ok := existing.(aastypes.ScalingPolicy); !ok {
			diffs.diff = true
			diffs.invalidPolicyTypeDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("existing policy %v type is different", p.Name))
		} else {
			if ssPolicy.PolicyType != aastypes.PolicyTypeStepScaling {
				diffs.diff = true
				diffs.invalidPolicyTypeDiff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("existing policy %v type is different", p.Name))
			}
			config := ssPolicy.StepScalingPolicyConfiguration
			if config != nil {

				if p.StepAdjustmentType != string(config.AdjustmentType) {
					diffs.diff = true
					diffs.stepScalingDetailsDiff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v, step-scaling adjustment type definition is different", p.Name))
				}

				if p.CoolDownSeconds != *config.Cooldown {
					diffs.diff = true
					diffs.stepScalingDetailsDiff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v, step-scaling cooldown time seconds is different", p.Name))
				}

				if p.StepMetricAggregation != string(config.MetricAggregationType) {
					diffs.diff = true
					diffs.stepScalingDetailsDiff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v, step-scaling metric aggregate type is different", p.Name))
				}

				if util.CoalesceInt32(p.StepMinAdjustment, 0) != util.CoalesceInt32(config.MinAdjustmentMagnitude, 0) {
					diffs.diff = true
					diffs.stepScalingDetailsDiff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v, step-scaling min adjustment is different", p.Name))
				}

				if len(p.StepAdjustments) != len(config.StepAdjustments) {
					diffs.diff = true
					diffs.stepScalingDetailsDiff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v, step-scaling adjustments are different", p.Name))
				} else {
					for n, adj := range p.StepAdjustments {
						existing := config.StepAdjustments[n]
						if util.CoalesceFloat64(adj.IntervalLowerBound, 0) != util.CoalesceFloat64(existing.MetricIntervalLowerBound, 0) ||
							util.CoalesceFloat64(adj.IntervalUpperBound, 0) != util.CoalesceFloat64(existing.MetricIntervalUpperBound, 0) ||
							adj.Value != *existing.ScalingAdjustment {
							diffs.diff = true
							diffs.stepScalingDetailsDiff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("policy %v, step-scaling adjustment step # %v is different", p.Name, n))
						}
					}
				}

			} else {
				// case when taregt tracking policy config is nil,
				diffs.diff = true
				diffs.invalidPolicyTypeDiff = true
				diffs.stepScalingDetailsDiff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("existing policy %v type is different", p.Name))
			}
		}

		// if diffs founds
		if diffs.diff {
			return diffs, nil
		}

		return nil, nil

	case ScheduledAction:

		if schAction, ok := existing.(aastypes.ScheduledAction); !ok {
			diffs.diff = true
			diffs.invalidPolicyTypeDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("existing policy %v type is different", p.Name))
		} else {
			if p.ScheduleStartTime != schAction.StartTime.Format(schedulingTimeFormat) ||
				p.ScheduleEndTime != schAction.EndTime.Format(schedulingTimeFormat) ||
				p.ScheduleMaxCapacity != *schAction.ScalableTargetAction.MaxCapacity ||
				p.ScheduleMinCapacity != *schAction.ScalableTargetAction.MinCapacity ||
				p.ScheduleCron != *schAction.Schedule {
				diffs.diff = true
				diffs.scheduledActionDetailsDiff = true
				diffs.Resource = existing
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("scheduled action %v is different", p.Name))
			}
		}

		// if diffs founds
		if diffs.diff {
			return diffs, nil
		}

		return nil, nil
	}
	return nil, nil
}

// Apply provisions the auto-scaling or scheduled action policy for the supplied scaling target
func (p *ApplicationAutoScalingPolicy) Apply(ctx context.Context, rctx Context) error {

	if err := validateStringFieldNotEmpty(p.Name, "policy name"); err != nil {
		return err
	}
	if err := validateStringFieldNotEmpty(p.Type, "policy type"); err != nil {
		return err
	}
	if err := validateStringFieldNotEmpty(p.ResourceId, "resource id"); err != nil {
		return err
	}
	if err := validateStringFieldNotEmpty(p.ScalableDimension, "scalable dimension"); err != nil {
		return err
	}
	if err := validateStringFieldNotEmpty(p.ServiceNamespace, "serivce namespace"); err != nil {
		return err
	}

	var err error
	aasClient := client.ApplicationAutoScaling(ctx, rctx.ProviderName)

	switch p.Type {
	//target-tracking
	case TargetTracking:

		var predefinedMetricSpec *aastypes.PredefinedMetricSpecification
		var customMetricSpec *aastypes.CustomizedMetricSpecification

		//when target-tracking p.Type = custom
		if p.TargetMetricName == AutoScalingMetricCustom {

			customMetricSpec, err = p.toCustomizedMetricSpecifiction(ctx)
			if err != nil {
				return err
			}
		} else {

			predefinedMetricSpec, err = p.toPredefinedMetricSpecifiction(ctx, rctx)
			if err != nil {
				return err
			}
		}

		targetPolicyInput := &aas.PutScalingPolicyInput{
			PolicyName:        aws.String(p.Name),
			PolicyType:        aastypes.PolicyTypeTargetTrackingScaling,
			ResourceId:        aws.String(p.ResourceId),
			ScalableDimension: aastypes.ScalableDimension(p.ScalableDimension),
			ServiceNamespace:  aastypes.ServiceNamespace(p.ServiceNamespace),
			TargetTrackingScalingPolicyConfiguration: &aastypes.TargetTrackingScalingPolicyConfiguration{
				DisableScaleIn:                aws.Bool(p.DisableScaleIn), //allow scale-in
				ScaleInCooldown:               aws.Int32(p.CoolDownSeconds),
				ScaleOutCooldown:              aws.Int32(p.CoolDownSeconds),
				CustomizedMetricSpecification: customMetricSpec,
				PredefinedMetricSpecification: predefinedMetricSpec,
				TargetValue:                   aws.Float64(p.TargetMetricValue),
			},
		}
		_, err = aasClient.PutScalingPolicy(ctx, targetPolicyInput)

		if err != nil {
			return errors.Wrapf(err, "error attaching target-trackig autoscaling policy %v", p.Name)
		}

		log.WithFields(log.Fields{
			"Name": p.Name,
		}).Info(color.Green("target-tracking auto-scaling policy created"))

	// step-scaling
	case StepScaling:
		//minAdjustment magnitude only needed when PercentChangeInCapacity adjustment type is used
		var minAdjustmentMangnitude *int32
		if p.StepAdjustmentType == string(aastypes.AdjustmentTypePercentChangeInCapacity) {
			minAdjustmentMangnitude = p.StepMinAdjustment
		}

		// step adjustment
		var stepAdjustments []aastypes.StepAdjustment
		for _, adjustment := range p.StepAdjustments {
			stepAdjustments = append(stepAdjustments, aastypes.StepAdjustment{
				MetricIntervalLowerBound: adjustment.IntervalLowerBound,
				MetricIntervalUpperBound: adjustment.IntervalUpperBound,
				ScalingAdjustment:        aws.Int32(adjustment.Value),
			})
		}

		targetPolicy := &aas.PutScalingPolicyInput{
			PolicyName:        aws.String(p.Name),
			PolicyType:        aastypes.PolicyTypeStepScaling,
			ResourceId:        aws.String(p.ResourceId),
			ScalableDimension: aastypes.ScalableDimension(p.ScalableDimension),
			ServiceNamespace:  aastypes.ServiceNamespace(p.ServiceNamespace),
			StepScalingPolicyConfiguration: &aastypes.StepScalingPolicyConfiguration{
				AdjustmentType:         aastypes.AdjustmentType(p.StepAdjustmentType),           //ChangeInCapacity | PercentChangeInCapacity | ExactCapacity
				Cooldown:               aws.Int32(p.CoolDownSeconds),                            //3m
				MetricAggregationType:  aastypes.MetricAggregationType(p.StepMetricAggregation), //Average | Minimum | Maximum
				MinAdjustmentMagnitude: minAdjustmentMangnitude,                                 //for PercentChangeInCapacity
				StepAdjustments:        stepAdjustments,
			},
		}
		_, err := aasClient.PutScalingPolicy(ctx, targetPolicy)
		if err != nil {
			return errors.Wrapf(err, "error attaching autoscaling policy %v", p.Name)
		}
		log.WithFields(log.Fields{
			"Name": p.Name,
		}).Info(color.Green("step scaling auto-scaling policy created"))

	// scheduled
	case ScheduledAction:

		var startTime *time.Time
		var endTime *time.Time

		if p.ScheduleStartTime != "" {

			startTimeVal, err := time.Parse(schedulingTimeFormat, p.ScheduleStartTime)
			if err != nil {
				return errors.Wrapf(err, "invalid scheduled start time %v defined for %v ",
					p.ScheduleStartTime, p.Name)
			}
			startTime = &startTimeVal
		}
		if p.ScheduleEndTime != "" {

			endTimeVal, err := time.Parse(schedulingTimeFormat, p.ScheduleEndTime)
			if err != nil {
				return errors.Wrapf(err, "invalid scheduled end time %v defined for %v ",
					p.ScheduleEndTime, p.Name)
			}
			endTime = &endTimeVal
		}

		if p.ScheduleMinCapacity > p.ScheduleMaxCapacity {
			return errors.New(fmt.Sprintf(
				"scaling schedule min capacity %v cannot to greater than max %v",
				p.ScheduleMinCapacity, p.ScheduleMaxCapacity))
		}

		scaleAction := &aas.PutScheduledActionInput{
			ScheduledActionName: aws.String(p.Name),
			ResourceId:          aws.String(p.ResourceId),
			ScalableDimension:   aastypes.ScalableDimension(p.ScalableDimension),
			ServiceNamespace:    aastypes.ServiceNamespace(p.ServiceNamespace),
			Schedule:            aws.String(p.ScheduleCron),
			StartTime:           startTime,
			EndTime:             endTime,
			ScalableTargetAction: &aastypes.ScalableTargetAction{
				MaxCapacity: aws.Int32(p.ScheduleMaxCapacity),
				MinCapacity: aws.Int32(p.ScheduleMinCapacity),
			},
		}

		_, err := aasClient.PutScheduledAction(ctx, scaleAction)
		if err != nil {
			return errors.Wrapf(err, "error attaching scheduled autoscaling policy %v", p.Name)
		}

		log.WithFields(log.Fields{
			"Name": p.Name,
		}).Info(color.Green("scheduled action auto-scaling created"))
	}
	return nil
}

// Delete deletes the auto-scaling or scheduled action policy for the supplied scaling target
func (p *ApplicationAutoScalingPolicy) Delete(ctx context.Context, rctx Context) error {

	if err := validateStringFieldNotEmpty(p.Name, "policy name"); err != nil {
		return err
	}
	if err := validateStringFieldNotEmpty(p.Type, "policy type"); err != nil {
		return err
	}
	if err := validateStringFieldNotEmpty(p.ResourceId, "resource id"); err != nil {
		return err
	}
	if err := validateStringFieldNotEmpty(p.ScalableDimension, "scalable dimension"); err != nil {
		return err
	}
	if err := validateStringFieldNotEmpty(p.ServiceNamespace, "serivce namespace"); err != nil {
		return err
	}

	aasClient := client.ApplicationAutoScaling(ctx, rctx.ProviderName)

	switch p.Type {
	//target-tracking
	case TargetTracking, StepScaling:

		_, err := aasClient.DeleteScalingPolicy(ctx, &aas.DeleteScalingPolicyInput{
			PolicyName:        aws.String(p.Name),
			ResourceId:        aws.String(p.ResourceId),
			ScalableDimension: aastypes.ScalableDimension(p.ScalableDimension),
			ServiceNamespace:  aastypes.ServiceNamespace(p.ServiceNamespace),
		})

		if err != nil {
			return errors.Wrapf(err, "error deleting existing scaling policy %v for service", p.Name)
		}

		log.WithFields(log.Fields{
			"Name": p.Name,
		}).Info(color.Red(fmt.Sprintf("%v auto-scaling policy deleted", p.Type)))

	// scheduled
	case ScheduledAction:
		_, err := aasClient.DeleteScheduledAction(ctx, &aas.DeleteScheduledActionInput{
			ScheduledActionName: &p.Name,
			ResourceId:          aws.String(p.ResourceId),
			ScalableDimension:   aastypes.ScalableDimension(p.ScalableDimension),
			ServiceNamespace:    aastypes.ServiceNamespace(p.ServiceNamespace),
		})

		if err != nil {
			return errors.Wrapf(err, "error deleting existing scheduled action %v for service", p.Name)
		}

		log.WithFields(log.Fields{
			"Name": p.Name,
		}).Info(color.Red("scheduled action auto-scaling deleted"))

	}

	return nil
}

// toCustomizedMetricSpecifiction convert this
func (p ApplicationAutoScalingPolicy) toCustomizedMetricSpecifiction(ctx context.Context) (*aastypes.CustomizedMetricSpecification, error) {
	var dimensions []aastypes.MetricDimension
	for name, value := range p.TargetCustomMetric.Dimensions {
		dimensions = append(dimensions, aastypes.MetricDimension{
			Name:  aws.String(name),
			Value: aws.String(value),
		})
	}
	customMetricSpec := &aastypes.CustomizedMetricSpecification{
		MetricName: aws.String(p.TargetCustomMetric.Name),
		Namespace:  aws.String(p.TargetCustomMetric.Namespace),
		Statistic:  aastypes.MetricStatistic(p.TargetCustomMetric.Statistic),
		Unit:       p.TargetCustomMetric.Unit,
		Dimensions: dimensions,
	}
	return customMetricSpec, nil
}

func (p ApplicationAutoScalingPolicy) toPredefinedMetricSpecifiction(ctx context.Context, rctx Context) (*aastypes.PredefinedMetricSpecification, error) {
	var metricName aastypes.MetricType
	var resourceLabel *string
	var err error

	switch p.TargetMetricName {
	case AutoScalingMetricCPU:
		metricName = aastypes.MetricTypeECSServiceAverageCPUUtilization // "ECSServiceAverageCPUUtilization"
	case AutoScalingMetricMemory:
		metricName = aastypes.MetricTypeECSServiceAverageMemoryUtilization //"ECSServiceAverageMemoryUtilization"
	case AutoScalingMetricRequestCount:
		metricName = aastypes.MetricTypeALBRequestCountPerTarget //"ALBRequestCountPerTarget"
		//"app/<lb-name>/<lb-id>/targetgroup/<tg-name>/<tg-id>"
		resourceLabel, err = p.getTargetMetricResourceLabel(ctx, rctx.ProviderName, p.TargetMetricResource)
		if err != nil {
			return nil, errors.Wrap(err, "error while building resource label for ALBRequestCountPerTarget autoscaling policy")
		}
	default:
		return nil, errors.Errorf("invalid targetMetricName %v provided for policy %v", p.TargetMetricName, p.Name)
	}

	predefinedMetricSpec := &aastypes.PredefinedMetricSpecification{
		PredefinedMetricType: metricName,
		ResourceLabel:        resourceLabel,
	}
	return predefinedMetricSpec, nil
}

// ApplicationAutoScalingPolicy builds and returns the resource label for adding target-tracking autoscaling policy for LB target group
func (p ApplicationAutoScalingPolicy) getTargetMetricResourceLabel(ctx context.Context, providerName, targetGroupName string) (*string, error) {

	tg, err := targetGroupFromName(ctx, providerName, targetGroupName)
	if err != nil {
		return nil, errors.Wrap(err, "error finding target group")
	}

	if len(tg.LoadBalancerArns) == 0 {
		return nil, errors.Errorf("cannot add target-tracking for a target group %v not attached to a load balancer", targetGroupName)
	}

	// The resource label is constructed using the final portions of the load balancer & target group ARNs being tracked
	// Example LB & TG ARNs:
	// Load Balancer : arn:aws:elasticloadbalancing:us-east-1:1234567890:loadbalancer/app/service-lb/719239b02aed4daf
	// Target Group  : arn:aws:elasticloadbalancing:us-east-1:1234567890:targetgroup/service-tg/a827f4b0afbd6d72
	// For load balancer, the final portion is everthing including & after "app/"
	// For Target groups, the final portion is everything including & after "targetgroup/"
	// the format to return is:  "app/<lb-name>/<lb-id> + forward-slash + targetgroup/<tg-name>/<tg-id>"
	// returned resource label would be: app/service-lb/719239b02aed4da/targetgroup/service-tg/a827f4b0afbd6d72
	tgArn := *tg.TargetGroupArn
	lbArn := tg.LoadBalancerArns[0]
	tgSub := tgArn[strings.Index(tgArn, "targetgroup"):]
	lbSub := lbArn[strings.Index(lbArn, "app"):]
	label := fmt.Sprintf("%v/%v", lbSub, tgSub)
	return &label, nil
}

// metricDimensionEquals compares two []MetricDimension by converting them to
// []string & using util.StringSliceEquals utility method.
// TODO: this method can be improved by doing a MetricDiminsion comparison instead
func metricDimensionEquals(left, right []aastypes.MetricDimension) bool {

	var leftDimStrArray []string
	for _, v := range left {
		leftDimStrArray = append(leftDimStrArray, fmt.Sprintf("%v=%v", v.Name, v.Value))
	}

	var rightDimStrArray []string
	for _, v := range right {
		rightDimStrArray = append(rightDimStrArray, fmt.Sprintf("%v=%v", v.Name, v.Value))
	}

	return util.StringSliceEquals(leftDimStrArray, rightDimStrArray)
}

// validateStringFieldNotEmpty checks if the supplied fieldVal is not-empty, if so returns an error
func validateStringFieldNotEmpty(fieldValue, fieldName string) error {
	if len(fieldValue) == 0 || len(strings.TrimSpace(fieldValue)) == 0 {
		return fmt.Errorf("%v is empty & invalid", fieldName)
	}
	return nil
}
