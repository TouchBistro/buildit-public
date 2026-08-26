package resource

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	TaskStatusStopped = "STOPPED"
)

type StandaloneTask struct {
	BaseResource `yaml:",inline"`
	Name                 string                             `yaml:"-"`
	LaunchType           string                             `yaml:"launchType"`
	CapacityProviders    []ECSCapacityProviderStrategy      `yaml:"capacityProviders"`
	TaskDefName          string                             `yaml:"taskDefName"`
	ClusterName          string                             `yaml:"clusterName"`
	NetworkConfiguration StandaloneTaskNetworkConfiguration `yaml:"networkConfiguration"`
	TimeoutSeconds       *int                               `yaml:"timeoutSeconds"` // explicitly set to zero to disable timeout
	Concurrent           *bool                              `yaml:"concurrent"`
	Overrides            *StandaloneTaskOverrides           `yaml:"overrides"`
	DependsOn            []Key                              `yaml:"dependsOn"`
	Tags                 map[string]string                  `yaml:"tags"`
	GlobalTags           map[string]string                  `yaml:"-"`
}

type StandaloneTaskNetworkConfiguration struct {
	AssignPublicIP     string   `yaml:"assignPublicIp"` // Enabled | Disabled when awsvpc networking
	SubnetNames        []string `yaml:"subnetNames"`
	SecurityGroupNames []string `yaml:"securityGroupNames"`
}

type StandaloneTaskOverrides struct {
	ContainerOverrides []StandaloneTaskContainerOverrides `yaml:"containerOverrides"`
}

type StandaloneTaskContainerOverrides struct {
	Name        string       `yaml:"name"`
	Command     []string     `yaml:"command"`
	Environment []TaskEnvVar `yaml:"environment"`
}

// Key returns the unique key for the resource for this buildit context
func (st StandaloneTask) Key() Key {
	return NewKey(st.Context.ProviderName, st.Identifier())
}

func (st StandaloneTask) Identifier() string {
	return st.Name
}

func (st *StandaloneTask) Normalize(ctx context.Context) {
	st.LaunchType = strings.ToUpper(st.LaunchType)
	st.NetworkConfiguration.AssignPublicIP = strings.ToUpper(st.NetworkConfiguration.AssignPublicIP)
	if st.NetworkConfiguration.AssignPublicIP == "" {
		st.NetworkConfiguration.AssignPublicIP = string(ecstypes.AssignPublicIpDisabled)
	}

	// for backward compatibility
	if st.TimeoutSeconds == nil {
		st.TimeoutSeconds = aws.Int(600) // default to 10min
	}
	if st.Overrides != nil && st.Overrides.ContainerOverrides == nil {
		st.Overrides.ContainerOverrides = make([]StandaloneTaskContainerOverrides, 0)
	}

	if st.Concurrent == nil {
		st.Concurrent = aws.Bool(false)
	}

	//Merge globalTags to standalone tasks tags, if key is not already present
	//later we'll use sg.Tags to add/update tags
	if st.Tags == nil {
		st.Tags = make(map[string]string)
	}
	ResourceTags(st.Tags).Merge(st.GlobalTags)
}

func (st StandaloneTask) Validate(ctx context.Context) error {
	var errMessages []string

	switch st.LaunchType {
	case Fargate, EC2, CAP:
	default:
		msg := fmt.Sprintf("invalid launch type %q, must be 'EC2', 'FARGATE' or 'CAP'", st.LaunchType)
		errMessages = append(errMessages, msg)
	}

	switch st.NetworkConfiguration.AssignPublicIP {
	case string(ecstypes.AssignPublicIpEnabled), string(ecstypes.AssignPublicIpDisabled):
	default:
		msg := fmt.Sprintf("invalid value for assignPublicIp %q, must be 'ENABLED' or 'DISABLED'", st.NetworkConfiguration.AssignPublicIP)
		errMessages = append(errMessages, msg)
	}

	if errMessages == nil {
		return nil
	}
	return &ValidationError{
		ResourceIdentifier: st.Identifier(),
		ResourceType:       "Standalone Task",
		Messages:           errMessages,
	}
}

// Apply starts a standalone tasks, or if already running, it will restart it.
func (st StandaloneTask) Apply(ctx context.Context) error {
	//concurrent is not allowed
	if !*st.Concurrent {
		diffs, err := st.Compare(ctx)
		if err != nil {
			return err
		}

		//if diff found & existing resource exists (standalone task)
		if diffs != nil && diffs.AWSResource() != nil {
			return st.applyDiffs(ctx, diffs)
		}
	}

	return st.apply(ctx)
}

// Destroy stops any currently running standalone tasks matching the supplied name
func (st StandaloneTask) Destroy(ctx context.Context) error {

	diffs, err := st.Compare(ctx)
	if err != nil {
		return errors.Wrapf(err, "error fetching running tasks for %v", st.Identifier())
	}

	if diffs != nil && diffs.AWSResource() == nil {
		log.WithFields(log.Fields{
			"Name": st.Identifier(),
		}).Info("standalone task is not running, nothing to stop, skipping")
		return nil
	}

	runningTasks := diffs.AWSResource().([]ecstypes.Task)
	return st.stop(ctx, runningTasks)
}

type StandaloneTaskDiff struct {
	BaseResourceDiff
}

// Compare checks if there are currently any tasks running that match
// the supplied task name.
// Standalone task is a special case resource. Since there is not long
// standing resource provisioned as a result, we only check if there is
// currently an instance of the standalone task running..
//
// If there are no instances of tasks running for this standalone task name,
// we consider this a diff of type were resource needs to be created...
//
// If there are any instances of tasks running for this standalone task name,
// we consider this a diff of type where resources needs to be re-created
//
// There is no field-by-field comparisons to be made for standalone tasks..
func (sg StandaloneTask) Compare(ctx context.Context) (ResourceDiff, error) {

	running, err := sg.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", sg.Identifier())
	}

	diffs := &StandaloneTaskDiff{}
	if running == nil {
		diffs.Messages = append(diffs.Messages, "standalone tasks instances are not running")
		return diffs, nil
	}

	// else we have running tasks
	diffs.Resource = running
	return diffs, nil
}

// run a standalone task
func (st StandaloneTask) apply(ctx context.Context) error {

	// We only support running one task at a time per standalone task.
	taskDef, err := taskDefByName(ctx, st.Context, st.TaskDefName)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve task def for standalone task")
	}

	network := taskDef.NetworkMode

	if st.LaunchType == Fargate && network != TaskNetworkingAwsvpc {
		return errors.Errorf("'FARGATE' tasks are not compatible with %s, expected %s", network, TaskNetworkingAwsvpc)
	}
	// no networking configuration must be provided when task networking != awsvpc
	if network != TaskNetworkingAwsvpc {
		if len(st.NetworkConfiguration.SubnetNames) != 0 || len(st.NetworkConfiguration.SecurityGroupNames) != 0 {
			return errors.Errorf("networking configuration parameters cannot be provided to configure standalone task with %s mode, remove assignPublicIp, subnets and seuritygroups", network)
		}
	} else {
		if len(st.NetworkConfiguration.SubnetNames) == 0 || len(st.NetworkConfiguration.SecurityGroupNames) == 0 {
			return errors.Errorf("networking configuration parameters not provided to configure standalone task with awsvpc mode, check assignPublicIP, subnets and securityGroups")
		}
	}

	// network configuration
	var networkConfiguration *ecstypes.NetworkConfiguration
	if network == TaskNetworkingAwsvpc {

		//TODO: this can be moved into Validate(ctx context.Context)

		//TODO: @esiddiqui move this to awsw/utils
		subnetIDs, err := awsw.NewEC2(ctx, st.Context.ProviderName).SubnetIdsByNames(ctx, st.NetworkConfiguration.SubnetNames)
		if err != nil {
			return errors.Wrap(err, "error getting subnets IDs from names")
		}
		securityGroupIDs, err := awsw.NewEC2(ctx, st.Context.ProviderName).SecurityGroupIdsByNames(ctx, nil, st.NetworkConfiguration.SecurityGroupNames)
		if err != nil {
			return errors.Wrap(err, "error getting security groups IDs from names")
		}

		networkConfiguration = &ecstypes.NetworkConfiguration{
			AwsvpcConfiguration: &ecstypes.AwsVpcConfiguration{
				AssignPublicIp: ecstypes.AssignPublicIp(st.NetworkConfiguration.AssignPublicIP),
				SecurityGroups: securityGroupIDs,
				Subnets:        subnetIDs,
			},
		}
	}

	// overrides
	var taskOverrides *ecstypes.TaskOverride
	if st.Overrides != nil {
		// DISCUSS(@maintainer): Should we explicitly validate the container names here or
		// should we just let AWS handle it when we make the RunTask call?
		var containerOverrides []ecstypes.ContainerOverride
		for _, co := range st.Overrides.ContainerOverrides {

			environment := make([]ecstypes.KeyValuePair, 0)
			for _, ev := range co.Environment {
				environment = append(environment, ecstypes.KeyValuePair{
					Name:  aws.String(ev.Name),
					Value: aws.String(ev.Value),
				})
			}
			containerOverrides = append(containerOverrides, ecstypes.ContainerOverride{
				Name:        aws.String(co.Name),
				Command:     co.Command,
				Environment: environment,
			})
		}
		taskOverrides = &ecstypes.TaskOverride{
			ContainerOverrides: containerOverrides,
		}
	}

	// tags
	var tags []ecstypes.Tag
	for k, v := range st.Tags {
		tags = append(tags, ecstypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	// launch type
	var launchType ecstypes.LaunchType
	var capacityProviderStrategy []ecstypes.CapacityProviderStrategyItem
	if st.LaunchType == CAP {
		capacityProviderStrategy = make([]ecstypes.CapacityProviderStrategyItem, len(st.CapacityProviders))
		for n, cp := range st.CapacityProviders {
			capacityProviderStrategy[n] = ecstypes.CapacityProviderStrategyItem{
				Base:             cp.Base,
				Weight:           cp.Weight,
				CapacityProvider: aws.String(cp.CapacityProviderName),
			}
		}
	} else {
		launchType = ecstypes.LaunchType(st.LaunchType)
	}

	ecsClient := client.ECS(ctx, st.Context.ProviderName)
	runTaskInput := &ecs.RunTaskInput{
		Cluster:                  aws.String(st.ClusterName),
		Count:                    aws.Int32(1),
		EnableECSManagedTags:     true,
		LaunchType:               launchType, //TODO: when CAP this is zero value
		CapacityProviderStrategy: capacityProviderStrategy,
		NetworkConfiguration:     networkConfiguration,
		Overrides:                taskOverrides,
		StartedBy:                aws.String(st.Name), //TODO: export this to yaml config, default to st.Name; use the standalone task name as the startedBy value so that we can query for existing tasks elsewhere.
		Tags:                     tags,
		TaskDefinition:           aws.String(st.TaskDefName),
		ReferenceId:              aws.String(st.Name),
	}

	runTaskOutput, err := ecsClient.RunTask(ctx, runTaskInput)
	if err != nil {
		return errors.Wrapf(err, "failed to run standalone task: %s", st.Identifier())
	}
	if len(runTaskOutput.Failures) > 0 {
		return errors.Errorf("failed to run standalone task: %s\n%s", st.Identifier(), formatFailures(runTaskOutput.Failures))
	}
	// Should only be a single task since we specified count 1.
	if len(runTaskOutput.Tasks) != 1 {
		return errors.Errorf("expected 1 task to be run, got count %d", len(runTaskOutput.Tasks))
	}
	log.WithFields(log.Fields{
		"Name": st.Name,
		"ARN":  *runTaskOutput.Tasks[0].TaskArn,
	}).Info(color.Green("standalone task started"))

	if *st.TimeoutSeconds == 0 {
		// No timeout configured, just return after starting the task.
		log.Debugf("No waiting for standalone task completion: %s", st.Identifier())
		return nil
	}

	// Now we need to wait for the task to complete. Keep occasionally polling and check the status.
	// The poll interval might need to be tweaked. Shorter intervals mean we can detect task
	// completion quicker but we will be making more AWS API calls.
	var completedTask *ecstypes.Task
	const checkInterval = 15
	// Determine the number of attempts based on the total timeout period.
	maxAttempts := *st.TimeoutSeconds / checkInterval
	for i := 0; i < maxAttempts; i++ {
		log.Debugf("Waiting %d seconds to see if standalone task is complete: %s", checkInterval, st.Identifier())
		time.Sleep(checkInterval * time.Second)
		describeTasksOutput, err := ecsClient.DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(st.ClusterName),
			Tasks:   []string{*runTaskOutput.Tasks[0].TaskArn},
		})
		if err != nil {
			return errors.Wrap(err, "failed to get task details")
		}
		if len(describeTasksOutput.Failures) > 0 {
			return errors.Errorf("failed to get task details:\n%s", formatFailures(describeTasksOutput.Failures))
		}
		if len(runTaskOutput.Tasks) != 1 {
			return errors.Errorf("expected 1 task, got count %d", len(runTaskOutput.Tasks))
		}
		// Use the desired status since there are some other states a task goes through while in
		// the process of being stopped (ex: DEPROVISIONING). Once the desired status is STOPPED
		// we know that the command run has completed so we can safely proceed with other operations
		// while ECS stops the task in parallel.
		if *describeTasksOutput.Tasks[0].DesiredStatus == TaskStatusStopped {
			completedTask = &describeTasksOutput.Tasks[0]
			break
		}
	}

	// Check if task finished, if not the timeout was reached so stop the task and report the failure
	if completedTask == nil {
		_, err := ecsClient.StopTask(ctx, &ecs.StopTaskInput{
			Cluster: aws.String(st.ClusterName),
			Task:    runTaskOutput.Tasks[0].TaskArn,
		})
		if err != nil {
			return errors.Wrap(err, "failed to stop task after timeout")
		}
		return errors.Errorf("timed out waiting for standalone task to complete")
	}

	// Task completed, check if it was successful by checking the container exit code.
	// Check the exit code for each container that was overridden.
	overriddenContainers := make(map[string]struct{})
	if st.Overrides != nil {
		for _, c := range st.Overrides.ContainerOverrides {
			overriddenContainers[c.Name] = struct{}{}
		}
	}
	for _, c := range completedTask.Containers {
		if _, ok := overriddenContainers[*c.Name]; !ok {
			// Skip since it wasn't overridden
			continue
		}
		// Verify it exited successfully
		// Hopefully this should never be nil but guard against it just to be safe
		exitCode := -1
		if c.ExitCode != nil {
			exitCode = int(*c.ExitCode)
		}
		if exitCode != 0 {
			return errors.Errorf("container %q exited with unsuccessful code %d", *c.Name, exitCode)
		}
	}
	return nil
}

// stop curently running tasks & run a new standalone task
func (st StandaloneTask) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	existing := diffs.AWSResource()
	err := st.stop(ctx, existing.([]ecstypes.Task))
	if err != nil {
		return errors.Wrapf(err, "error stopping currently running instances of %v", st.Identifier())
	}
	return st.apply(ctx)
}

// fetchExisting checks to see if there are any currently executing (Desired state) tasks
// around for the the supplied task name
func (st StandaloneTask) fetchExisting(ctx context.Context) ([]ecstypes.Task, error) {
	entry := log.WithFields(log.Fields{"Identifier": st.Identifier()})
	entry.Debug("checking if any existing tasks are running")
	ecsClient := client.ECS(ctx, st.Context.ProviderName)
	var taskArns []string
	var nextToken *string
	for {
		listTasksOutput, err := ecsClient.ListTasks(ctx, &ecs.ListTasksInput{
			Cluster:   aws.String(st.ClusterName),
			StartedBy: aws.String(st.Name),
			NextToken: nextToken,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to list tasks")
		}
		taskArns = append(taskArns, listTasksOutput.TaskArns...)
		if listTasksOutput.NextToken == nil {
			break
		}
		nextToken = listTasksOutput.NextToken
	}
	if len(taskArns) == 0 {
		// No tasks found, nothing to stop
		return nil, nil
	}

	describeTasksOutput, err := ecsClient.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(st.ClusterName),
		Tasks:   taskArns,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to describe tasks")
	}
	if len(describeTasksOutput.Failures) > 0 {
		return nil, errors.Errorf("failed to describe tasks:\n%s", formatFailures(describeTasksOutput.Failures))
	}
	if len(describeTasksOutput.Tasks) == 0 {
		entry.Debug("no tasks found")
		return nil, nil
	}

	return describeTasksOutput.Tasks, nil
}

// stop stops all tasks where DESIRED status is not STOPPED already
func (st StandaloneTask) stop(ctx context.Context, tasks []ecstypes.Task) error {
	entry := log.WithFields(log.Fields{"Identifier": st.Identifier()})
	ecsClient := client.ECS(ctx, st.Context.ProviderName)

	if len(tasks) == 0 {
		log.WithFields(log.Fields{
			"Name": st.Name,
		}).Debug(color.Yellow("standalone task does not exist"))
	}

	for _, task := range tasks {
		if *task.DesiredStatus == TaskStatusStopped {
			continue
		}
		entry.WithFields(log.Fields{
			"ARN": *task.TaskArn,
		}).Debug("stopping task")
		_, err := ecsClient.StopTask(ctx, &ecs.StopTaskInput{
			Cluster: aws.String(st.ClusterName),
			Task:    task.TaskArn,
		})
		if err != nil {
			return errors.Wrap(err, "failed to stop task")
		}

		entry.WithFields(log.Fields{
			"ARN": *task.TaskArn,
		}).Info(color.Red("standalone task stopped"))
	}
	return nil
}
