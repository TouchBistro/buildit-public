package resource

import (
	"context"
	"fmt"
	"strings"

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
	TargetTracking  = "target-tracking"
	StepScaling     = "step-scaling"
	ScheduledAction = "scheduled"
)

const (
	AutoScalingMetricCPU          = "cpu"
	AutoScalingMetricMemory       = "memory"
	AutoScalingMetricRequestCount = "request-count"
	AutoScalingMetricCustom       = "custom"
)

// ApplicationAutoScaling defines the autoscaling policies to apply
type ApplicationAutoScaling struct {
	ResourceId        string                          `yaml:"-"`
	ScalableDimension string                          `yaml:"-"`
	ServiceNamespace  string                          `yaml:"-"`
	Suspended         bool                            `yaml:"suspend"`     //suspend all scaling (dynamic scaling in/out & scheduled action scaling)
	MinCapacity       int32                           `yaml:"minCapacity"` //minimum capacity limit for scale-ins
	MaxCapacity       int32                           `yaml:"maxCapacity"` //maximum capacity limit for scale-outs
	Policies          []*ApplicationAutoScalingPolicy `yaml:"policies"`
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (a *ApplicationAutoScaling) Normalize(ctx context.Context) {
	for _, policy := range a.Policies {

		policy.ResourceId = a.ResourceId
		policy.ScalableDimension = a.ScalableDimension
		policy.ServiceNamespace = a.ServiceNamespace

		if policy.Type == StepScaling {
			adjType := strings.ToLower(policy.StepAdjustmentType)
			switch adjType {
			case "changeincapacity":
				policy.StepAdjustmentType = string(aastypes.AdjustmentTypeChangeInCapacity)
			case "percentchangeincapacity":
				policy.StepAdjustmentType = string(aastypes.AdjustmentTypePercentChangeInCapacity)
			case "exactcapacity":
				policy.StepAdjustmentType = string(aastypes.AdjustmentTypeExactCapacity)
			}

			metricAggr := strings.ToLower(policy.StepMetricAggregation)
			switch metricAggr {
			case "average":
				policy.StepMetricAggregation = string(aastypes.MetricAggregationTypeAverage)
			case "minimum":
				policy.StepMetricAggregation = string(aastypes.MetricAggregationTypeMinimum)
			case "maximum":
				policy.StepMetricAggregation = string(aastypes.MetricAggregationTypeMaximum)
			}
		}
	}
}

// validate runs a ligher validation func on 3 key scaling parameters, resourceId, scalableDiminesion & serviceNamespace
func (a ApplicationAutoScaling) validate() error {

	var errMessages []string
	if err := validateStringFieldNotEmpty(a.ResourceId, "resource id"); err != nil {
		errMessages = append(errMessages, err.Error())
	}

	if err := validateStringFieldNotEmpty(a.ScalableDimension, "scalable dimension"); err != nil {
		errMessages = append(errMessages, err.Error())
	}

	if err := validateStringFieldNotEmpty(a.ServiceNamespace, "serivce namespace"); err != nil {
		errMessages = append(errMessages, err.Error())
	}

	if errMessages != nil {
		return &ValidationError{
			ResourceIdentifier: a.ResourceId,
			ResourceType:       "Application Autoscaling",
			Messages:           errMessages,
		}
	}

	return nil
}

// Validate check if autoscaling + scheduled actions have a valid configuration
func (a ApplicationAutoScaling) Validate(ctx context.Context) error {
	var errMessages []string

	err := a.validate()
	if err != nil {
		errMessages = append(errMessages, err.(*ValidationError).Messages...)
	}

	//autoscaling target-tracking with custom metric
	for _, policy := range a.Policies {
		switch policy.Type {
		case TargetTracking:
			//target-tracking
			if policy.TargetMetricName == AutoScalingMetricCustom {
				if policy.TargetCustomMetric == nil {
					errMessages = append(errMessages, fmt.Sprintf("`targetCustomMetric` not provided for %v where `targetMetricName` is custom", policy.Name))
				} else {
					switch aastypes.MetricStatistic(policy.TargetCustomMetric.Statistic) {
					case aastypes.MetricStatisticAverage, aastypes.MetricStatisticSum,
						aastypes.MetricStatisticMinimum, aastypes.MetricStatisticMaximum:
					default:
						errMessages = append(errMessages, fmt.Sprintf("invalid custom metric statistic specified for %v, allowed values are Average, Sum, Minimum or Maximum (case-sensitive)", policy.Name))
					}
				}
			}
		case StepScaling: //step-scaling
			switch aastypes.AdjustmentType(policy.StepAdjustmentType) {
			case aastypes.AdjustmentTypeChangeInCapacity, aastypes.AdjustmentTypeExactCapacity, aastypes.AdjustmentTypePercentChangeInCapacity:
			default:
				errMessages = append(errMessages, fmt.Sprintf("invalid step scaling policy adjustment type specified %v, allowed values are %v, %v and %v", policy.StepAdjustmentType,
					aastypes.AdjustmentTypeChangeInCapacity, aastypes.AdjustmentTypeExactCapacity, aastypes.AdjustmentTypePercentChangeInCapacity))

			}
		}
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: a.ResourceId,
		ResourceType:       "Application Autoscaling",
		Messages:           errMessages,
	}
}

// Apply not implemented & not called directly for now...
func (a ApplicationAutoScaling) Apply(ctx context.Context) error {
	panic("unimplemented")
}

// Delete scalable target & all associated policies & scheduled actions
func (a ApplicationAutoScaling) Delete(ctx context.Context, rctx Context) error {

	// validate key scaling parameters
	if err := a.validate(); err != nil {
		return err
	}
	// this would de-registers, as well as delete all scaling policies & scheduled actions for
	// the specified scalable resource, dimension & namespace
	return a.deRegisterScalableTarget(ctx, rctx)

}

type ApplicationAutoScalingDiff struct {
	BaseResourceDiff

	diff                 bool
	maxCapacityDiff      bool
	minCapacityDiff      bool
	suspendedDiff        bool
	scalableTargetAdd    bool
	policiesToAdd        []*ApplicationAutoScalingPolicy // add these policies
	scalableTargetDelete bool
	policiesToDelete     map[string]interface{} // delete these policies by name ScalingPolicy or ScheduledActions
}

// Compares this autoscaling configuration with existing confiuration & policies &
// returns a set of diffs or a non-nil error if
func (a *ApplicationAutoScaling) Compare(ctx context.Context, rctx Context) (ResourceDiff, error) {

	if err := a.validate(); err != nil {
		return nil, err
	}

	// check if this resourceId is a scalable target
	existing, err := a.fetchExistingScalableTarget(ctx, rctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error checking if resource %v is a scalable target", a.ResourceId)
	}

	// prefetch all policies that may exists for this resource/dimension/namespace..
	existingPoliciesMap, err := a.fetchAllExistingPolicies(ctx, rctx)
	if err != nil {
		return nil, err
	}

	autoscalingNotDefined := len(a.Policies) == 0
	autoscalingDoesNotExist := existing == nil && len(existingPoliciesMap) == 0

	// if no polcies defined in this ApplicaitonAutoScaling object
	if autoscalingNotDefined {

		// if not a scalable target, & no policies exists for this resource either...
		if autoscalingDoesNotExist {
			// nothing defined, nothing exists, no diff...
			return nil, nil
		}

		// if scalable target or policies exists, we need to delete all existing policies + scalable target
		// create a new diff object to indicate deletion of all existing scaling policies & scalable target for
		// this resource
		aasDiffs := &ApplicationAutoScalingDiff{
			diff:                 true,
			scalableTargetDelete: true,
			policiesToDelete:     existingPoliciesMap,
		}
		aasDiffs.Messages = append(aasDiffs.Messages, "scaling policies exist, all of them will be deleted")
		return aasDiffs, nil
	}

	// if scaling is defined, but existing policies dont exists, we add
	// add scaling target & all new policies
	if len(a.Policies) > 0 && autoscalingDoesNotExist {
		// create a new diff object to indicate registration of the autoscaling target for this resource
		// and addition of all the defined autoscaling policies & actions
		aasDiffs := &ApplicationAutoScalingDiff{
			diff:              true,
			scalableTargetAdd: true,
			policiesToAdd:     a.Policies,
		}
		aasDiffs.Messages = append(aasDiffs.Messages, fmt.Sprintf("no scaling policies exist; %v new policies to be added", len(a.Policies)))
		return aasDiffs, nil
	}

	// when both old & new policies exist, we will do a diff..

	aasDiffs := &ApplicationAutoScalingDiff{}
	awsTarget := existing

	if a.MaxCapacity != util.CoalesceInt32(awsTarget.MaxCapacity, 0) {
		aasDiffs.diff = true
		aasDiffs.maxCapacityDiff = true
		aasDiffs.Messages = append(aasDiffs.Messages, "scaling max capacity is different")
	}

	if a.MinCapacity != util.CoalesceInt32(awsTarget.MinCapacity, 0) {
		aasDiffs.diff = true
		aasDiffs.minCapacityDiff = true
		aasDiffs.Messages = append(aasDiffs.Messages, "scaling min capacity is different")
	}

	// this config & aws target have different suspended states
	if a.Suspended != isSuspended(awsTarget.SuspendedState) {
		aasDiffs.diff = true
		aasDiffs.suspendedDiff = true
		aasDiffs.Messages = append(aasDiffs.Messages, "scaling suspended state is different")
	}

	for _, policy := range a.Policies {

		if existingPolicy, ok := existingPoliciesMap[policy.Name]; !ok {
			//missing needs to be added
			aasDiffs.diff = true
			aasDiffs.policiesToAdd = append(aasDiffs.policiesToAdd, policy) // add this policy
			aasDiffs.Messages = append(aasDiffs.Messages, fmt.Sprintf("policy %v to be added ", policy.Name))
		} else {
			polDiff, err := policy.Compare(ctx, rctx, existingPolicy)
			if err != nil {
				return nil, err
			}

			if polDiff != nil { // diff found, the add this defined policy, remove the existing policy
				aasDiffs.diff = true
				aasDiffs.policiesToAdd = append(aasDiffs.policiesToAdd, policy)
				if aasDiffs.policiesToDelete == nil {
					aasDiffs.policiesToDelete = make(map[string]interface{})
				}
				aasDiffs.policiesToDelete[policy.Name] = existingPolicy
				aasDiffs.Messages = append(aasDiffs.Messages, fmt.Sprintf("existing policy %v will be replaced with new", policy.Name))
			}
			delete(existingPoliciesMap, policy.Name)
		}
	}

	// all remaining existing policies to be deleted...
	for k, v := range existingPoliciesMap {
		aasDiffs.diff = true
		if aasDiffs.policiesToDelete == nil {
			aasDiffs.policiesToDelete = make(map[string]interface{})
		}
		aasDiffs.policiesToDelete[k] = v
		aasDiffs.Messages = append(aasDiffs.Messages, fmt.Sprintf("existing policiy %v will be deleted", k))
	}

	if aasDiffs.diff {
		return aasDiffs, nil
	}

	return nil, nil
}

// IsZeroValue performs a composite check & returns a true if a represents a zero-value for ApplicationAutoScaling
func (a ApplicationAutoScaling) IsZeroValue() bool {
	return a.MinCapacity == 0 && a.MaxCapacity == 0 && len(a.Policies) == 0
}

// apply creates the application autoscaling scalable target & scaling policies & scheduled actions
func (a ApplicationAutoScaling) apply(ctx context.Context, rctx Context) error {

	// validate key scaling parameters
	if err := a.validate(); err != nil {
		return err
	}

	// add or update scalable targets...
	err := a.registerScalableTarget(ctx, rctx)
	if err != nil {
		return err
	}

	// create all policies
	for _, policy := range a.Policies {
		err := policy.Apply(ctx, rctx)
		if err != nil {
			return errors.Wrapf(err, "error creating scaling policy or scheduled action %v", policy.Name)
		}
	}
	return nil
}

// applyDiffs create/updates scalable targets, scaling policies & scheduled actions
func (a ApplicationAutoScaling) applyDiffs(ctx context.Context, rctx Context, diffs ResourceDiff) error {

	if err := a.validate(); err != nil {
		return err
	}

	if diffs == nil {
		return nil
	}

	if aasDiffs, ok := diffs.(*ApplicationAutoScalingDiff); !ok {
		return errors.Errorf("invalid diff supplied")
	} else {

		if aasDiffs.maxCapacityDiff || aasDiffs.minCapacityDiff || aasDiffs.suspendedDiff || aasDiffs.scalableTargetAdd {
			//update or create the scalable target
			if err := a.registerScalableTarget(ctx, rctx); err != nil {
				return err
			}
		}

		if aasDiffs.scalableTargetDelete {
			//remove scalable target & all associated policies/actions
			if err := a.deRegisterScalableTarget(ctx, rctx); err != nil {
				return err
			}

			return nil // no need to add/delete policies, once scalable target has been deleted
		}

		if len(aasDiffs.policiesToDelete) > 0 {
			for n, policy := range aasDiffs.policiesToDelete {

				t := TargetTracking
				switch policy := policy.(type) {
				case aastypes.ScalingPolicy:
					if policy.PolicyType == aastypes.PolicyTypeStepScaling {
						t = StepScaling
					}
				case aastypes.ScheduledAction:
					t = ScheduledAction
				}

				p := ApplicationAutoScalingPolicy{
					Name:              n,
					Type:              t,
					ResourceId:        a.ResourceId,
					ScalableDimension: a.ScalableDimension,
					ServiceNamespace:  a.ServiceNamespace,
				}
				if err := p.Delete(ctx, rctx); err != nil {
					return err
				}
			}
		}

		if len(aasDiffs.policiesToAdd) > 0 {
			// add all policies the are not already applied or are different
			for _, p := range aasDiffs.policiesToAdd {
				if err := p.Apply(ctx, rctx); err != nil {
					return err
				}
			}
		}

	}

	return nil
}

// registerScalableTarget registers and/or updates scalable target configuration
func (a ApplicationAutoScaling) registerScalableTarget(ctx context.Context, rctx Context) error {

	// Set service as an auto-scaling target
	aasClient := client.ApplicationAutoScaling(ctx, rctx.ProviderName)

	_, err := aasClient.RegisterScalableTarget(ctx, &aas.RegisterScalableTargetInput{
		MaxCapacity:       aws.Int32(a.MaxCapacity),
		MinCapacity:       aws.Int32(a.MinCapacity),
		ResourceId:        aws.String(a.ResourceId),
		ScalableDimension: aastypes.ScalableDimension(a.ScalableDimension),
		ServiceNamespace:  aastypes.ServiceNamespace(a.ServiceNamespace),
		SuspendedState: &aastypes.SuspendedState{
			DynamicScalingInSuspended:  aws.Bool(a.Suspended),
			DynamicScalingOutSuspended: aws.Bool(a.Suspended),
			ScheduledScalingSuspended:  aws.Bool(a.Suspended),
		},
	})

	if err != nil {
		return errors.Wrap(err, "error registering service as autoscaling target")
	}

	log.WithFields(log.Fields{
		"ResourceId": a.ResourceId,
	}).Info(color.Green("registered scalable target"))
	return nil
}

// deRegisterScalableTarget de registers this as a scalable target for autoscaling
func (a ApplicationAutoScaling) deRegisterScalableTarget(ctx context.Context, rctx Context) error {

	aasClient := client.ApplicationAutoScaling(ctx, rctx.ProviderName)

	outrc, err := aasClient.DescribeScalableTargets(ctx, &aas.DescribeScalableTargetsInput{
		ResourceIds:       []string{a.ResourceId},
		ScalableDimension: aastypes.ScalableDimension(a.ScalableDimension), //aastypes.ScalableDimensionECSServiceDesiredCount,
		ServiceNamespace:  aastypes.ServiceNamespace(a.ServiceNamespace),   //aastypes.ServiceNamespaceEcs,
	})

	if err != nil {
		return errors.Wrapf(err, "error checking is service %v is a scalable target", a.ResourceId)
	}

	if len(outrc.ScalableTargets) != 0 {
		_, err = aasClient.DeregisterScalableTarget(ctx, &aas.DeregisterScalableTargetInput{
			ResourceId:        aws.String(a.ResourceId),
			ScalableDimension: aastypes.ScalableDimension(a.ScalableDimension), //aastypes.ScalableDimensionECSServiceDesiredCount,
			ServiceNamespace:  aastypes.ServiceNamespace(a.ServiceNamespace),
		})

		if err != nil {
			return errors.Wrapf(err, "error deregistering scalable target %v", a.ResourceId)
		}
	}

	log.WithFields(log.Fields{
		"ResourceId": a.ResourceId,
	}).Info(color.Red("deregistered scalable target"))

	return nil
}

// fetchExistingScalableTarget finds the scalable target object for the supplied resource Id & parameters
func (a ApplicationAutoScaling) fetchExistingScalableTarget(ctx context.Context, rctx Context) (*aastypes.ScalableTarget, error) {

	if err := a.validate(); err != nil {
		return nil, err
	}

	aasClient := client.ApplicationAutoScaling(ctx, rctx.ProviderName)
	outrc, err := aasClient.DescribeScalableTargets(ctx, &aas.DescribeScalableTargetsInput{
		ResourceIds:       []string{a.ResourceId},
		ScalableDimension: aastypes.ScalableDimension(a.ScalableDimension),
		ServiceNamespace:  aastypes.ServiceNamespace(a.ServiceNamespace),
	})

	if err != nil {
		return nil, errors.Wrapf(err, "error checking if resource %v is a scalable target", a.ResourceId)
	}

	if len(outrc.ScalableTargets) == 0 {
		return nil, nil
	}

	for _, target := range outrc.ScalableTargets {
		if *target.ResourceId == a.ResourceId && target.ScalableDimension == aastypes.ScalableDimension(a.ScalableDimension) &&
			target.ServiceNamespace == aastypes.ServiceNamespace(a.ServiceNamespace) {
			return &target, nil
		}
	}

	return nil, nil
}

// fetchAllExistingPolicies fetches the scaling policy or scheduled action if it exists, or a non-nil error
func (a *ApplicationAutoScaling) fetchAllExistingPolicies(ctx context.Context, rctx Context) (map[string]interface{}, error) {

	if err := validateStringFieldNotEmpty(a.ResourceId, "resource id"); err != nil {
		return nil, err
	}

	if err := validateStringFieldNotEmpty(a.ScalableDimension, "scalable dimension"); err != nil {
		return nil, err
	}

	if err := validateStringFieldNotEmpty(a.ServiceNamespace, "serivce namespace"); err != nil {
		return nil, err
	}

	pmap := make(map[string]interface{})
	aasClient := client.ApplicationAutoScaling(ctx, rctx.ProviderName)

	// fetch scaling policies
	outSp, err := aasClient.DescribeScalingPolicies(ctx, &aas.DescribeScalingPoliciesInput{
		ResourceId:        aws.String(a.ResourceId),
		ServiceNamespace:  aastypes.ServiceNamespace(a.ServiceNamespace),
		ScalableDimension: aastypes.ScalableDimension(a.ScalableDimension),
	})

	if err != nil {
		return nil, fmt.Errorf("error fetching autoscaling policies fpr resource Id %v", a.ResourceId)
	}

	for _, sp := range outSp.ScalingPolicies {
		pmap[*sp.PolicyName] = sp //aastypes.ScalingPolicy
	}

	// fetch scheduled actions
	outSa, err := aasClient.DescribeScheduledActions(ctx, &aas.DescribeScheduledActionsInput{
		ResourceId:        aws.String(a.ResourceId),
		ServiceNamespace:  aastypes.ServiceNamespace(a.ServiceNamespace),
		ScalableDimension: aastypes.ScalableDimension(a.ScalableDimension),
	})

	if err != nil {
		return nil, fmt.Errorf("error fetching scheduled policies fpr resource Id %v", a.ResourceId)
	}

	for _, sa := range outSa.ScheduledActions {
		pmap[*sa.ScheduledActionName] = sa //aastypes.ScheduledAction
	}

	return pmap, nil
}

// isSuspended returns true if the supplied suspended state represents a suspended
func isSuspended(suspendedState *aastypes.SuspendedState) bool {
	return suspendedState != nil && (*suspendedState.DynamicScalingInSuspended &&
		*suspendedState.DynamicScalingOutSuspended &&
		*suspendedState.ScheduledScalingSuspended)
}
