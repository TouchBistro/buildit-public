package resource

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"

	"github.com/aws/aws-sdk-go-v2/aws"
	aastypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	log "github.com/sirupsen/logrus"
)

const (
	AssignPublicIpEnabled  = ecstypes.AssignPublicIpEnabled
	AssignPublicIpDisabled = ecstypes.AssignPublicIpDisabled
)

const (
	// EC2 Launch Type
	EC2 = "EC2"
	// Fargate Launch Type
	Fargate = "FARGATE"
	// CAP A capacity provider; This is "NOT" an ECS LaunchType but buildit uses CAP to indicate the capacity providers need to be used.
	CAP = "CAP"
)

const (
	CAP_Fargate     = "FARGATE"
	CAP_FargateSpot = "FARGATE_SPOT"
)

const (
	//Replica Service Type
	Replica string = "REPLICA"
	//Daemon Service type
	Daemon = "DAEMON"
)

// ECSCapacityProviderStrategy respresent a capacity provider strategy item
// to be provided, when a custom capacity provider strategy is used
type ECSCapacityProviderStrategy struct {
	Base                 int32  `yaml:"base"`
	Weight               int32  `yaml:"weight"`
	CapacityProviderName string `yaml:"name"`
}

// ECSServiceTargetGroupAssignment defines the target groups that will be used to register
// this service with a load balancer.
type ECSServiceTargetGroupAssignment struct {
	ContainerName   string  `yaml:"containerName"`
	ContainerPort   int32   `yaml:"containerPort"`
	TargetGroupName string  `yaml:"targetGroupName"`
	targetGroupArn  *string // used internally for update optimization..
}

// ECSServiceLoadBalancing defines load balancing targets for this service.
// Container name from the taskDef is required, since there can
// be more than 1 containers, and an existing target group must
// be specified.
// Attaching the target groups to an ALB happens when LB rules
// are defined.
type ECSServiceLoadBalancing struct {
	HealthcheckGracePeriod int32                             `yaml:"healthcheckGracePeriod"` //Time(s) when no LB healtcheck is performed
	TargetGroupAssignments []ECSServiceTargetGroupAssignment `yaml:"targetGroups"`
}

// ECSServiceDiscovery enabled CloudMap/Route53 based service discovery for the ECS Service
// the namespace must be created & the name nust not already exist, when creating a new service.
type ECSServiceDiscovery struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

// ECSDeploymentCircuitBreaker defines circuit breaker behavior for the service deployment
type ECSDeploymentCircuitBreaker struct {
	Enabled  bool `yaml:"enabled"`
	Rollback bool `yaml:"rollback"`
}

type ECSServiceCheckDeployment struct {
	Enabled              bool `yaml:"enabled"`
	TimeoutSeconds       int  `yaml:"timeoutSeconds"`
	FailedTasksThreshold int  `yaml:"failedTasksThreshold"`
}

// ECSService represents an ECS Service resource
// TODO
// DeploymentType    DeploymentType always RollingUpdate
// TaskPlacement     TaskPlacement always AZBalanced+Binpack
type ECSService struct {
	BaseResource `yaml:",inline"`
	Name                     string                        `yaml:"-"`
	LaunchType               *string                       `yaml:"launchType"`
	CapacityProviders        []ECSCapacityProviderStrategy `yaml:"capacityProviders"`
	TaskDefName              string                        `yaml:"taskDefName"`
	ClusterName              string                        `yaml:"clusterName"`
	ServiceType              *string                       `yaml:"serviceType"`       // Scheduling strategy, Replica, Dameon
	DesiredCount             *int32                        `yaml:"desiredCount"`      //Ignored when ServiceType=Daemon
	MinHealthyPercent        *int32                        `yaml:"minHealthyPercent"` //Deployment Min Healthy to keep when rolling
	MaxPercent               *int32                        `yaml:"maxPercent"`        //Deployment Max percent tasks
	DeploymentCircuitBreaker ECSDeploymentCircuitBreaker   `yaml:"deploymentCircuitBreaker"`
	AssignPublicIP           *string                       `yaml:"assignPublicIp"`     //Enabled | Disabled when awsvpc networking
	Subnets                  []string                      `yaml:"subnetNames"`        //Required for awsvpc tasks networking
	SecurityGroups           []string                      `yaml:"securityGroupNames"` //Required for awsvpc tasks networking
	LoadBalancing            ECSServiceLoadBalancing       `yaml:"loadBalancing"`
	ServiceDiscovery         ECSServiceDiscovery           `yaml:"serviceDiscovery"`
	ServiceAutoScaling       *ApplicationAutoScaling       `yaml:"autoScaling"`
	ForceNewDeployment       bool                          `yaml:"forceNewDeployment"` //If set, will always force an update,
	CheckDeployment          ECSServiceCheckDeployment     `yaml:"checkDeployment"`
	Tags                     map[string]string             `yaml:"tags"`
	GlobalTags               map[string]string             `yaml:"-"`
	DependsOn                []Key                         `yaml:"dependsOn"`
}

// Key returns the unique key for the resource for this buildit context
func (e ECSService) Key() Key {
	return NewKey(e.Context.ProviderName, e.Identifier())
}

// Identifier returns the unique identifier for this service
func (s ECSService) Identifier() string {
	return s.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (s *ECSService) Normalize(ctx context.Context) {

	// default service type to REPLICA
	if s.ServiceType == nil {
		s.ServiceType = aws.String(Replica)
	} else {
		s.ServiceType = aws.String(strings.ToUpper(*s.ServiceType))
	}

	// default launch type to CAP
	if s.LaunchType == nil {
		s.LaunchType = aws.String(CAP)
	}
	s.LaunchType = aws.String(strings.ToUpper(*s.LaunchType))

	if s.AssignPublicIP != nil {
		s.AssignPublicIP = aws.String(strings.ToUpper(*s.AssignPublicIP))
	} else {
		s.AssignPublicIP = aws.String(string(ecstypes.AssignPublicIpDisabled)) // DISABLED is the default
	}

	// deployment checking
	if s.CheckDeployment.TimeoutSeconds < 1 {
		s.CheckDeployment.TimeoutSeconds = 600 // default to 10min
	}
	if s.CheckDeployment.FailedTasksThreshold == 0 {
		s.CheckDeployment.FailedTasksThreshold = 3 //set to 3 if not set
	}

	// when replica, ensure desiredCount is defaulted to 0 when not supplied..
	if *s.ServiceType == Replica && s.DesiredCount == nil {
		s.DesiredCount = aws.Int32(0)
	}

	// Default health percent
	if s.MinHealthyPercent == nil {
		if *s.ServiceType == Replica {
			s.MinHealthyPercent = aws.Int32(100)
		} else {
			s.MinHealthyPercent = aws.Int32(0)
		}
	}
	if s.MaxPercent == nil {
		if *s.ServiceType == Replica {
			s.MaxPercent = aws.Int32(200)
		} else {
			s.MaxPercent = aws.Int32(100)
		}
	}
	// Merge globalTags to Target Group tags, if key is not already present
	// later we'll use s.Tags to add/update tags
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	ResourceTags(s.Tags).Merge(s.GlobalTags)

	// auto-scaling
	if s.ServiceAutoScaling == nil {
		s.ServiceAutoScaling = &ApplicationAutoScaling{}
	}

	if !s.ServiceAutoScaling.IsZeroValue() {
		// when autoscaling non-zero min/max capacity is supplied, we set the
		// desired count value to minCapacity if it's less than minCapacity or
		// maxCapacity if it's over maxCapacity
		if s.ServiceAutoScaling.MinCapacity != 0 && s.ServiceAutoScaling.MaxCapacity != 0 {
			if *s.ServiceType == Replica {
				dc := *s.DesiredCount
				if dc < s.ServiceAutoScaling.MinCapacity {
					s.DesiredCount = aws.Int32(s.ServiceAutoScaling.MinCapacity)
				} else if dc > s.ServiceAutoScaling.MaxCapacity {
					s.DesiredCount = aws.Int32(s.ServiceAutoScaling.MaxCapacity)
				}
			}
		}
	}

	s.ServiceAutoScaling.ResourceId = "service/" + s.ClusterName + "/" + s.Name
	s.ServiceAutoScaling.ScalableDimension = string(aastypes.ScalableDimensionECSServiceDesiredCount)
	s.ServiceAutoScaling.ServiceNamespace = string(aastypes.ServiceNamespaceEcs)
	s.ServiceAutoScaling.Normalize(ctx)

	// default container name and port
	for i, target := range s.LoadBalancing.TargetGroupAssignments {
		if target.ContainerName == "" {
			s.LoadBalancing.TargetGroupAssignments[i].ContainerName = "service"
		}
		if target.ContainerPort == 0 {
			s.LoadBalancing.TargetGroupAssignments[i].ContainerPort = 8080
		}
	}
}

// Validate checks that the ECS Service has a valid configuration.
// If the configuration is invalid an error will be returned
// that contains details on what was invalid.
func (s ECSService) Validate(ctx context.Context) error {

	var errMessages []string

	switch *s.LaunchType {
	case Fargate, EC2, CAP:
	default:
		//invalid launch type
		msg := fmt.Sprintf("invalid launch type %q, must be 'EC2', 'FARGATE' or 'CAP'", *s.LaunchType)
		errMessages = append(errMessages, msg)
	}

	// only one of launch type or capacity provider strategy must be provided.
	// If none is provided, the default capacity provider strategy for the ECS Cluster is used
	if *s.LaunchType != CAP && len(s.CapacityProviders) > 0 {
		msg := "capacity providers can only be specified when launch type is CAP"
		errMessages = append(errMessages, msg)
	}

	switch *s.ServiceType {
	case Replica, Daemon:
	//invalid scheduling strategy
	default:
		msg := fmt.Sprintf("invalid service type %q, must be 'REPLICA' or 'DAEMON'", *s.ServiceType)
		errMessages = append(errMessages, msg)
	}

	//desired count, autoscaling, capacity provider/strategy must be ommited when daemon
	if *s.ServiceType == Daemon {

		if *s.LaunchType == Fargate {
			errMessages = append(errMessages, "FARGATE launch type doesn't support 'DAEMON' service type")
		}

		//desired count supplied
		if s.DesiredCount != nil {
			errMessages = append(errMessages, "cannot specify a value for desired count when scheduling strategy is DAEMON")
		}

		//autoscaling
		if s.ServiceAutoScaling != nil {
			if s.ServiceAutoScaling.MaxCapacity != 0 || s.ServiceAutoScaling.MinCapacity != 0 ||
				len(s.ServiceAutoScaling.Policies) > 0 {
				errMessages = append(errMessages, "cannot specify a scaling policy when scheduling strategy is DAEMON")
			}
		}

		//launch type
		if *s.LaunchType == CAP {
			errMessages = append(errMessages, "cannot use capacity provider strategy when scheduling strategy is DAEMON, use EC2")
		}

		//capacity provider strategy
		if len(s.CapacityProviders) > 0 {
			errMessages = append(errMessages, "cannot specify capacity providers when scheduling strategy is DAEMON")
		}

		//deployment max percent
		if *s.MaxPercent > 100 {
			errMessages = append(errMessages, "cannot specify max percent value over 100 when scheduling strategy is DAEMON, use <= 100")
		}
	}

	if *s.MinHealthyPercent >= *s.MaxPercent {
		errMessages = append(errMessages, "cannot specify minimum healthy percent value greater or equal to max percent value")
	}

	if !s.DeploymentCircuitBreaker.Enabled && s.DeploymentCircuitBreaker.Rollback {
		errMessages = append(errMessages, "cannot enable deployment rollback if deployment circuit breaker is being disabled")
	}

	if (s.ServiceDiscovery.Name == "" && s.ServiceDiscovery.Namespace != "") ||
		(s.ServiceDiscovery.Name != "" && s.ServiceDiscovery.Namespace == "") {
		errMessages = append(errMessages, "both service discovery name and namespace must be provided")
	}

	if s.AssignPublicIP != nil {
		if *s.AssignPublicIP != string(AssignPublicIpEnabled) && *s.AssignPublicIP != string(AssignPublicIpDisabled) {
			msg := fmt.Sprintf("invalid value for assignPublicIp %q, must be 'ENABLED' or 'DISABLED'", *s.AssignPublicIP)
			errMessages = append(errMessages, msg)
		}
	}

	if len(s.LoadBalancing.TargetGroupAssignments) == 0 && s.LoadBalancing.HealthcheckGracePeriod > 0 {
		errMessages = append(errMessages, "health check grace period is only valid for services with load balancing")
	}

	//autoscaling target-tracking with custom metric
	if s.ServiceAutoScaling != nil && !s.ServiceAutoScaling.IsZeroValue() {
		if len(s.ServiceAutoScaling.Policies) > 0 {
			if *s.DesiredCount < s.ServiceAutoScaling.MinCapacity || *s.DesiredCount > s.ServiceAutoScaling.MaxCapacity {
				msg := fmt.Sprintf("when autoscaling is enabled, the value for service desired count(%v) must be between min (%v) & max (%v) autoscaling capacity",
					*s.DesiredCount, s.ServiceAutoScaling.MinCapacity, s.ServiceAutoScaling.MaxCapacity)
				errMessages = append(errMessages, msg)
			}
		}
		err := s.ServiceAutoScaling.Validate(ctx)
		if err != nil {
			errMessages = append(errMessages, err.(*ValidationError).Messages...)
		}
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: s.Identifier(),
		ResourceType:       "ECS Service",
		Messages:           errMessages,
	}
}

// Apply changes
func (s ECSService) Apply(ctx context.Context) error {
	log.Debugf("creating ecs service %v", s.Identifier())

	diffs, err := s.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		// no diffs found; also no force deployment requested..
		if !s.ForceNewDeployment {
			log.WithFields(log.Fields{
				"Name": s.Identifier(),
			}).Info("no updates required")
			return nil
		} else {
			// force new deployment is a special case for the ecs service resource
			// hanlded here since it wont show as a diff in the Compare(). This is
			// intentional to avoid Compare() returning non-nil diff just because of
			// force new deployment & causing other uses-cases of Compare() e.g. e2e
			// testing to fail validation of Apply()
			//
			// here we fake a dummy diff with just foreceDeploymentDff:true & let it
			// flow to the following

			diffs = &ECSServiceDiff{
				foreceDeploymentDiff: true,
			}

			existing, err := s.fetchExisting(ctx)
			if err != nil {
				return err
			}
			taskDef, err := taskDefByName(ctx, s.Context, s.TaskDefName)

			if err != nil {
				return err
			}

			diffs.(*ECSServiceDiff).Resource = existing
			diffs.(*ECSServiceDiff).servicetaskDef = taskDef
		}
	}

	// if diff found & existing resource exists
	if diffs != nil && diffs.AWSResource() != nil {
		log.WithField("Name", s.Identifier()).Info("ecs service already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = s.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update ecs service %v", s.Identifier())
		}
		return nil
	}

	return s.apply(ctx)
}

// Destroy the resource
func (s ECSService) Destroy(ctx context.Context) error {

	svc, err := s.fetchExisting(ctx)

	if err != nil {
		return errors.Wrapf(err, "error listing service %v", s.Name)
	}

	if svc == nil {
		log.WithFields(log.Fields{
			"Name": s.Name,
		}).Info("ECS Service does not exist, nothing to destroy")
		return nil
	}

	ecsClient := client.ECS(ctx, s.Context.ProviderName)
	_, err = ecsClient.DeleteService(ctx, &ecs.DeleteServiceInput{
		Cluster: aws.String(s.ClusterName),
		Service: aws.String(s.Name),
		Force:   aws.Bool(true), //forces deleting a service even is desiredSize > 0
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting service %v", s.Name)
	}

	err = waitUntilServiceInactive(ctx, s.Context.ProviderName, s.ClusterName, s.Name)
	if err != nil {
		return errors.Wrapf(err, "error checking service status for %s", s.Name)
	}

	log.WithFields(log.Fields{
		"Name": s.Name,
	}).Info(color.Red("ecs service destroyed"))

	return nil
}

// Wait waits for the deployment of the ECS service to be complete.
// It does this by continuously polling AWS until it sees that the desired number of
// new tasks are running and all old tasks are gone.
func (s ECSService) Wait(ctx context.Context) error {
	if !s.CheckDeployment.Enabled {
		log.Debugf("Skipping deployment check for service %s", s.Name)
		return nil
	}

	//TODO:
	log.Debugf("Launching deployment check for %v", s.Name)
	log.Debugf("Deployment Timeout (sec): %v", s.CheckDeployment.TimeoutSeconds)
	log.Debugf("Failed Task Threshold: %v", s.CheckDeployment.FailedTasksThreshold)

	start := time.Now()
	ecsClient := client.ECS(ctx, s.Context.ProviderName)

	// Lookup the task def so we can get the ARN. We will need this to see if the service has cutover to new tasks
	// with this ARN.

	taskDef, err := taskDefByName(ctx, s.Context, s.TaskDefName)
	if err != nil {
		return errors.Wrap(err, "failed to find task definition")
	}
	wantTaskDefARN := *taskDef.TaskDefinitionArn
	log.Debugf("Looking for taskdef ARN %s for service %s", color.Cyan(wantTaskDefARN), color.Cyan(s.Name))

	statsdClient := client.Statsd()
	// There should always be at least one container defined and the first container should always be the main service container.
	// Any sidecars should come after.
	// TODO: fix this, the assumption is weak...
	// TODO: need to review whats being sent to DD; can we get that info from the service tags?
	dockerTags := taskDef.ContainerDefinitions[0].DockerLabels
	err = statsdClient.SendEvent("deploys.draining", fmt.Sprintf("Checking for service drain on %s", s.Name), dockerTags)
	if err != nil {
		log.Warnf("Failed to send statsd event: %v", err)
	}

	// DISCUSS(@maintainer): Should this be configurable?
	checkInterval := 15 * time.Second
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.CheckDeployment.TimeoutSeconds)*time.Second)
	defer cancel()

	// We want to wait for all old tasks to have drained and to have the desired number of new tasks.
	// Then we can say that the deploy of the service has completed successfully.

	var deploymentID string
	reportedFailedTasks := make(map[string]struct{})
	for {
		// Wait for either the context to be done or the sleep duration.
		// This adds a wait period in between checks so we can give AWS a chance to finish the
		// deployment before checking again.
		if err := util.SleepWithContext(ctx, checkInterval); err != nil {
			return err
		}

		//fetch deployments for this service...
		describeServicesOutput, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  aws.String(s.ClusterName),
			Services: []string{s.Name},
		})
		if err != nil {
			return errors.Wrapf(err, "failed to get service in cluster: %s", s.ClusterName)
		}
		if len(describeServicesOutput.Failures) > 0 {
			return errors.Wrapf(err, "errors occurred while getting services:\n\t%s", formatFailures(describeServicesOutput.Failures))
		}
		if len(describeServicesOutput.Services) != 1 {
			return errors.Wrapf(err, "expected 1 service named %s, got %d", s.Name, len(describeServicesOutput.Services))
		}

		awsService := describeServicesOutput.Services[0]
		deploymentOk := false
		rollingBack := false
		var runningCount, desiredCount, failedCount int32
		var rolloutState ecstypes.DeploymentRolloutState

		for _, deployment := range awsService.Deployments {
			if *deployment.Status == "PRIMARY" {

				if deploymentID != "" {
					if deploymentID != *deployment.Id {
						elapsed := time.Since(start)
						log.Warnf(color.Red("Deployment of service %v (%v) failed after %v; PRIMARY deployment is now %v; This happens when the triggerred deployment rolls back, or a new deployment is triggered"), s.Name, deploymentID, elapsed, *deployment.Id)
						return errors.Errorf("Deployment %v failing for service: %v", deploymentID, s.Name)
					}
				}

				// fetch information about PRIMARY deployment. the PRIMARY deployment represents the one that was just
				// triggered with the update to this service. If deployment circuit breaker is configured with rollback,
				// we may see a new primary deployment in the event of a failure.
				deploymentID = *deployment.Id
				runningCount = deployment.RunningCount
				desiredCount = deployment.DesiredCount
				rolloutState = deployment.RolloutState
				failedCount = deployment.FailedTasks

				if rolloutState == ecstypes.DeploymentRolloutStateFailed {
					elapsed := time.Since(start)
					log.Errorf(color.Red("Deployment of service %v (%v) failed; ECS set deployment status to FAILED after %v"), s.Name, deploymentID, elapsed)
					log.Errorf(color.Red("Failure reason: %v"), util.Coalesce(deployment.RolloutStateReason, "not available"))
					return errors.Errorf("Deployment %v failing for service: %v", deploymentID, s.Name)
				}

				// TODO
				// since we have failed tasks, let's investigate some  of these tasks & their reason
				// to give an early feedback to deployer...

				// TODO need to deprecate buildit `checkDeployment` or redesign it to avoid confusion
				// ECS deployment circuit breaker has its own configuration to determine a failed rollout deployment
				// and based on the settings, it would either leave the service in failed state OR run a roll back
				// deployment if possible (there is a successful previous deploy)
				//
				// if number of failed tasks (tasks that fail to bootstrap and/or marked unhealthy by ECS) for this
				// deployment has reached a failure threshold, then we report error. Depending on the service's
				// bootstrap time & service/LB healthcheck settings, this may take longer for some services than
				// others. with this in mind, the checkDeployment.timeoutSeconds & checkDeployment.failedTasksThreshold
				// must be set accordingly for each service, the default is 600s & 3 respectively.
				if failedCount >= int32(s.CheckDeployment.FailedTasksThreshold) {
					elapsed := time.Since(start)
					log.Errorf(color.Red("Deployment of service %v (%v) failed; %v tasks failed after %v"), s.Name, deploymentID, failedCount, elapsed)
					return errors.Errorf("Deployment %v failing for service: %v", deploymentID, s.Name)
				}

				// it takes a few seconds for the deployment object to reflect the desired/running counts; so we need to
				// check the 'rollout state' as well, else this check would always pass, since
				// 'runningCount == desiredCount == 0' is the initial value, 'deploymentOk' will almost always be set to
				// true rightaway.'rolloutState' value is set by ECS which checks the draining status of the 'ACTIVE'
				// deployment as well. So it is set a bit AFTER the 'running' count has reached the 'desired' count value.
				if (runningCount == desiredCount) && (rolloutState == ecstypes.DeploymentRolloutStateCompleted) {
					if *deployment.TaskDefinition == wantTaskDefARN {
						deploymentOk = true
					} else {
						rollingBack = true
					}
					break
				}
			}
		}

		// if buildit detects a deployment in completed status, but a different taskDef arn or deployment ID
		// it should be considered a rollback (ecs deployment circuit breaker features)
		if rollingBack {
			elapsed := time.Since(start)
			log.Infof(color.Yellow("Deployment of service %v failed; ECS rolled back to last good known deployment in %v"), s.Name, elapsed)
			return errors.Errorf("Deployment %v failing for service: %v", deploymentID, s.Name)
		}

		if !deploymentOk {
			log.WithFields(log.Fields{
				"Desired": desiredCount,
				"Running": runningCount,
				"Failed":  failedCount,
				"Rollout": rolloutState,
			}).Infof(color.Yellow("Deployment of service %v is not yet complete"), s.Name)

			tasks, err := s.tasksForDeployment(ctx, deploymentID, ecstypes.DesiredStatusStopped) // stopped tasks
			if err != nil {
				log.Warnf("error retrieving task details for this deployment: %v", err.Error())
			}

			for _, task := range tasks {
				taskId := util.Coalesce(task.TaskArn, "")
				taskId = taskId[strings.LastIndex(taskId, "/")+1:]

				// if not reported for this deployment already...
				if _, ok := reportedFailedTasks[taskId]; !ok {
					log.Infof(color.Red("\t%v (%v) - %v"),
						taskId, util.Coalesce(task.LastStatus, ""), util.Coalesce(task.StoppedReason, ""))
				}
				reportedFailedTasks[taskId] = struct{}{} // reported already..
			}
			continue
		} else {
			log.WithFields(log.Fields{
				"Desired": desiredCount,
				"Running": runningCount,
				"Failed":  failedCount,
				"Rollout": rolloutState,
			}).Infof(color.Green("Deployment of service %v is complete"), s.Name)

			// process
			tasks, err := s.tasksForDeployment(ctx, deploymentID, ecstypes.DesiredStatusRunning) // running tasks
			if err != nil {
				log.Warnf("error retrieving task details for this deployment: %v", err.Error())
			}

			for _, task := range tasks {
				taskId := util.Coalesce(task.TaskArn, "")
				taskId = taskId[strings.LastIndex(taskId, "/")+1:]
				taskIp := "unknown"
				if len(task.Attachments) > 0 {
					for _, detail := range task.Attachments[0].Details {
						if util.Coalesce(detail.Name, "") == "privateIPv4Address" {
							taskIp = util.Coalesce(detail.Value, "")
						}
					}
					//}
				}
				log.Infof(color.Green("\t%v (%v) - %v"),
					taskId, util.Coalesce(task.LastStatus, ""), taskIp)
			}

			break
		}
	}

	elapsed := time.Since(start)
	log.Infof("ECS Service %s successfully deployed in %v", s.Name, elapsed)
	err = statsdClient.SendEvent("deploys.completed", fmt.Sprintf("Successfully deployed %s", s.Name), dockerTags)
	if err != nil {
		log.Warnf("Failed to send statsd event: %v", err)
	}
	return nil
}

// tasksForDeployment returns a list of tasks running for a deployment, else a non-nil error
// if there is an problem listing them
func (s ECSService) tasksForDeployment(ctx context.Context, deploymentId string, status ecstypes.DesiredStatus) ([]ecstypes.Task, error) {
	var token *string
	var tasks []ecstypes.Task
	client := client.ECS(ctx, s.Context.ProviderName)

	for {
		out, err := client.ListTasks(ctx, &ecs.ListTasksInput{
			Cluster:   aws.String(s.ClusterName),
			StartedBy: aws.String(deploymentId),
			// ServiceName: aws.String(s.Name),
			DesiredStatus: status,
			MaxResults:    aws.Int32(10),
			NextToken:     token,
		})

		if err != nil {
			return nil, err
		}

		log.Debugf("%v tasks returned for service: %v, deployment: %v",
			len(out.TaskArns), s.Name, deploymentId)
		if len(out.TaskArns) > 0 {
			out2, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
				Cluster: aws.String(s.ClusterName),
				Tasks:   out.TaskArns,
			})

			if err != nil {
				return nil, err
			}

			tasks = append(tasks, out2.Tasks...)
		}

		token = out.NextToken
		if token == nil {
			break
		}
	}
	return tasks, nil
}

type ECSServiceDiff struct {
	BaseResourceDiff

	serviceArn string

	diff                     bool
	schedulingStrategyDiff   bool
	desiredCountDiff         bool
	taskDefDiff              bool
	launchTypeFargateDiff    bool
	ec2ToCapDiff             bool
	capToEc2Diff             bool
	defaultCapToCapDiff      bool
	capToDefaultCapDiff      bool
	capToCapDiff             bool
	minOrMaxHealthyDiff      bool
	deployCircuitBreakerDiff bool
	loadBalancingDiff        bool
	lbHealthcheckDiff        bool
	serviceDiscoveryDiff     bool
	serviceDiscoveryArn      *string
	serviceNetworkingDiff    bool

	servicetaskDef        *ecstypes.TaskDefinition
	securityGroupIdsToAdd []string
	subnetIdsToAdd        []string

	foreceDeploymentDiff bool

	tagsDiff bool
	tagDiff  util.TagDiffResult

	autoscalingDiff  bool
	autoscalingDiffs *ApplicationAutoScalingDiff

	previousDeploymentFailed bool
}

// Compare fetches the corresponding ECS Service from AWS & if it exists, checks
// if this definition is equal to the AWS resource
func (s ECSService) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := s.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", s.Identifier())
	}

	diffs := &ECSServiceDiff{}
	if existing == nil {
		diffs.Messages = append(diffs.Messages, "ecs service does not exist")
		return diffs, nil
	}

	diffs.Resource = existing
	diffs.serviceArn = *existing.ServiceArn

	// comparing
	if string(existing.SchedulingStrategy) != *s.ServiceType {
		diffs.diff = true
		diffs.schedulingStrategyDiff = true
		diffs.Messages = append(diffs.Messages, "scheduling strategy is different")
	}

	// desiredCount
	if *s.ServiceType == Replica {
		// when service autoscaling is enabled for a service, a simple diff for desiredCount defined & currently set
		// can return incorrect values. The desiredCount value returned by AWS is the effective desiredCount at the moment
		// which is either the desiredCount value set by the service definition OR if applicaiton autoscaling or scheduled
		// scaling is active, will be between min & max capacity.

		if existing.DesiredCount != *s.DesiredCount &&
			(s.ServiceAutoScaling == nil || s.ServiceAutoScaling.IsZeroValue() || (s.ServiceAutoScaling != nil && (existing.DesiredCount < s.ServiceAutoScaling.MinCapacity || existing.DesiredCount > s.ServiceAutoScaling.MaxCapacity))) {
			diffs.diff = true
			diffs.desiredCountDiff = true
			diffs.Messages = append(diffs.Messages, "desired count is different")
		}
	}

	// task definition
	oldTaskDefName, oldTaskDefRev := taskDefNameRevisionFromArn(ctx, s.Context, *existing.TaskDefinition)
	newTaskDefName, newTaskDefRev := taskDefNameRevisionFromArn(ctx, s.Context, s.TaskDefName)
	if oldTaskDefName != newTaskDefName || oldTaskDefRev != newTaskDefRev {
		diffs.diff = true
		diffs.taskDefDiff = true
		diffs.Messages = append(diffs.Messages, "desired count is different")
	}

	// capacity provider strategy
	// var capDiff bool
	oldLaunchTypeWasFargate := existing.LaunchType == Fargate
	oldLaunchTypeWasEC2 := existing.LaunchType == EC2
	newLaunchTypeIsEC2 := *s.LaunchType == EC2
	oldLaunchTypeWasCAP := existing.LaunchType == ""
	newLaunchTypeIsCAP := *s.LaunchType == CAP

	if oldLaunchTypeWasFargate && *s.LaunchType != Fargate ||
		!oldLaunchTypeWasFargate && *s.LaunchType == Fargate {
		// switching to or away from FARGATE!
		diffs.diff = true
		diffs.launchTypeFargateDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("launch type changed from %v to %v", existing.LaunchType, s.LaunchType))
	} else if oldLaunchTypeWasEC2 && newLaunchTypeIsCAP {
		// switching from EC2 to CAP
		diffs.diff = true
		diffs.ec2ToCapDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("launch type changed from %v to %v", existing.LaunchType, s.LaunchType))
	} else if oldLaunchTypeWasCAP && newLaunchTypeIsEC2 {
		// switching from CAP to EC2
		diffs.diff = true
		diffs.capToEc2Diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("launch type changed from %v to %v", existing.LaunchType, s.LaunchType))
		// return errors.New("cannot update service from capacity provider strategy to EC2 launch type")
	} else if oldLaunchTypeWasCAP && newLaunchTypeIsCAP {

		oldCAPWasDefault := isDefaultCapacityProviderStrategy(ctx, s.Context, s.ClusterName, existing.CapacityProviderStrategy)
		// oldAndNewCAPsAreDifferent := diffCapacityProviderStrategies(old.CapacityProviderStrategy, new.CapacityProviders)
		newCAPIsDefault := len(s.CapacityProviders) == 0 || //greedy diff
			isSameAsDefaultCAPStrategy(ctx, s.Context, s.ClusterName, s.CapacityProviders)
		// Updating from default CAP provider to default CAP provider would result in no change
		if oldCAPWasDefault && !newCAPIsDefault {
			// switching from default CAP to non-default CAP
			diffs.diff = true
			diffs.defaultCapToCapDiff = true
			diffs.Messages = append(diffs.Messages, "launch type changed from default to non-default capacity provider")
		} else if !oldCAPWasDefault && newCAPIsDefault {
			// switching from non-default CAP to default CAP
			diffs.diff = true
			diffs.capToDefaultCapDiff = true
			diffs.Messages = append(diffs.Messages, "launch type changed from non-default to default capacity provider")
		} else if !oldCAPWasDefault && !newCAPIsDefault {
			if diffCapacityProviderStrategies(existing.CapacityProviderStrategy, s.CapacityProviders) {
				diffs.diff = true
				diffs.capToCapDiff = true
				diffs.Messages = append(diffs.Messages, "launch type changed from non-default to another non-default capacity provider")
			} // else no-op
		}
	}

	// min max healthy percent
	if *existing.DeploymentConfiguration.MinimumHealthyPercent != *s.MinHealthyPercent ||
		*existing.DeploymentConfiguration.MaximumPercent != *s.MaxPercent {
		diffs.diff = true
		diffs.minOrMaxHealthyDiff = true
		diffs.Messages = append(diffs.Messages, "minimum or maximum healthy percent is different")
	}

	// deployment circuit breaker settings
	if ((existing.DeploymentConfiguration == nil || existing.DeploymentConfiguration.DeploymentCircuitBreaker == nil || !existing.DeploymentConfiguration.DeploymentCircuitBreaker.Enable) && s.DeploymentCircuitBreaker.Enabled) ||
		((existing.DeploymentConfiguration != nil && existing.DeploymentConfiguration.DeploymentCircuitBreaker != nil && existing.DeploymentConfiguration.DeploymentCircuitBreaker.Enable) && !s.DeploymentCircuitBreaker.Enabled) {
		diffs.diff = true
		diffs.deployCircuitBreakerDiff = true
		diffs.Messages = append(diffs.Messages, "deployment circuit breaker config is different")
	} else if (existing.DeploymentConfiguration != nil && existing.DeploymentConfiguration.DeploymentCircuitBreaker != nil && existing.DeploymentConfiguration.DeploymentCircuitBreaker.Enable) && s.DeploymentCircuitBreaker.Enabled {
		if existing.DeploymentConfiguration.DeploymentCircuitBreaker.Rollback != s.DeploymentCircuitBreaker.Rollback {
			diffs.diff = true
			diffs.deployCircuitBreakerDiff = true
			diffs.Messages = append(diffs.Messages, "deployment circuit breaker config is different")
		}
	}

	// load balancing
	if len(s.LoadBalancing.TargetGroupAssignments) != len(existing.LoadBalancers) {
		diffs.loadBalancingDiff = true
		diffs.Messages = append(diffs.Messages, "service load balancing configurations are not the same")
	} else {
		// make a map existing loadbalancing...
		existingLbMap := make(map[string]ecstypes.LoadBalancer)
		for _, lb := range existing.LoadBalancers {
			existingLbMap[*lb.TargetGroupArn] = lb
		}

		var arn *string
		for n, definedLb := range s.LoadBalancing.TargetGroupAssignments {
			arn, err = targetGroupARNFromName(ctx, s.Context.ProviderName, definedLb.TargetGroupName)
			if err != nil {
				return nil, errors.Wrap(err, "error looking up target group arn")
			}
			s.LoadBalancing.TargetGroupAssignments[n].targetGroupArn = arn
			if existingLb, ok := existingLbMap[*arn]; !ok {
				diffs.loadBalancingDiff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("load balancing, target grouop %q to be added", definedLb.TargetGroupName))
			} else {
				// defined lb is there, but may be different
				if *existingLb.ContainerPort != definedLb.ContainerPort || *existingLb.ContainerName != definedLb.ContainerName {
					diffs.loadBalancingDiff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("load balancing, target grouop %q to be updated", definedLb.TargetGroupName))
				}
				delete(existingLbMap, *arn) // if exists, diff or not, we remove it from map...
			}
		}

		if len(existingLbMap) > 0 {
			diffs.loadBalancingDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v target grouops to be deleted", len(existingLbMap)))
		}
	}

	diffs.diff = diffs.diff || diffs.loadBalancingDiff

	// healthcheckGracePeriod
	// only healcheck grace period is same
	if len(s.LoadBalancing.TargetGroupAssignments) > 0 && //need to do this, to make sure lb's enabled
		util.CoalesceComparable(existing.HealthCheckGracePeriodSeconds, 0) != s.LoadBalancing.HealthcheckGracePeriod {
		diffs.diff = true
		diffs.lbHealthcheckDiff = true
		diffs.Messages = append(diffs.Messages, "load balancing healthcheck grace period is different")
	}

	if s.ServiceDiscovery.Name == "" && len(existing.ServiceRegistries) == 0 {
		// no op
	} else if s.ServiceDiscovery.Name == "" && len(existing.ServiceRegistries) > 0 {
		// service discovery being removed
		diffs.diff = true
		diffs.serviceDiscoveryDiff = true
		diffs.serviceDiscoveryArn = nil
		diffs.Messages = append(diffs.Messages, "service discovery config to be removed")
	} else {
		// fetch sd service arn
		sds := awsw.NewServiceDiscovery(ctx, s.Context.ProviderName)
		sdArn, err := sds.ServiceArnForNamespaceAndService(ctx,
			s.ServiceDiscovery.Namespace, s.ServiceDiscovery.Name)
		if err != nil {
			return nil, errors.Wrap(err, "cannot find service registry")
		}
		// service discovery is being added
		if len(existing.ServiceRegistries) == 0 {
			diffs.diff = true
			diffs.serviceDiscoveryDiff = true
			diffs.serviceDiscoveryArn = sdArn
			diffs.Messages = append(diffs.Messages, "service discovery config is being added")
		} else {
			// service discovery is being updated
			awsSd := existing.ServiceRegistries[0] //we only look at the first, since buildit supports only 1 SD name atm.

			// service registry "service.namespace" wasn't found
			// this happens if a new service discovery name was supplied
			// which hasn't been created but different from the existing
			// service discovery that exists
			if sdArn == nil {
				diffs.diff = true
				diffs.serviceDiscoveryDiff = true
				diffs.serviceDiscoveryArn = nil // should not happen
				diffs.Messages = append(diffs.Messages, "service discovery config is different, may not exist already")
			} else if *awsSd.RegistryArn != *sdArn {
				diffs.diff = true
				diffs.serviceDiscoveryDiff = true
				diffs.serviceDiscoveryArn = sdArn
				diffs.Messages = append(diffs.Messages, "service discovery config is different")
			}
		}
	}

	// subnets & security groups
	// TODO: this is assuming we are not changing the taskdef netowrking from awsvpc -> something else.
	oldUsingAwsvpc := existing.NetworkConfiguration != nil && existing.NetworkConfiguration.AwsvpcConfiguration != nil
	taskDef, err := taskDefByName(ctx, s.Context, s.TaskDefName)
	if err != nil {
		return nil, errors.Wrap(err, "cannot determine network mode for the task definition")
	}
	newUsingAwsvpc := taskDef.NetworkMode == ecstypes.NetworkModeAwsvpc

	if oldUsingAwsvpc && newUsingAwsvpc {
		if s.AssignPublicIP == nil || len(s.Subnets) == 0 || len(s.SecurityGroups) == 0 {
			return nil, errors.New("networking configuration parameters not provided to configure ecs service with awsvpc mode, check assignPublicIP subnets and securityGroups")
		}

		var subnetIDs []string
		var securityGroupIDs []string
		oldNetworkConfig := existing.NetworkConfiguration

		// assign public IP
		diffNetworking := (*s.AssignPublicIP != string(oldNetworkConfig.AwsvpcConfiguration.AssignPublicIp))

		// only changing subnets or security groups
		diffNetworking = diffNetworking ||
			len(s.Subnets) != len(oldNetworkConfig.AwsvpcConfiguration.Subnets) ||
			len(s.SecurityGroups) != len(oldNetworkConfig.AwsvpcConfiguration.SecurityGroups)

		// fetch subnet/security group Ids
		var err error
		subnetIDs, err = awsw.NewEC2(ctx, s.Context.ProviderName).SubnetIdsByNames(ctx, s.Subnets)
		if err != nil {
			return nil, errors.Wrap(err, "error getting subnets IDs from names")
		}

		// find vpc id from the first subnet, to filter out vpcs
		var vpcId *string
		vpcId, err = awsw.NewEC2(ctx, s.Context.ProviderName).VpcIdBySubnetName(ctx, s.Subnets[0])
		if err != nil {
			return nil, errors.Wrap(err, "error getting subnet vpc details")
		}

		securityGroupIDs, err = awsw.NewEC2(ctx, s.Context.ProviderName).SecurityGroupIdsByNames(ctx, vpcId, s.SecurityGroups)
		if err != nil {
			return nil, errors.Wrap(err, "error getting security groups IDs from names")
		}

		if !diffNetworking {
			diffNetworking =
				util.DiffStringSlices(subnetIDs, oldNetworkConfig.AwsvpcConfiguration.Subnets) ||
					util.DiffStringSlices(securityGroupIDs, oldNetworkConfig.AwsvpcConfiguration.SecurityGroups)
		}

		if diffNetworking {
			diffs.diff = true
			diffs.serviceNetworkingDiff = true
			diffs.subnetIdsToAdd = subnetIDs
			diffs.securityGroupIdsToAdd = securityGroupIDs
			diffs.Messages = append(diffs.Messages, "service networking or security is different")
		}
	}

	// tags
	awsTags, err := awsw.NewECS(ctx, s.Context.ProviderName).GetResourceTags(ctx, *existing.ServiceArn)
	if err != nil {
		return nil, err
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, s.Tags); tagDiff.HasChanges() {
		diffs.diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	// Update Autoscaling
	asDiffs, err := s.ServiceAutoScaling.Compare(ctx, s.Context)
	if err != nil {
		return nil, err
	}
	if asDiffs != nil {
		diffs.diff = true
		diffs.autoscalingDiff = true
		diffs.autoscalingDiffs = asDiffs.(*ApplicationAutoScalingDiff)
		diffs.Messages = append(diffs.Messages, asDiffs.Differences()...)
		diffs.Messages = append(diffs.Messages, "autoscaling policies are different")
	}

	// Check for last failed deployment
	// This happens when nothing changes in a service, but the last
	// deployment was a failuter (no rollback etc.)
	// In this case there is generally 0 tasks running & must be
	// force deployed
	if len(existing.Deployments) > 0 {
		if existing.Deployments[0].RolloutState == ecstypes.DeploymentRolloutStateFailed {
			diffs.diff = true
			diffs.previousDeploymentFailed = true
			diffs.Messages = append(diffs.Messages, "previous deployment is in failed state")
		}
	}

	if diffs.diff {
		diffs.servicetaskDef = taskDef // add taskDef info as well..
		return diffs, nil
	}

	log.Debugf("no diffs found for ecs service %v", s.Name)
	return nil, nil
}

// fetchExisting attemps to sync state for this resource with AWS, or returns an error
func (s ECSService) fetchExisting(ctx context.Context) (*ecstypes.Service, error) {

	ecsClient := client.ECS(ctx, s.Context.ProviderName)

	out, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(s.ClusterName),
		Services: []string{s.Name},
	})
	if err != nil {
		return nil, errors.Wrap(err, "error while retrieving service details")
	}

	if len(out.Services) == 0 {
		log.WithField("Name", s.Name).Debug("service does not exist")
		return nil, nil
	}

	old := out.Services[0]

	log.Debugf("service exists in %v status", *old.Status)
	//old services in INACTIVE state are candidates of deletion
	if *old.Status == "INACTIVE" {
		return nil, nil
	}

	return &old, nil
}

// apply creates the ECS service
func (s ECSService) apply(ctx context.Context) error {

	// set launchType or capacity provider strategy
	// ref: https://aws.amazon.com/blogs/containers/managing-compute-for-amazon-ecs-clusters-with-capacity-providers
	// there are 2 options when launching a service or scheduled task in an ECS Cluter.
	// EC2 or Fargate. However, you can choose to not use either & specify a custom strategy by using a capacity
	// provider. This is done by not supplying a `lauchType` attribute and using a capacity provider strategy instead.
	// With buildit, we need to explicitly specify `CAP` as the launchType value which sets the launchType to nil in
	// ECS & forces ECS to use the supplied or the cluster default capacity provider strategy for placement of tasks.
	var launchType ecstypes.LaunchType
	var capacityProviderStrategy []ecstypes.CapacityProviderStrategyItem
	if *s.LaunchType == CAP {
		capacityProviderStrategy = make([]ecstypes.CapacityProviderStrategyItem, len(s.CapacityProviders))
		for n, cp := range s.CapacityProviders {
			capacityProviderStrategy[n] = ecstypes.CapacityProviderStrategyItem{
				Base:             cp.Base,
				Weight:           cp.Weight,
				CapacityProvider: aws.String(cp.CapacityProviderName),
			}
		}
	} else {
		launchType = ecstypes.LaunchType(*s.LaunchType)
	}

	// TODO: Task placement constraints & strategy; currently hardcoded to binpack(memory) & AZ Spread
	// task placement must only be provided when scheduling strategy is REPLICA. For more information on
	// task placement strategy (& constraints) see reference link below:
	//  https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-placement.html
	// task placement strategy is only supported when launchType = EC2 is used.

	// since CAP can still use a strategy with FARGATE or FARGATE_SPOT capacity provider,
	// which do not support placementStrategy (or placementConstraints) we need to check each
	// supplied strategy and skip adding placement if FARGATE/FARGATE_SPOT is found.
	// when no explicity CAP strategy is supplied, the default strategy used by cluster may include
	// FARGATE/FARGATE_SPOT - so if launchType is not EC2/FARGATE, and no explicity CAP strategy is supplied for
	// a service, the default ECS Cluster CAP provider will be used & needs to be checked...
	var placementStrategy []ecstypes.PlacementStrategy
	usesFargate, err := s.usesFargate(ctx)
	if err != nil {
		return errors.Wrapf(err, "error checking if service uses fargate or fargate-based capacity provider strategy")
	}

	//only when service doesn't use FARGATE & type is not REPLICA, we assigne a placement strategy...
	if !usesFargate && *s.ServiceType == Replica {
		placementStrategy = make([]ecstypes.PlacementStrategy, 2)
		placementStrategy[0] = ecstypes.PlacementStrategy{
			Field: aws.String("attribute:ecs.availability-zone"),
			Type:  ecstypes.PlacementStrategyTypeSpread,
		}
		placementStrategy[1] = ecstypes.PlacementStrategy{
			Field: aws.String("memory"),
			Type:  ecstypes.PlacementStrategyTypeBinpack,
		}
	}

	// check network mode for the taskDef
	var networkConfiguration *ecstypes.NetworkConfiguration
	taskDef, err := taskDefByName(ctx, s.Context, s.TaskDefName)
	if err != nil {
		return errors.Wrap(err, "cannot determine network mode for the task definition")
	}

	network := taskDef.NetworkMode
	if *s.LaunchType == Fargate && network != ecstypes.NetworkModeAwsvpc {
		return errors.Errorf(
			"'FARGATE' tasks are not compatible with %s, expected %s", network, TaskNetworkingAwsvpc)
	}

	// no networking configuration must be provided when task networking != awsvpc
	if network != ecstypes.NetworkModeAwsvpc {
		if s.AssignPublicIP != nil || len(s.Subnets) != 0 || len(s.SecurityGroups) != 0 {
			return errors.Errorf("networking configuration parameters cannot be provided to configure ecs service with %s mode, remove assignPublicIp, subnets and seuritygroups", network)
		}
	}

	if network == ecstypes.NetworkModeAwsvpc {

		// default to DISABLED when awsvpc to avoid breaking changes...
		if s.AssignPublicIP == nil {
			s.AssignPublicIP = aws.String(string(ecstypes.AssignPublicIpDisabled))
		}

		if s.AssignPublicIP == nil || len(s.Subnets) == 0 || len(s.SecurityGroups) == 0 {
			return errors.New("networking configuration parameters not provided to configure ecs service with awsvpc mode, check assignPublicIP subnets and securityGroups")
		}

		subnetIDs, err := awsw.NewEC2(ctx, s.Context.ProviderName).SubnetIdsByNames(ctx, s.Subnets)
		if err != nil {
			return errors.Wrap(err, "error getting subnets IDs from names")
		}

		// find vpc id from the first subnet, to filter out vpcs
		var vpcId *string
		vpcId, err = awsw.NewEC2(ctx, s.Context.ProviderName).VpcIdBySubnetName(ctx, s.Subnets[0])
		if err != nil {
			return errors.Wrap(err, "error getting subnet vpc details")
		}

		securityGroupIDs, err := awsw.NewEC2(ctx, s.Context.ProviderName).SecurityGroupIdsByNames(ctx, vpcId, s.SecurityGroups)
		if err != nil {
			return errors.Wrap(err, "error getting security groups IDs from names")
		}

		networkConfiguration = &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				AssignPublicIp: ecstypes.AssignPublicIp(*s.AssignPublicIP),
				SecurityGroups: securityGroupIDs,
				Subnets:        subnetIDs,
			},
		}
	}

	// Deployment Circuit Breaker settings
	var deploymentCircuitBreaker *ecstypes.DeploymentCircuitBreaker
	if s.DeploymentCircuitBreaker.Enabled {
		deploymentCircuitBreaker = &ecstypes.DeploymentCircuitBreaker{
			Enable:   s.DeploymentCircuitBreaker.Enabled,
			Rollback: s.DeploymentCircuitBreaker.Rollback,
		}

	}

	// NOTE: pull container info from taskDef, need container name/port
	// for now we only have 1-1, but it can be >1 container /taskDef
	// so it's kind of necessary that it provided.
	// Also port mappings can be >1 for a container & we'll probably be
	// better off asking the client to provide which one to use for
	// the target group.

	// Loadbalancer & healthcheck settings; if no load balancer target groups are specified
	// ignore the health check grace period; this results in an error.
	var loadBalancers []ecstypes.LoadBalancer
	var healtcheckGracePeriodSeconds *int32
	if len(s.LoadBalancing.TargetGroupAssignments) > 0 {
		loadBalancers = make([]ecstypes.LoadBalancer, len(s.LoadBalancing.TargetGroupAssignments))
		for n, tg := range s.LoadBalancing.TargetGroupAssignments {
			arn, err := targetGroupARNFromName(ctx, s.Context.ProviderName, tg.TargetGroupName)
			if err != nil {
				return errors.Wrap(err, "failed to lookup target group arn")
			}
			loadBalancers[n] = ecstypes.LoadBalancer{
				ContainerName:  aws.String(tg.ContainerName),
				ContainerPort:  aws.Int32(tg.ContainerPort),
				TargetGroupArn: arn,
			}
		}
		healtcheckGracePeriodSeconds = aws.Int32(s.LoadBalancing.HealthcheckGracePeriod)
	}

	// Service Discovery
	var serviceRegistries []ecstypes.ServiceRegistry
	if s.ServiceDiscovery.Name != "" && s.ServiceDiscovery.Namespace != "" {
		arn, err := awsw.NewServiceDiscovery(ctx, s.Context.ProviderName).ServiceArnForNamespaceAndService(ctx, s.ServiceDiscovery.Namespace, s.ServiceDiscovery.Name)
		if err != nil || arn == nil {
			return errors.Wrap(err, "error creating service")
		}
		serviceRegistries = []ecstypes.ServiceRegistry{
			{
				RegistryArn: arn,
			},
		}
	}

	// tags
	tags := make([]ecstypes.Tag, 0)
	for k, v := range s.Tags {
		tag := ecstypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		tags = append(tags, tag)
	}

	//TODO: check if we need to open up DeploymentController to yaml config
	input := ecs.CreateServiceInput{
		Cluster:                  aws.String(s.ClusterName),
		ServiceName:              aws.String(s.Name),
		TaskDefinition:           aws.String(s.TaskDefName),
		LaunchType:               launchType, // only supply a launch type if it's EC2|FARGATE|EXTERNAL (not supported by buildit today)
		CapacityProviderStrategy: capacityProviderStrategy,
		PlacementStrategy:        placementStrategy,
		SchedulingStrategy:       ecstypes.SchedulingStrategy(*s.ServiceType), // replica|daemon
		DesiredCount:             s.DesiredCount,
		DeploymentConfiguration: &ecstypes.DeploymentConfiguration{
			MinimumHealthyPercent:    s.MinHealthyPercent,
			MaximumPercent:           s.MaxPercent,
			DeploymentCircuitBreaker: deploymentCircuitBreaker,
		},
		HealthCheckGracePeriodSeconds: healtcheckGracePeriodSeconds,
		LoadBalancers:                 loadBalancers,
		NetworkConfiguration:          networkConfiguration,
		ServiceRegistries:             serviceRegistries,
		EnableECSManagedTags:          true,
		PropagateTags:                 ecstypes.PropagateTagsService, // not configured
		Tags:                          tags,
	}

	ecsClient := client.ECS(ctx, s.Context.ProviderName)

	_, err = ecsClient.CreateService(ctx, &input)
	if err != nil {
		return errors.Wrap(err, "error creating ECS service")
	}

	log.WithFields(log.Fields{
		"Name": s.Name,
	}).Info(color.Green("ecs service created"))

	statsdClient := client.Statsd()
	dockerTags := taskDef.ContainerDefinitions[0].DockerLabels
	err = statsdClient.SendEvent("deploys.started", fmt.Sprintf("Started a deploy for service %s", s.Name), dockerTags)
	if err != nil {
		log.Warnf("Failed to send statsd event: %v", err)
	}

	// Autoscaling
	if !s.ServiceAutoScaling.IsZeroValue() {
		err = s.ServiceAutoScaling.apply(ctx, s.Context)
		if err != nil {
			return errors.Wrap(err, "error configuring autoscaling policies for the service")
		}
	}

	return nil
}

// applyDiffs applies the changes to an existing ECS service
func (s ECSService) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": s.Identifier(),
		}).Info("no updates required for ecs service")
		return nil
	}

	svcDiffs, ok := diffs.(*ECSServiceDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// invalid diffs
	if svcDiffs.schedulingStrategyDiff {
		return errors.Errorf("cannot update service type (scheduling strategy)")
	}

	if svcDiffs.launchTypeFargateDiff {
		return errors.New("cannot switch launch type to, or from FARGATE")
	}

	if svcDiffs.capToEc2Diff {
		return errors.New("cannot update service from capacity provider strategy to EC2 launch type")
	}

	if svcDiffs.defaultCapToCapDiff {
		return errors.New("cannot update service from a non-default to default capacity provider strategy")
	}

	if svcDiffs.capToDefaultCapDiff {
		return errors.New("cannot update service from a non-default to default capacity provider strategy")
	}

	if svcDiffs.previousDeploymentFailed {
		s.ForceNewDeployment = true // forcing deployment due to failed
		log.Info("previous deployment failed, force deployment")
	}

	// create update input
	input := &ecs.UpdateServiceInput{
		Cluster:            aws.String(s.ClusterName),
		Service:            aws.String(s.Name),
		ForceNewDeployment: s.ForceNewDeployment,
	}

	//desiredCount
	if svcDiffs.desiredCountDiff {
		input.DesiredCount = s.DesiredCount
	}

	// task definition
	if svcDiffs.taskDefDiff {
		input.TaskDefinition = aws.String(s.TaskDefName)
	}

	// capacity provider strategy
	if svcDiffs.ec2ToCapDiff || svcDiffs.defaultCapToCapDiff || svcDiffs.capToCapDiff {
		var capacityProviderStrategy []ecstypes.CapacityProviderStrategyItem
		for _, cp := range s.CapacityProviders {
			capacityProviderStrategy = append(capacityProviderStrategy, ecstypes.CapacityProviderStrategyItem{
				Base:             cp.Base,
				Weight:           cp.Weight,
				CapacityProvider: aws.String(cp.CapacityProviderName),
			})
		}
		input.CapacityProviderStrategy = capacityProviderStrategy
	}

	//min max healthy percent
	if svcDiffs.minOrMaxHealthyDiff {
		input.DeploymentConfiguration = &ecstypes.DeploymentConfiguration{
			MinimumHealthyPercent: s.MinHealthyPercent,
			MaximumPercent:        s.MaxPercent,
		}
	}

	//deployment circuit breaker settings
	if svcDiffs.deployCircuitBreakerDiff {
		deploymentCircuitBreaker := &ecstypes.DeploymentCircuitBreaker{
			Enable:   s.DeploymentCircuitBreaker.Enabled,
			Rollback: s.DeploymentCircuitBreaker.Rollback,
		}
		if input.DeploymentConfiguration != nil {
			input.DeploymentConfiguration.DeploymentCircuitBreaker = deploymentCircuitBreaker
		} else {
			input.DeploymentConfiguration = &ecstypes.DeploymentConfiguration{
				DeploymentCircuitBreaker: deploymentCircuitBreaker,
			}
		}
	}

	// service discovery
	if svcDiffs.serviceDiscoveryDiff {
		// TODO remove this
		//  return errors.New("service discovery cannot be updated")
		input.ServiceRegistries = []ecstypes.ServiceRegistry{{
			RegistryArn: svcDiffs.serviceDiscoveryArn,
		}}
	}

	// networking
	if svcDiffs.serviceNetworkingDiff {
		input.NetworkConfiguration = &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				AssignPublicIp: ecstypes.AssignPublicIp(*s.AssignPublicIP),
				SecurityGroups: svcDiffs.securityGroupIdsToAdd,
				Subnets:        svcDiffs.subnetIdsToAdd,
			},
		}
	}

	// healthcheckGracePeriod
	if svcDiffs.lbHealthcheckDiff {
		input.HealthCheckGracePeriodSeconds = aws.Int32(s.LoadBalancing.HealthcheckGracePeriod)
	}

	// load balancing
	if svcDiffs.loadBalancingDiff {
		lbs := make([]ecstypes.LoadBalancer, 0) // make the array & add to input anyway since deleting all tgs requires an empty []
		for _, tg := range s.LoadBalancing.TargetGroupAssignments {
			var err error
			arn := tg.targetGroupArn
			if tg.targetGroupArn == nil {
				log.Warnf("getting arn for target group, %v", color.Yellow("yukk!!"))
				arn, err = targetGroupARNFromName(ctx, s.Context.ProviderName, tg.TargetGroupName)
				if err != nil {
					return errors.Wrap(err, "failed to lookup target group arn")
				}
			}
			lbs = append(lbs, ecstypes.LoadBalancer{
				ContainerName:  aws.String(tg.ContainerName),
				ContainerPort:  aws.Int32(tg.ContainerPort),
				TargetGroupArn: arn,
			})
		}
		input.LoadBalancers = lbs
	}

	ecsClient := client.ECS(ctx, s.Context.ProviderName)
	if svcDiffs.diff || svcDiffs.foreceDeploymentDiff {
		if !svcDiffs.diff {
			log.WithField("Name", s.Name).Info(
				"no diffs found, but service will be updated as Force New Deployment flag is ON")
		}
		_, err := ecsClient.UpdateService(ctx, input)
		if err != nil {
			return errors.Wrap(err, "error updating service")
		}
		log.WithField("Name", s.Name).Info(color.Yellow("ecs service updated"))

		// send stats
		statsdClient := client.Statsd()
		taskDef := svcDiffs.servicetaskDef                         // TODO: fix this
		dockerTags := taskDef.ContainerDefinitions[0].DockerLabels // aws.StringValueMap(taskDef.ContainerDefinitions[0].DockerLabels)
		err = statsdClient.SendEvent("deploys.started", fmt.Sprintf("Started a deploy for service %s", s.Name), dockerTags)
		if err != nil {
			log.Warnf("Failed to send statsd event: %v", err)
		}
	}

	// tags
	if svcDiffs.tagsDiff {
		upserts := svcDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			err := awsw.NewECS(ctx, s.Context.ProviderName).AddResourceTags(ctx, svcDiffs.serviceArn, upserts)
			if err != nil {
				return errors.Wrapf(err, "error updating ecs service tags for %v", s.Identifier())
			}
		}

		if len(svcDiffs.tagDiff.Deleted) > 0 {
			err := awsw.NewECS(ctx, s.Context.ProviderName).DeleteResourceTags(ctx, svcDiffs.serviceArn, svcDiffs.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error updating ecs service tags for %v", s.Identifier())
			}
		}
	}

	// Update Autoscaling
	// if autoscaling diffs are found, we apply those diffs.
	if svcDiffs.autoscalingDiff {
		if svcDiffs.autoscalingDiffs != nil {
			err := s.ServiceAutoScaling.applyDiffs(ctx, s.Context, svcDiffs.autoscalingDiffs)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// usesFargate returns true if the service is to be run with Fargate launch type OR a capacity provider strategy based
// on fargate (either explicitly or implicitly)
func (s ECSService) usesFargate(ctx context.Context) (bool, error) {

	switch *s.LaunchType {
	case Fargate:
		return true, nil
	case EC2:
		return false, nil
	}

	//if explicit CAP strategy is supplied, check if it's FARGATE based...
	if len(s.CapacityProviders) > 0 {
		for _, cps := range s.CapacityProviders {
			if cps.CapacityProviderName == CAP_Fargate || cps.CapacityProviderName == CAP_FargateSpot {
				return true, nil
			}
		}
	} else {
		//if no explicit CAP strategy is supplied, check default capacity provider strategy of the cluster...
		ecsClient := client.ECS(ctx, s.Context.ProviderName)
		out, err := ecsClient.DescribeClusters(ctx, &ecs.DescribeClustersInput{
			Clusters: []string{s.ClusterName},
		})

		if err != nil {
			return false, errors.Wrapf(err, "error fetching cluter details for %v", s.ClusterName)
		}

		for _, cluster := range out.Clusters {
			if *cluster.ClusterName == s.ClusterName {
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

// waitUntilServiceInactive waits untils the ecs service is in either removed or in INACTIVE. Currently this
// function will wait 15s between each check & perform a maximum of 40 checks, so the maximum wait time before
// this function exits with a failure is 600s or 10m
func waitUntilServiceInactive(ctx context.Context, providerName string, cluster string, service string) error {

	ecsClient := client.ECS(ctx, providerName)
	done := false
	retries := retriesRemaining

	for !done {
		log.Debugf("waiting %v seconds to check service status", waitInSeconds)
		time.Sleep(time.Duration(waitInSeconds) * time.Second)

		servResp, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  aws.String(cluster),
			Services: []string{service},
		})

		if err != nil {
			return errors.Wrap(err, "error fetching service details")
		}

		//if service is not found, return success
		if len(servResp.Services) == 0 {
			log.Infof("service %v has been removed", service)
			return nil
		}

		if *servResp.Services[0].Status == "INACTIVE" {
			log.Infof("service %v is now in INACTIVE state", service)
			return nil
		}

		retries--
		done = retries == 0
	}

	return errors.New("couldln't verify is service was INACTIVE within specified time")
}

// helper to lookup tg ARN by name
func targetGroupARNFromName(ctx context.Context, providerName string, targetGroupName string) (*string, error) {
	tg, err := targetGroupFromName(ctx, providerName, targetGroupName)
	if err != nil {
		return nil, errors.Wrapf(err, "error finding target group %v", targetGroupName)
	}
	return tg.TargetGroupArn, nil
}

// helper to lookup tg by name
func targetGroupFromName(ctx context.Context, providerName string, targetGroupName string) (*elbv2types.TargetGroup, error) {
	elbClient := client.ELB(ctx, providerName)

	out, err := elbClient.DescribeTargetGroups(ctx, &elbv2.DescribeTargetGroupsInput{
		Names: []string{targetGroupName},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to find target group")
	}
	if len(out.TargetGroups) == 0 {
		return nil, errors.Errorf("coudln't find target group by name %v", targetGroupName)
	}
	return &out.TargetGroups[0], nil
}

// helper to lookup taskDef by taskDefName
// taskDefName can be the taskDefinition family or family:revision
// if revision is not specfied LATEST is assumed
func taskDefByName(ctx context.Context, rctx Context, taskDefName string) (*ecstypes.TaskDefinition, error) {

	ecsClient := client.ECS(ctx, rctx.ProviderName)

	out, err := ecsClient.DescribeTaskDefinition(
		ctx, &ecs.DescribeTaskDefinitionInput{
			TaskDefinition: &taskDefName,
		})

	if err != nil {
		return nil, errors.Wrap(err, "failed to find task defintion")
	}

	return out.TaskDefinition, nil
}

// returns the taskDef name & revision from the arn; to be used with API returned or valid taskDef ARNs, no validation
func taskDefNameRevisionFromArn(ctx context.Context, rctx Context, arn string) (taskDefName string, taskDefRev int32) {

	index := strings.LastIndex(arn, "/")
	var nameRev string
	if index == -1 {
		nameRev = arn
	} else {
		nameRev = arn[index+1:]
	}

	index = strings.LastIndex(nameRev, ":")
	if index == -1 {
		taskDef, _ := taskDefByName(ctx, rctx, nameRev)
		if taskDef != nil {
			taskDefRev = taskDef.Revision
		}
		taskDefName = nameRev
		return
	}

	ver, err := strconv.Atoi(nameRev[index+1:])
	if err != nil {
		taskDef, _ := taskDefByName(ctx, rctx, nameRev)
		if taskDef != nil {
			taskDefRev = taskDef.Revision
		}
		taskDefName = nameRev
		return
	}
	return nameRev[0:index], int32(ver)
}

// compares 2 capacity providers
func diffCapacityProviderStrategies(left []ecstypes.CapacityProviderStrategyItem, right []ECSCapacityProviderStrategy) bool {

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

// returns true if the cluster with the provided `clusterName` has a default capacity provider strategy
// equal to the provider strategy `left` - if the cluster cannot be described, this would return `false`
func isDefaultCapacityProviderStrategy(ctx context.Context, rctx Context, clusterName string, left []ecstypes.CapacityProviderStrategyItem) bool {

	ecsClient := client.ECS(ctx, rctx.ProviderName)
	out, err := ecsClient.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{clusterName},
	})

	if err != nil || len(out.Clusters) == 0 {
		return false
	}

	right := out.Clusters[0].DefaultCapacityProviderStrategy //default
	if len(left) != len(right) {
		return false
	}

	rMap := make(map[string]ecstypes.CapacityProviderStrategyItem)
	for _, rVal := range right {
		rMap[*rVal.CapacityProvider] = rVal
	}
	for _, lVal := range left {
		rVal, ok := rMap[*lVal.CapacityProvider]
		if !ok {
			return false
		}
		if *lVal.CapacityProvider != *rVal.CapacityProvider ||
			lVal.Base != rVal.Base || lVal.Weight != rVal.Weight {
			return false
		}
		delete(rMap, *lVal.CapacityProvider)
	}
	return true
}

// returns true if the cluster with the provided `clusterName` has a default capacity provider strategy
// similar to the provider strategy representaton `left` - if the cluster cannot be described, this would return `false`
func isSameAsDefaultCAPStrategy(ctx context.Context, rctx Context, clusterName string, left []ECSCapacityProviderStrategy) bool {

	ecsClient := client.ECS(ctx, rctx.ProviderName)
	out, err := ecsClient.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{clusterName},
	})

	if err != nil || len(out.Clusters) == 0 {
		return false
	}

	right := out.Clusters[0].DefaultCapacityProviderStrategy //default
	if len(left) != len(right) {
		return false
	}

	rMap := make(map[string]ecstypes.CapacityProviderStrategyItem)
	for _, rVal := range right {
		rMap[*rVal.CapacityProvider] = rVal
	}
	for _, lVal := range left {
		rVal, ok := rMap[lVal.CapacityProviderName]
		if !ok {
			return false
		}
		if lVal.CapacityProviderName != *rVal.CapacityProvider ||
			lVal.Base != rVal.Base || lVal.Weight != rVal.Weight {
			return false
		}
		delete(rMap, lVal.CapacityProviderName)
	}
	return true
}
