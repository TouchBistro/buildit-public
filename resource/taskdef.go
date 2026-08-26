package resource

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

const (
	taskExecutionRoleDefault string = "ecsTaskExecutionRole"
)

const (
	// TaskNetworkingBridge bridge docker networking
	TaskNetworkingBridge = ecstypes.NetworkModeBridge
	// TaskNetworkingHost host docker networking
	TaskNetworkingHost = ecstypes.NetworkModeHost
	// TaskNetworkingAwsvpc awsvpc docker CNI plugin
	TaskNetworkingAwsvpc = ecstypes.NetworkModeAwsvpc
	// TaskNetworkingNone no networking
	TaskNetworkingNone = ecstypes.NetworkModeNone
	// TaskNetworkingDefault default task networking is bridge
	TaskNetworkingDefault = ecstypes.NetworkModeBridge
)

const (
	// HostBindMount represents the bind-mount host volume
	HostBindMount = "host"
	// EFSVolume represents the Amazon EFS volume
	EFSVolume = "efs"
	// DockerVolume represents a docker volume
	DockerVolume = "docker"
	// FSXWindowsVolume represents an Amazon FSX windows storage
	FSXWindowsVolume = "fsx"
)

// for specific documentation on ECS taskdef or container attributes, see AWS documentation:
// https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-resource-ecs-taskdefinition.html

// for corresponding docker reference see Docker documentation:
// https://docs.docker.com/engine/reference/run/

// ContainerPortMapping represents a port mapping definition
type ContainerPortMapping struct {
	Name          string `yaml:"name"`
	ContainerPort int32  `yaml:"containerPort"`
	HostPort      int32  `yaml:"hostPort"` // same as container port for awsvpc/host network mode
	Protocol      string `yaml:"protocol"` // tcp or udp only
}

// ContainerHealthcheck represents container healthcheck definition
type ContainerHealthcheck struct {
	Command     []string `yaml:"command"`
	Interval    int32    `yaml:"interval"`    // healtchecking interval in seconds
	Timeout     int32    `yaml:"timeout"`     // timeout in seconds for healthcheck try
	StartPeriod int32    `yaml:"startPeriod"` // grace period before failing healtchecks are considered
	Retries     int32    `yaml:"retries"`     // number of retries for failed healtchecks before marking containers down
}

// ContainerMountPoints represents volume mount points for containers
type ContainerMountPoints struct {
	ReadOnly      bool   `yaml:"readOnly"`
	ContainerPath string `yaml:"containerPath"`
	SourceVolume  string `yaml:"sourceVolume"`
}

// ContainerLogging represents the taskDef log driver configuration
type ContainerLogging struct {
	Driver        string            `yaml:"logDriver"`
	Options       map[string]string `yaml:"options"`
	SecretOptions map[string]string `yaml:"secretOptions"`
}

// ContainerLinuxParameters  the linux parameters for the container
type ContainerLinuxParameters struct {
	Capabilities       KernelCapabilities `yaml:"capabilities"`
	InitProcessEnabled *bool              `yaml:"initProcessEnabled"`
	// TODO: implement as needed, note that all of these are not supported on Fargate
	// SharedMemorySize
	// Tmpfs
	// Devices
	// MaxSwap
	// Swappiness
}

// KernelCapabilities the kernel parameters to add or drop
type KernelCapabilities struct {
	Add  []string `yaml:"add"`
	Drop []string `yaml:"drop"`
}

// FirelensConfiguration configuration for a firelens container
type FirelensConfiguration struct {
	Type    *string           `yaml:"type"`
	Options map[string]string `yaml:"options"`
}

// RuntimePlatform configuration for the task
type RuntimePlatform struct {
	CpuArchitecture       string `yaml:"cpuArchitecture"`       // X86_64 | ARM64
	OperatingSystemFamily string `yaml:"operatingSystemFamily"` // LINUX | WINDOWS_SERVER_2019_FULL | WINDOWS_SERVER_2019_CORE | WINDOWS_SERVER_2022_FULL | WINDOWS_SERVER_2022_CORE
}

// Container represents the container image & its configuration to run within a task
type Container struct {
	Name              string                    `yaml:"name"`
	Image             string                    `yaml:"image"`
	CPU               *int32                    `yaml:"cpu"`
	Memory            *int32                    `yaml:"memory"`            // maximum memory (MiB) to be allocated (hard) to the container
	MemoryReservation *int32                    `yaml:"memoryReservation"` // initial memory (MiB) reservation (soft), can go upto hard memory value
	PortMappings      []ContainerPortMapping    `yaml:"portMappings"`
	Healthcheck       ContainerHealthcheck      `yaml:"healthcheck"`
	Entrypoint        []string                  `yaml:"entrypoint"`
	EnvVars           map[string]string         `yaml:"envVars"`
	Secrets           map[string]string         `yaml:"secrets"`
	Labels            map[string]string         `yaml:"labels"`
	TagsAsLabels      *bool                     `yaml:"tagsAsLabels"`
	DependsOn         []ContainerDependsOn      `yaml:"dependsOn"`
	MountPoints       []ContainerMountPoints    `yaml:"mountPoints"`
	Logging           ContainerLogging          `yaml:"logConfiguration"`
	LinuxParameters   *ContainerLinuxParameters `yaml:"linuxParameters"`
	Command           []string                  `yaml:"command"` // The command that is passed to the container. If there are multiple arguments, each argument should be a separated string in the array.
	Ulimit            []ContainerUlimit         `yaml:"ulimits"`
	Essential         *bool                     `yaml:"essential"` // true or nil => essential, false to explicity set essential status of a container to false.
	Privileged        *bool                     `yaml:"privileged"`
	FirelensConfig    *FirelensConfiguration    `yaml:"firelensConfiguration"`
	WorkingDirectory  *string                   `yaml:"workingDir"`
}

type ContainerDependsOn struct {
	Condition string `yaml:"condition"` // START | COMPLETE | SUCCESS | HEALTHY
	Container string `yaml:"container"`
}

// ContainerUlimit represents ulimit settings to be passed to a container
type ContainerUlimit struct {
	Name      string `yaml:"name"`
	SoftLimit int32  `yaml:"softLimit"`
	HardLimit int32  `yaml:"hardLimit"`
}

// ContainerVolumes represents the volumes to be made available to the containers
type ContainerVolumes struct {
	Name                       string  `yaml:"name"`
	Type                       *string `yaml:"type"`                       // host (defult) | efs | docker (not supported) | fsx (not supported)
	SourcePath                 *string `yaml:"sourcePath"`                 // hot bind path
	EFSFilesSystemID           *string `yaml:"efsFilesystemId"`            // efs filesystem ID (fs-*) or buildit efs-filesystem resource name
	EFSAccessPointID           *string `yaml:"efsAccessPointId"`           // efs access-point ID (fsap-*) or access point name (optional, also requires SourcePath = '/')
	EFSIAMAuthorizationEnabled *bool   `yaml:"efsIAMAuthorizationEnabled"` // true/yes=ENABLED, false/no=DISABLED
	EFSRootDirectory           *string `yaml:"efsRootDir"`                 // root directory for the efs filesystem to mount; for access-points must be '/'
	// TODO docker volume mount configurations are not supported yet
	// Only bind-mounts & EFS volumes are supported
}

// TaskDef represents a service Task Definition
type TaskDef struct {
	BaseResource `yaml:",inline"`
	Name                    string              `yaml:"name"`
	Role                    string              `yaml:"role"`
	ExecutionRole           string              `yaml:"executionRole"`
	NetworkMode             string              `yaml:"networkMode"`             // bridge, host, awsvpc, none
	PidMode                 *string             `yaml:"pidMode"`                 // host, task
	IpcMode                 *string             `yaml:"ipcMode"`                 // host, task, none (Default: docker daemon set value)
	RequiresCompatibilities []string            `yaml:"requiresCompatibilities"` // EC2, FARGATE, EXTERNAL defaults to EC2
	TaskMemory              string              `yaml:"taskMemory"`              // Task Memory in MiB or as string with units (1024 or 1)
	TaskCPU                 string              `yaml:"taskCPU"`                 // Task CPU in units or vCPUs (256 or .25 vCPU, 1 vcpu)
	EphemeralStorage        *int32              `yaml:"ephemeralStorage"`        // Epehemeral storage to allow the task
	RuntimePlatform         *RuntimePlatform    `yaml:"runtimePlatform"`         // Runtime platform for the task
	Containers              []*Container        `yaml:"containers"`
	Volumes                 []*ContainerVolumes `yaml:"volumes"`
	DependsOn               []Key               `yaml:"dependsOn"`
	Tags                    map[string]string   `yaml:"tags"`
	GlobalTags              map[string]string   `yaml:"-"`
}

// Key returns the unique key for the resource for this buildit context
func (t TaskDef) Key() Key {
	return NewKey(t.Context.ProviderName, t.Identifier())
}

// Identifier returns the unique ID
func (t TaskDef) Identifier() string {
	return t.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (t *TaskDef) Normalize(ctx context.Context) {
	// taskExecutionRole can be defaulted to "ecsTaskExecutionRole"
	if t.ExecutionRole == "" {
		t.ExecutionRole = taskExecutionRoleDefault
	}

	// set default network mode to awsvpc or switch supplied values for networkMode
	if t.NetworkMode == "" {
		t.NetworkMode = "awsvpc"
	} else {
		t.NetworkMode = strings.ToLower(t.NetworkMode)
	}

	// switch supplied values for pidMode & ipcMode to lowercase
	if t.PidMode != nil {
		t.PidMode = aws.String(strings.ToLower(*t.PidMode))
	}

	if t.IpcMode != nil {
		t.IpcMode = aws.String(strings.ToLower(*t.IpcMode))
	}

	// set defaults for compatibilities if not supplied, else set supplied values to uppercase.
	if len(t.RequiresCompatibilities) == 0 {
		t.RequiresCompatibilities = append(t.RequiresCompatibilities, "FARGATE")
	} else {
		for n, c := range t.RequiresCompatibilities {
			t.RequiresCompatibilities[n] = strings.ToUpper(c)
		}
	}

	if t.RuntimePlatform != nil {
		if t.RuntimePlatform.CpuArchitecture == "" && t.RuntimePlatform.OperatingSystemFamily == "" {
			t.RuntimePlatform = nil
		} else {
			t.RuntimePlatform.CpuArchitecture = strings.ToUpper(t.RuntimePlatform.CpuArchitecture)
			t.RuntimePlatform.OperatingSystemFamily = strings.ToUpper(t.RuntimePlatform.OperatingSystemFamily)
		}
	}

	// volumes
	for _, vol := range t.Volumes {
		if vol.Type == nil {
			vol.Type = aws.String(HostBindMount)
		}

		if *vol.Type == EFSVolume && vol.EFSRootDirectory == nil {
			vol.EFSRootDirectory = aws.String("/") // default root path for an efs type volume is `/` (root of fs)
		}

		if *vol.Type == EFSVolume && vol.EFSIAMAuthorizationEnabled == nil {
			vol.EFSIAMAuthorizationEnabled = aws.Bool(false)
		}
	}

	// task memory and cpu, default to smallest values that allows both Linux and Windows on Fargate
	if t.TaskMemory == "" {
		t.TaskMemory = "2048" // default to 2048 MiB
	}
	if t.TaskCPU == "" {
		t.TaskCPU = "1024" // default to 1024 CPU units (1 vCPU)
	}

	// Merge globalTags to taskDef tags, if key is not already present
	// later we'll use t.Tags to add/update tags
	if t.Tags == nil {
		t.Tags = make(map[string]string)
	}
	ResourceTags(t.Tags).Merge(t.GlobalTags)

	// container by container noralizations...
	for _, cont := range t.Containers {

		// cpu
		if cont.CPU == nil {
			cont.CPU = aws.Int32(0) // set 0 when not supplied..
		}
		// if essential is nil, set it to true
		if cont.Essential == nil {
			cont.Essential = aws.Bool(true)
		}

		// if tagsAsLabels is nil, set it to true
		if cont.TagsAsLabels == nil {
			cont.TagsAsLabels = aws.Bool(true)
		}

		// if tagsAsLabels is true, user all tag key/values as default for labels
		// override any key/values defined in cont.Labels...
		if *cont.TagsAsLabels {

			if cont.Labels == nil {
				cont.Labels = make(map[string]string)
			}

			for k, v := range t.Tags {

				// buildit's own tags are internal metadata, not container config. Projecting
				// them would make adding a built-in tag a container change, forcing a new task
				// definition revision and a rolling redeploy of every service using it.
				// Keep this above the rewrite below, which strips the prefix off k.
				if util.IsBuilditTagKey(k) {
					continue
				}

				// replace ':' with '-' since this is not a legal char in docker labels
				k = strings.ReplaceAll(k, ":", "-")

				// if k doesn't exist in cont.Labels add k,v to it...
				if _, ok := cont.Labels[k]; !ok {
					cont.Labels[k] = v
				}
			}
		}

		// firelens config
		if cont.FirelensConfig != nil && cont.FirelensConfig.Type == nil {
			cont.FirelensConfig.Type = aws.String(string(ecstypes.FirelensConfigurationTypeFluentbit)) // default to be used
		}
	}
}

// Validate checks that the Task Def has a valid configuration.
// If the configuration is invalid an error will be returned
// that contains details on what was invalid.
func (t TaskDef) Validate(ctx context.Context) error {
	var errMessages []string

	if len(t.Containers) == 0 {
		errMessages = append(errMessages, "error creating taskdef, no containers provided")
	}

	for _, c := range t.Containers {

		if c.FirelensConfig != nil && c.FirelensConfig.Type != nil {
			if *c.FirelensConfig.Type != "fluentbit" {
				errMessages = append(errMessages, fmt.Sprintf(
					"invalid type supplied for firelens config, %v is not allowed; only fluentbit supported", *c.FirelensConfig.Type))
			}
		}

		if len(c.Ulimit) > 0 {
			for _, ulimit := range c.Ulimit {
				switch strings.ToLower(ulimit.Name) {
				case "cpu", "data", "fsize", "locks", "memlock",
					"msgqueue", "nice", "nofile", "nproc", "rss",
					"rtprio", "rttime", "sigpending", "stack":
					// no-op
				default:
					errMessages = append(errMessages, fmt.Sprintf("invalid ulimit name, %v is not allowed", ulimit.Name))
				}
			}
		}
	}

	// Make sure any obvious required properties are set
	switch t.NetworkMode {
	case string(ecstypes.NetworkModeBridge), string(ecstypes.NetworkModeHost),
		string(ecstypes.NetworkModeAwsvpc), string(ecstypes.NetworkModeNone):
	default:
		msg := fmt.Sprintf(`invalid network mode %q, must be "bridge", "host", "awsvpc", or "none"`, t.NetworkMode)
		errMessages = append(errMessages, msg)
	}

	// PidMode
	if t.PidMode != nil {
		switch *t.PidMode {
		case string(ecstypes.PidModeHost), string(ecstypes.PidModeTask):
		default:
			msg := fmt.Sprintf(`invalid pid mode %q, must be "%v", "%v" or not specified`,
				*t.PidMode, ecstypes.PidModeHost, ecstypes.PidModeTask)
			errMessages = append(errMessages, msg)
		}
	}

	// IpcMode
	if t.IpcMode != nil {
		switch *t.IpcMode {
		case string(ecstypes.IpcModeHost), string(ecstypes.IpcModeTask), string(ecstypes.IpcModeNone):
		default:
			msg := fmt.Sprintf(`invalid ipc mode %q, must be "%v", "%v", "%v" or not specified`,
				*t.IpcMode, ecstypes.IpcModeHost, ecstypes.IpcModeTask, ecstypes.IpcModeNone)
			errMessages = append(errMessages, msg)
		}
	}

	if t.EphemeralStorage != nil {
		if *t.EphemeralStorage < 21 || *t.EphemeralStorage > 200 {
			errMessages = append(errMessages, "ephemeral storage must be between 21 & 200")
		}
	}

	if t.RuntimePlatform != nil {
		if t.RuntimePlatform.CpuArchitecture == "" {
			errMessages = append(errMessages, "cpu architecture must be specified when runtime platform is set")
		} else {
			switch t.RuntimePlatform.CpuArchitecture {
			case string(ecstypes.CPUArchitectureX8664), string(ecstypes.CPUArchitectureArm64):
			default:
				errMessages = append(errMessages, fmt.Sprintf("invalid cpu architecture %q, must be X86_64 or ARM64", t.RuntimePlatform.CpuArchitecture))
			}
		}

		if t.RuntimePlatform.OperatingSystemFamily == "" {
			errMessages = append(errMessages, "operating system family must be specified when runtime platform is set")
		} else {
			switch t.RuntimePlatform.OperatingSystemFamily {
			case string(ecstypes.OSFamilyLinux),
				string(ecstypes.OSFamilyWindowsServer2019Full),
				string(ecstypes.OSFamilyWindowsServer2019Core),
				string(ecstypes.OSFamilyWindowsServer2022Full),
				string(ecstypes.OSFamilyWindowsServer2022Core):
			default:
				errMessages = append(errMessages, fmt.Sprintf("invalid operating system family %q, must be one of: LINUX, WINDOWS_SERVER_2019_FULL, WINDOWS_SERVER_2019_CORE, WINDOWS_SERVER_2022_FULL, WINDOWS_SERVER_2022_CORE", t.RuntimePlatform.OperatingSystemFamily))
			}
		}
	}

	// Tags cannot be empty, so make sure necessary global tags are set
	// TODO(@maintainer): Should we add a way to configure task def specific tags?
	if len(t.Tags) == 0 {
		errMessages = append(errMessages, "task def requires tags, please set necessary global tags")
	}

	if len(t.RequiresCompatibilities) > 0 {
		for _, c := range t.RequiresCompatibilities {
			if c != string(ecstypes.CompatibilityEc2) && c != string(ecstypes.CompatibilityFargate) && c != string(ecstypes.CompatibilityExternal) {
				errMessages = append(errMessages, "task def compatibilities can only be EC2, FARGATE or EXTERNAL")
			}
		}
	}

	// volumes
	for _, vol := range t.Volumes {
		switch *vol.Type {
		case HostBindMount:
			// for host (bindmount) volumes, source path is required
		case EFSVolume:
			// efs filesystem id is required for efs volumes
			if vol.EFSFilesSystemID == nil {
				errMessages = append(errMessages, fmt.Sprintf("for volume %v of type %v, 'efsFilesystemId' must be specified", vol.Name, *vol.Type))
			}
			// efs root directory is required for efs type volumes (this condition should never occur, normalize sets the default path to /)
			if vol.EFSRootDirectory == nil {
				errMessages = append(errMessages, fmt.Sprintf("for volume %v of type %v, 'efsRootDir' must be specified", vol.Name, *vol.Type))
			}
			// when an efs access point is used, the efs root directory mus be /
			if vol.EFSAccessPointID != nil && (vol.EFSRootDirectory != nil && *vol.EFSRootDirectory != "/") {
				errMessages = append(errMessages, fmt.Sprintf("for volume %v of type %v, efsAccessPointId is supplied, but the efsRootDir is not equal to '/' (root)", vol.Name, *vol.Type))
			}
		}
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: t.Identifier(),
		ResourceType:       "Task Def",
		Messages:           errMessages,
	}
}

// resolveEFSVolumeIDs resolves efsFilesystemId / efsAccessPointId values that reference buildit
// resource names into their physical EFS IDs. Physical IDs (fs-* / fsap-*) pass through
// untouched, so resolution is idempotent and only calls AWS for name references. Volumes are
// pointers, so the resolved IDs persist for the rest of the resource lifecycle.
func (t TaskDef) resolveEFSVolumeIDs(ctx context.Context) error {
	var efsClient *awsw.EFS
	for _, vol := range t.Volumes {
		if vol.Type == nil || *vol.Type != EFSVolume {
			continue
		}
		if efsClient == nil {
			c := awsw.NewEFS(ctx, t.Context.ProviderName)
			efsClient = &c
		}

		fsID, apID, err := efsClient.ResolveVolumeIDs(ctx, vol.EFSFilesSystemID, vol.EFSAccessPointID)
		if err != nil {
			return errors.Wrapf(err, "volume %v", vol.Name)
		}
		vol.EFSFilesSystemID = fsID
		vol.EFSAccessPointID = apID
	}
	return nil
}

// Apply creates the given task definition. If a task definition with the given name already exists
// and it is not equal to the current task def a new revision of the task def will be created.
func (t TaskDef) Apply(ctx context.Context) error {
	log.Debugf("creating taskdef %v", t.Identifier())

	diffs, err := t.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": t.Identifier(),
		}).Info("no updates required")
		return nil
	}

	// if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", t.Identifier()).Info("taskdef already exists, creating a new revision")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = t.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to create a new revision for taskdef %v", t.Identifier())
		}
		return nil
	}

	return t.apply(ctx)
}

// Destroy will deregister all revisions of the task def and set them to INACTIVE.
func (t TaskDef) Destroy(ctx context.Context) error {
	log.Debugf("deregistering taskdef %v", t.Identifier())

	existing, err := t.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding target group %v", t.Name)
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": t.Identifier(),
		}).Info("taskdef does not exist, nothing to destroy, skippping ")
		return nil
	}

	// Task defs can't be deleted, however they can be deregistered which sets them to INACTIVE

	// TODO(@maintainer): is there a way to check if a task def is in use before deregistering?
	ecsClient := client.ECS(ctx, t.Context.ProviderName)

	log.WithField("Name", t.Identifier()).Info("Checking if task def exists")

	var taskDefArns []string
	var nextToken *string
loop:
	for {
		respListTaskDefs, err := ecsClient.ListTaskDefinitions(ctx, &ecs.ListTaskDefinitionsInput{
			NextToken:    nextToken,
			FamilyPrefix: aws.String(t.Name),
		})
		if err != nil {
			return errors.Wrap(err, "error listing task defs")
		}

		taskDefArns = append(taskDefArns, respListTaskDefs.TaskDefinitionArns...)

		if respListTaskDefs.NextToken == nil {
			break loop
		}
		nextToken = respListTaskDefs.NextToken
	}

	if taskDefArns == nil {
		log.WithField("Name", t.Identifier()).Info("Task def does not exist, nothing to destroy")
		return nil
	}

	log.WithField("Name", t.Identifier()).Info("Deregistering all revisions of task def")
	for _, td := range taskDefArns {
		_, err := ecsClient.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{
			TaskDefinition: aws.String(td),
		})
		if err != nil {
			return errors.Wrapf(err, "failed to deregister task def %s", td)
		}
	}

	log.WithFields(log.Fields{
		"Name": t.Identifier(),
		"ARN":  taskDefArns[len(taskDefArns)-1],
	}).Info(color.Red("all revisions of ecs taskdef deregistered"))
	return nil
}

// TaskDefDiff type captures diffs between this & AWS resource
type TaskDefDiff struct {
	BaseResourceDiff

	diff     bool
	tagsDiff bool
	tagDiff  util.TagDiffResult
}

// Compare fetches the existing TaskDef & if it exists, checks if this
// is equal to the AWS resource
func (t TaskDef) Compare(ctx context.Context) (ResourceDiff, error) {
	// resolve any buildit resource name references in efs volumes to their physical IDs so the
	// comparison below is against what AWS stores
	if err := t.resolveEFSVolumeIDs(ctx); err != nil {
		return nil, err
	}

	existing, err := t.fetchExisting(ctx)
	if err != nil {
		return nil, err
	}

	// var diff bool
	diffs := &TaskDefDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "taskdef does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	// requires compatibilities
	var awsTaskdefRequiresCompatibilities []string
	for _, c := range existing.RequiresCompatibilities {
		awsTaskdefRequiresCompatibilities = append(awsTaskdefRequiresCompatibilities, string(c))
	}
	if util.DiffStringSlices(awsTaskdefRequiresCompatibilities, t.RequiresCompatibilities) {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "requires compatibilities are not the same")
	}

	// task iam role
	if t.Role == "" {
		if existing.TaskRoleArn != nil {
			diffs.diff = true
			diffs.Messages = append(diffs.Messages, "task role will be updated %v -> %v", *existing.TaskDefinitionArn, t.Role)
		}
	} else {
		taskRoleArn, err := awsw.NewIAM(ctx, t.Context.ProviderName).RoleArnForName(ctx, t.Role)
		if err != nil {
			return nil, err
		}

		if *taskRoleArn != *existing.TaskRoleArn {
			diffs.diff = true
			diffs.Messages = append(diffs.Messages, "task role will be updated %v -> %v", *existing.TaskRoleArn, *taskRoleArn)
		}
	}

	// task execution role
	taskExecutionRole, err := awsw.NewIAM(ctx, t.Context.ProviderName).RoleArnForName(ctx, t.ExecutionRole)
	if err != nil {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "task execution role is not the same")
	}

	if *taskExecutionRole != *existing.ExecutionRoleArn {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "task execution role iwill be updated %v -> %v", *existing.ExecutionRoleArn, *taskExecutionRole)
	}

	// network mode
	if t.NetworkMode != string(existing.NetworkMode) {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "network mode is not the same")
	}

	// pid mode
	var t_PidMode string
	if t.PidMode != nil {
		t_PidMode = *t.PidMode
	}
	if t_PidMode != string(existing.PidMode) {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "pid mode is not the same")
	}

	// ipc mode
	var t_IpcMode string
	if t.IpcMode != nil {
		t_IpcMode = *t.IpcMode
	}
	if t_IpcMode != string(existing.IpcMode) {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "ipc mode is not the same")
	}

	// task memory
	if t.TaskMemory != *existing.Memory {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "task memory is not the same")
	}

	// task cpu
	if t.TaskCPU != *existing.Cpu {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "task cpu is not the same")
	}

	// ephemeral storage
	definedEphemeralStorage := util.CoalesceInt32(t.EphemeralStorage, 20)
	existingEhemeralStorage := int32(20)
	if existing.EphemeralStorage != nil {
		existingEhemeralStorage = existing.EphemeralStorage.SizeInGiB
	}

	if definedEphemeralStorage != existingEhemeralStorage {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("ephemeral storage is not the same, existing %v, defined %v", existingEhemeralStorage, definedEphemeralStorage))
	}

	// runtime platform
	if t.RuntimePlatform != nil {
		if existing.RuntimePlatform == nil {
			diffs.diff = true
			diffs.Messages = append(diffs.Messages, "runtime platform will be added")
		} else {
			if t.RuntimePlatform.CpuArchitecture != string(existing.RuntimePlatform.CpuArchitecture) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("runtime platform cpu architecture is not the same, existing %v, defined %v", existing.RuntimePlatform.CpuArchitecture, t.RuntimePlatform.CpuArchitecture))
			}
			if t.RuntimePlatform.OperatingSystemFamily != string(existing.RuntimePlatform.OperatingSystemFamily) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("runtime platform operating system family is not the same, existing %v, defined %v", existing.RuntimePlatform.OperatingSystemFamily, t.RuntimePlatform.OperatingSystemFamily))
			}
		}
	} else if existing.RuntimePlatform != nil {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "runtime platform will be removed")
	}

	var containersDiff bool
	if len(t.Containers) != len(existing.ContainerDefinitions) {
		containersDiff = true
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "task containers are not the same")
	}

	if !containersDiff {
		// Identify the containers by name
		existingContainers := make(map[string]ecstypes.ContainerDefinition)
		for _, c := range existing.ContainerDefinitions {
			existingContainers[*c.Name] = c
		}

		for _, container := range t.Containers {
			awsContainer, ok := existingContainers[container.Name]

			if !ok {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("container '%v' will be added to taskdef", container.Name))
				continue // new container, no diffs needed
			}

			delete(existingContainers, container.Name)

			if container.Image != *awsContainer.Image {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("image tag for container '%v' does not match existing= %v & new=%v", container.Name, color.Yellow(*awsContainer.Image), color.Yellow(container.Image)))
			}

			if util.CoalesceInt32(container.CPU, 0) != util.CoalesceInt32(&awsContainer.Cpu, 0) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("cpu for container '%v' does not match", container.Name))
			}

			if util.CoalesceInt32(container.Memory, 0) != util.CoalesceInt32(awsContainer.Memory, 0) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("memory for container '%v' does not match", container.Name))
			}

			if util.CoalesceInt32(container.MemoryReservation, 0) != util.CoalesceInt32(awsContainer.MemoryReservation, 0) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("memory reservation for container '%v' does not match", container.Name))
			}

			if len(container.PortMappings) != len(awsContainer.PortMappings) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("port mappings for container '%v' do not match", container.Name))
			}

			existingPortMappings := make(map[int32]ecstypes.PortMapping)
			for _, pm := range awsContainer.PortMappings {
				existingPortMappings[*pm.ContainerPort] = pm
			}

			for _, pm := range container.PortMappings {
				awsPM, ok := existingPortMappings[pm.ContainerPort]
				if !ok {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("port mappings for container '%v' do not match", container.Name))
				} else {
					if pm.HostPort != *awsPM.HostPort {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("port mappings (host port) for container '%v' do not match", container.Name))
					}

					if pm.Protocol != string(awsPM.Protocol) {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("port mappings (protocol) for container '%v' do not match", container.Name))
					}

					if pm.Name != util.Coalesce(awsPM.Name, "") {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("port mappings (name) for container '%v' do not match %q -> %q", container.Name, util.Coalesce(awsPM.Name, ""), pm.Name))
					}
				}
			}

			// Healthcheck is optional
			if awsContainer.HealthCheck == nil {
				if container.Healthcheck.Command != nil {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("healthcheck commands for container '%v' added", container.Name))
				}
			} else {
				if len(container.Healthcheck.Command) != len(awsContainer.HealthCheck.Command) {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("number of healthcheck commands for container '%v' do not match", container.Name))

				}

				// Order matters for commands so this is easy
				for i, cmd := range container.Healthcheck.Command {
					if cmd != awsContainer.HealthCheck.Command[i] {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("healthcheck command # %v for container '%v' do not match, existing=%v, new=%v", i, container.Name, color.Yellow(awsContainer.HealthCheck.Command[i]), color.Yellow(cmd)))
					}
				}

				if container.Healthcheck.Interval != *awsContainer.HealthCheck.Interval {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("healthcheck interval for container '%v' does not match, existing=%v, new=%v", container.Name, *awsContainer.HealthCheck.Interval, container.Healthcheck.Interval))

				}

				if container.Healthcheck.Timeout != *awsContainer.HealthCheck.Timeout {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("healthcheck timeout for container '%v' does not match, existing=%v, new=%v", container.Name, *awsContainer.HealthCheck.Timeout, container.Healthcheck.Timeout))

				}

				if container.Healthcheck.StartPeriod != *awsContainer.HealthCheck.StartPeriod {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("healthcheck start period for container '%v' does not match, existing=%v, new=%v", container.Name, *awsContainer.HealthCheck.StartPeriod, container.Healthcheck.StartPeriod))
				}

				if container.Healthcheck.Retries != *awsContainer.HealthCheck.Retries {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("healthcheck retries for container '%v' do not match, , existing=%v, new=%v", container.Name, *awsContainer.HealthCheck.Retries, container.Healthcheck.Retries))
				}
			}

			// env vars
			if len(container.EnvVars) != len(awsContainer.Environment) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("number of environment variables defined for container '%v' do not match", container.Name))
			}

			for _, awsEnvVar := range awsContainer.Environment {
				v, ok := container.EnvVars[*awsEnvVar.Name]
				if !ok {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("environment variable '%v' to be deleted for container %v", *awsEnvVar.Name, container.Name))
				} else if v != *awsEnvVar.Value {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages,
						fmt.Sprintf("value of environment variable '%v' defined for container '%v' do not match %v -> %v",
							*awsEnvVar.Name, container.Name, *awsEnvVar.Value, v))
				}
			}

			// secrets
			if len(container.Secrets) != len(awsContainer.Secrets) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("number of secrets defined for container '%v' do not match", container.Name))
			}

			for _, awsSecret := range awsContainer.Secrets {
				secret, ok := container.Secrets[*awsSecret.Name]
				if !ok {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("secret value '%v' to be deleted for container '%v'", *awsSecret.Name, container.Name))
				} else {
					secretArn, err := secretArnFromName(ctx, t.Context, secret)
					if err != nil {
						return nil, err
					}

					if secretArn != *awsSecret.ValueFrom {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("secret arn for '%v' defined for container '%v' do not match", *awsSecret.Name, container.Name))
					}
				}
			}

			// firelens config
			if container.FirelensConfig != nil && awsContainer.FirelensConfiguration == nil ||
				container.FirelensConfig == nil && awsContainer.FirelensConfiguration != nil {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("aws firelens configuration for container '%v' do not match", container.Name))
			} else {
				if container.FirelensConfig != nil && awsContainer.FirelensConfiguration != nil {
					var container_FirelensConfig_Type string
					if container.FirelensConfig.Type != nil {
						container_FirelensConfig_Type = *container.FirelensConfig.Type
					}
					if container_FirelensConfig_Type != string(awsContainer.FirelensConfiguration.Type) {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("aws firelens log router type for container '%v' does not match, existing=%v, new=%v", container.Name, string(awsContainer.FirelensConfiguration.Type), container_FirelensConfig_Type))
					}

					// match options now...
					if len(container.FirelensConfig.Options) != len(awsContainer.FirelensConfiguration.Options) {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("aws firelens log router options for container '%v' do not match", container.Name))
					}
					for k, awsV := range awsContainer.FirelensConfiguration.Options {
						// key doesn't exist...
						cV, ok := container.FirelensConfig.Options[k]
						if !ok {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("aws firelens log router option '%v' for container '%v' will be removed", k, container.Name))
						} else if cV != awsV {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("value for aws firelens log router option '%v' for container '%v' do not match, existing=%v, new=%v", k, container.Name, color.Yellow(awsV), color.Yellow(cV)))
						}
					}
				}
			}

			// labels
			if len(container.Labels) != len(awsContainer.DockerLabels) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("labels for container '%v' do not match", container.Name))
			}

			for label, value := range container.Labels {
				awsValue, ok := awsContainer.DockerLabels[label]
				if !ok {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("label '%v' for container '%v' will be added", color.Yellow(label), container.Name))
				} else if value != awsValue {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("value for label '%v' for container '%v' do not match", color.Yellow(label), container.Name))
				}
			}

			// entrypoint
			if len(container.Entrypoint) != len(awsContainer.EntryPoint) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("entrypoints for container '%v' do not match", container.Name))
			} else {
				for i, e := range container.Entrypoint {
					if e != awsContainer.EntryPoint[i] {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("entrypoints for container '%v' do not match", container.Name))
					}
				}
			}

			// command
			if len(container.Command) != len(awsContainer.Command) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("commands for container '%v' do not match", container.Name))
			} else {
				for i, cmd := range container.Command {
					if cmd != awsContainer.Command[i] {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("commands for container '%v' do not match", container.Name))
					}
				}
			}

			// container dependsOn
			if len(container.DependsOn) != len(awsContainer.DependsOn) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("container dependencies for '%v' do not match", container.Name))
			} else {
				existingDependencies := make(map[string]ecstypes.ContainerDependency)
				for _, d := range awsContainer.DependsOn {
					existingDependencies[*d.ContainerName] = d
				}
				for _, d := range container.DependsOn {
					if v, ok := existingDependencies[d.Container]; !ok {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("container dependencies for '%v' do not match", container.Name))
					} else {
						if v.Condition != ecstypes.ContainerCondition(strings.ToUpper(d.Condition)) {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("container dependencies for '%v' do not match", container.Name))
						}
					}
				}
			}

			if len(container.MountPoints) != len(awsContainer.MountPoints) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("mount points for container '%v' do not match", container.Name))
			} else {

				existingMountPoints := make(map[string]ecstypes.MountPoint)
				for _, m := range awsContainer.MountPoints {
					existingMountPoints[*m.ContainerPath] = m
				}

				for _, mp := range container.MountPoints {
					awsMP, ok := existingMountPoints[mp.ContainerPath]
					if !ok {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("mount points for container '%v' do not match", container.Name))
					} else {

						if mp.ReadOnly != *awsMP.ReadOnly {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("mount points for container '%v' do not match", container.Name))
						}

						if mp.SourceVolume != *awsMP.SourceVolume {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("mount point volume for container '%v' do not match", container.Name))
						}
					}
					delete(existingMountPoints, mp.ContainerPath)
				}

				// Should never happen, but just to be safe
				if len(existingMountPoints) > 0 {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("mount points for container '%v' do not match", container.Name))
				}
			}

			// linux parameters
			if container.LinuxParameters == nil && awsContainer.LinuxParameters != nil ||
				container.LinuxParameters != nil && awsContainer.LinuxParameters == nil {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("linux params for container '%v' do not match", container.Name))
			} else if container.LinuxParameters != nil && awsContainer.LinuxParameters != nil {
				// capabilities
				capabilities := container.LinuxParameters.Capabilities
				if len(capabilities.Add) > 0 || len(capabilities.Drop) > 0 {
					if awsContainer.LinuxParameters.Capabilities != nil {
						awsAdd := awsContainer.LinuxParameters.Capabilities.Add
						awsDrop := awsContainer.LinuxParameters.Capabilities.Drop

						if len(awsAdd) != len(capabilities.Add) {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("linux param capabilities_add for container '%v' do not match", container.Name))
						} else if util.DiffStringSlices(capabilities.Add, awsAdd) {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("linux param capabilities_add for container '%v' do not match", container.Name))
						}

						if len(awsDrop) != len(capabilities.Drop) {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("linux param capabilities_drop for container '%v' do not match", container.Name))
						} else if util.DiffStringSlices(capabilities.Drop, awsDrop) {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("linux param capabilities_drop for container '%v' do not match", container.Name))
						}
					} else {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("linux param capabilities for container '%v' do not match", container.Name))
					}
				}

				// init process enabled
				if util.CoalesceComparable(container.LinuxParameters.InitProcessEnabled, false) != util.CoalesceComparable(awsContainer.LinuxParameters.InitProcessEnabled, false) {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("linux param init_process_enabled for container '%v' do not match", container.Name))
				}
			}

			// Logging is optional
			if awsContainer.LogConfiguration == nil {
				if container.Logging.Driver != "" {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("log driver configuration for container '%v' does not match", container.Name))
				}
			} else {
				if container.Logging.Driver != string(awsContainer.LogConfiguration.LogDriver) {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("log driver configuration for container '%v' does not match", container.Name))
				}

				if len(container.Logging.Options) != len(awsContainer.LogConfiguration.Options) {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("log driver options for container '%v' do not match", container.Name))
				}

				for opt, val := range container.Logging.Options {
					awsVal, ok := awsContainer.LogConfiguration.Options[opt]
					if !ok {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("log configuration options for container '%v' do not match", container.Name))
					} else {
						if val != awsVal {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("log configuration options for container '%v' do not match %q -> %q", container.Name, awsVal, val))
						}
					}
				}

				if len(container.Logging.SecretOptions) != len(awsContainer.LogConfiguration.SecretOptions) {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("log configuration secret options for container '%v' do not match", container.Name))
				}

				for _, awsLogSecrets := range awsContainer.LogConfiguration.SecretOptions {
					logSecretVal, ok := container.Logging.SecretOptions[*awsLogSecrets.Name]
					if !ok {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("log configuration secret options for container '%v' do not match", container.Name))
					} else {

						secretArn, err := secretArnFromName(ctx, t.Context, logSecretVal)
						if err != nil {
							return nil, err
						}

						if secretArn != *awsLogSecrets.ValueFrom {
							diffs.diff = true
							diffs.Messages = append(diffs.Messages, fmt.Sprintf("log configuration secret options for container '%v' do not match", container.Name))
						}
					}
				}

			}

			// ulimits
			if len(container.Ulimit) != len(awsContainer.Ulimits) {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("ulimit definition for container '%v' does not match", container.Name))
			} else {
				ulimitsMap := make(map[string]ContainerUlimit)
				for _, limit := range container.Ulimit {
					ulimitsMap[limit.Name] = limit
				}
				for _, awsUlimit := range awsContainer.Ulimits {
					val, ok := ulimitsMap[string(awsUlimit.Name)]
					if !ok {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("ulimit name for container '%v' does not match", container.Name))
					}
					if val.SoftLimit != awsUlimit.SoftLimit ||
						val.HardLimit != awsUlimit.HardLimit {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, fmt.Sprintf("ulimit value for container '%v' does not match", container.Name))
					}
				}
			}

			// essential
			if container.Essential == nil && awsContainer.Essential != nil ||
				container.Essential != nil && awsContainer.Essential == nil {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("essential value for container '%v' does not match", container.Name))
			} else if container.Essential != nil && awsContainer.Essential != nil {
				if *container.Essential != *awsContainer.Essential {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("essential value for container '%v' does not match", container.Name))
				}
			}

			// privileged
			if container.Privileged == nil && awsContainer.Privileged != nil ||
				container.Privileged != nil && awsContainer.Privileged == nil {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("privilege value for container '%v' does not match", container.Name))
			} else if container.Privileged != nil && awsContainer.Privileged != nil {
				if *container.Privileged != *awsContainer.Privileged {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, fmt.Sprintf("privilege value for container '%v' does not match", container.Name))
				}
			}

			// working directory
			if util.Coalesce(container.WorkingDirectory, "") != util.Coalesce(awsContainer.WorkingDirectory, "") {
				diffs.diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("working directory for container '%v' does not match", container.Name))
			}

		} // for c: range containers

		// Should never happen but do this just to be safe
		if len(existingContainers) > 0 {
			diffs.diff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v containers to be removed", len(existingContainers)))
		}
	}

	if len(t.Volumes) != len(existing.Volumes) {
		diffs.diff = true
		diffs.Messages = append(diffs.Messages, "volume definition does not match")
	}

	existingVolumes := make(map[string]ecstypes.Volume)
	for _, v := range existing.Volumes {
		existingVolumes[*v.Name] = v
	}

	for _, volume := range t.Volumes {
		awsVolume, ok := existingVolumes[volume.Name]
		if !ok {
			diffs.diff = true
			diffs.Messages = append(diffs.Messages, "volume definition does not match")
		} else {

			if *volume.Type == HostBindMount {
				// specified bind-mount, existing is somethign else
				if awsVolume.Host == nil {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, "volume definition, host does not match")
				}
				// different source path found in existing
				if util.Coalesce(volume.SourcePath, "") != util.Coalesce(awsVolume.Host.SourcePath, "") {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, "volume definition, source path does not match")
				}
			}

			if *volume.Type == EFSVolume {
				// specified  efs volume, existing is something else
				if awsVolume.EfsVolumeConfiguration == nil {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, "efs volume configuration does not match")
				}

				// different efs filesystem ID or rootDirectory specified for efs
				if (*volume.EFSFilesSystemID != *awsVolume.EfsVolumeConfiguration.FileSystemId) ||
					(*volume.EFSRootDirectory != *awsVolume.EfsVolumeConfiguration.RootDirectory ||
						awsVolume.EfsVolumeConfiguration.TransitEncryption != ecstypes.EFSTransitEncryptionEnabled /*default used, not configurable*/) {
					diffs.diff = true
					diffs.Messages = append(diffs.Messages, "efs volume configuration does not match")
				}

				if volume.EFSAccessPointID != nil {
					if awsVolume.EfsVolumeConfiguration == nil || awsVolume.EfsVolumeConfiguration.AuthorizationConfig == nil ||
						awsVolume.EfsVolumeConfiguration.AuthorizationConfig.AccessPointId == nil || *volume.EFSAccessPointID != *awsVolume.EfsVolumeConfiguration.AuthorizationConfig.AccessPointId {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, "efs volume configuration (accesspoint id) does not match")
					}
				} else {
					if awsVolume.EfsVolumeConfiguration != nil && awsVolume.EfsVolumeConfiguration.AuthorizationConfig != nil && awsVolume.EfsVolumeConfiguration.AuthorizationConfig.AccessPointId != nil {
						diffs.diff = true
						diffs.Messages = append(diffs.Messages, "efs volume configuration (accesspoint id) does not match")
					}
				}
			}
		}
	}

	// tags
	awsTags, err := awsw.NewECS(ctx, t.Context.ProviderName).GetResourceTags(ctx, *existing.TaskDefinitionArn)
	if err != nil {
		return nil, err
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, t.Tags); tagDiff.HasChanges() {
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if diffs.diff || diffs.tagsDiff {
		return diffs, nil
	}

	// We made it!
	return nil, nil
}

// fetchExisting fetches the matching Task Def from AWS.
// If an error occurs, a non-nil error is returned. If the
// task def is not found both the task def and error will be nil.
func (t TaskDef) fetchExisting(ctx context.Context) (*ecstypes.TaskDefinition, error) {
	ecsClient := client.ECS(ctx, t.Context.ProviderName)

	// DescribeTaskDefinition lets you search for a task def by family name directly
	// Unfortunately if the family doesn't exist is does not return a meaningful error,
	// just a generic one. Therefore we use ListTaskDefinitonFamilies first to check if
	// a task def with the given name actually exists
	var nextToken *string
loop:
	for {
		respListTaskDefFamilies, err := ecsClient.ListTaskDefinitionFamilies(ctx, &ecs.ListTaskDefinitionFamiliesInput{
			NextToken:    nextToken,
			FamilyPrefix: aws.String(t.Name),
			// Only return task def families that have ACTIVE revisions. Otherwise DescribeTaskDefinition below
			// will fail. This allows us to treat a task def with all revisions inactive the same as one that doesn't exist.
			Status: ecstypes.TaskDefinitionFamilyStatusActive,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error listing task def families")
		}

		for _, family := range respListTaskDefFamilies.Families {
			if family == t.Name {
				break loop
			}
		}

		if respListTaskDefFamilies.NextToken == nil {
			return nil, nil
		}
		nextToken = respListTaskDefFamilies.NextToken
	}

	respDescribeTaskDef, err := ecsClient.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: aws.String(t.Name),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to look up task def %s", t.Identifier())
	}

	return respDescribeTaskDef.TaskDefinition, nil
}

// apply creates the given task definition. If a task definition with the given name already exists
// and it is not equal to the current task def a new revision of the task def will be created.
func (t TaskDef) apply(ctx context.Context) error {
	ecsClient := client.ECS(ctx, t.Context.ProviderName)

	// resolve any buildit resource name references in efs volumes to their physical IDs; a no-op
	// when Compare already resolved them (physical IDs pass through without AWS calls)
	if err := t.resolveEFSVolumeIDs(ctx); err != nil {
		return err
	}

	taskRoleArn, err := awsw.NewIAM(ctx, t.Context.ProviderName).RoleArnForName(ctx, t.Role)
	if err != nil {
		return errors.Wrap(err, "cannot lookup role arn for task IAM role specified")
	}

	taskExecutionRole, err := awsw.NewIAM(ctx, t.Context.ProviderName).RoleArnForName(ctx, t.ExecutionRole)
	if err != nil {
		return errors.Wrap(err, "cannot lookup role arn for task IAM execution role specified")
	}

	// Containers
	var containers []ecstypes.ContainerDefinition
	if len(t.Containers) == 0 {
		return errors.New("error creating taskdef, no containers provided")
	}
	for _, c := range t.Containers {

		// env Vars
		var keyValuePair []ecstypes.KeyValuePair = nil
		if len(c.EnvVars) != 0 {
			keyValuePair = make([]ecstypes.KeyValuePair, len(c.EnvVars))
			kvIndex := 0
			for k, v := range c.EnvVars {
				keyValuePair[kvIndex] = ecstypes.KeyValuePair{
					Name:  aws.String(k),
					Value: aws.String(v),
				}
				kvIndex++
			}
		}

		// secrets
		var secrets []ecstypes.Secret
		if len(c.Secrets) > 0 {
			for k, v := range c.Secrets {
				arn, err := secretArnFromName(ctx, t.Context, v)
				if err != nil {
					return errors.Wrapf(err, "error looking up secret %v", v)
				}
				secrets = append(secrets, ecstypes.Secret{
					Name:      aws.String(k),
					ValueFrom: aws.String(arn),
				})
			}
		}

		// healtcheck
		var healthcheck *ecstypes.HealthCheck = nil
		if c.Healthcheck.Command != nil { // make this better, use *healthcheck
			healthcheck = &ecstypes.HealthCheck{
				Command:     c.Healthcheck.Command,
				Interval:    aws.Int32(c.Healthcheck.Interval),
				Retries:     aws.Int32(c.Healthcheck.Retries),
				StartPeriod: aws.Int32(c.Healthcheck.StartPeriod),
				Timeout:     aws.Int32(c.Healthcheck.Timeout),
			}
		}

		// logging
		var logConfiguration *ecstypes.LogConfiguration = nil
		if c.Logging.Driver != "" { // TODO make this better, user *logging

			// log configuration secretOptions
			var secretOpts []ecstypes.Secret
			if len(c.Logging.SecretOptions) > 0 {
				for k, v := range c.Logging.SecretOptions {
					arn, err := secretArnFromName(ctx, t.Context, v)
					if err != nil {
						return errors.Wrapf(err, "error looking up log config secret options from %v", v)
					}
					secretOpts = append(secretOpts, ecstypes.Secret{
						Name:      aws.String(k),
						ValueFrom: aws.String(arn),
					})
				}
			}

			logConfiguration = &ecstypes.LogConfiguration{
				LogDriver:     ecstypes.LogDriver(c.Logging.Driver),
				Options:       c.Logging.Options,
				SecretOptions: secretOpts,
			}
		}

		// mounts
		var mountPoints []ecstypes.MountPoint
		for _, m := range c.MountPoints {
			mountPoints = append(mountPoints, ecstypes.MountPoint{
				ContainerPath: aws.String(m.ContainerPath),
				SourceVolume:  aws.String(m.SourceVolume),
				ReadOnly:      aws.Bool(m.ReadOnly),
			})
		}

		// volumeFrom
		// TODO

		// portMapping
		var portMappings []ecstypes.PortMapping
		for _, p := range c.PortMappings {
			var portMappingName *string
			if len(p.Name) != 0 {
				portMappingName = aws.String(p.Name)
			}
			portMappings = append(portMappings, ecstypes.PortMapping{
				ContainerPort: aws.Int32(p.ContainerPort),
				HostPort:      aws.Int32(p.HostPort),
				Protocol:      ecstypes.TransportProtocol(p.Protocol),
				Name:          portMappingName,
			})
		}

		// linux parameters
		var linuxParameters *ecstypes.LinuxParameters
		if c.LinuxParameters != nil {
			linuxParameters = &ecstypes.LinuxParameters{
				InitProcessEnabled: c.LinuxParameters.InitProcessEnabled,
			}
			if len(c.LinuxParameters.Capabilities.Add) > 0 ||
				len(c.LinuxParameters.Capabilities.Drop) > 0 {
				linuxParameters.Capabilities = &ecstypes.KernelCapabilities{
					Add:  c.LinuxParameters.Capabilities.Add,
					Drop: c.LinuxParameters.Capabilities.Drop,
				}
			}
		}

		// ulimits
		var ulimits []ecstypes.Ulimit
		for _, ulimit := range c.Ulimit {
			ulimits = append(ulimits, ecstypes.Ulimit{
				Name:      ecstypes.UlimitName(ulimit.Name),
				SoftLimit: ulimit.SoftLimit,
				HardLimit: ulimit.HardLimit,
			})
		}

		// TODO: this is needless check, since if essential = nil
		// ECS also treats it as "true"; however, here it is done
		// for backward compatibility with buildit where this value
		// was always hard-coded as `true`. Can be safely removed
		// when performing any major service overhauls..
		isEssential := c.Essential
		if isEssential == nil {
			isEssential = aws.Bool(true)
		}

		// firelens config
		var firelensConfig *ecstypes.FirelensConfiguration
		if c.FirelensConfig != nil {
			firelensConfig = &ecstypes.FirelensConfiguration{
				Type:    ecstypes.FirelensConfigurationType(*c.FirelensConfig.Type),
				Options: c.FirelensConfig.Options,
			}
		}

		// container dependency
		var dependencies []ecstypes.ContainerDependency
		for _, d := range c.DependsOn {
			dependencies = append(dependencies, ecstypes.ContainerDependency{
				ContainerName: aws.String(d.Container),
				Condition:     ecstypes.ContainerCondition(strings.ToUpper(d.Condition)),
			})
		}

		containers = append(containers, ecstypes.ContainerDefinition{
			Command:      c.Command,
			Cpu:          *c.CPU,
			DependsOn:    dependencies,
			DockerLabels: c.Labels,
			EntryPoint:   c.Entrypoint,
			Environment:  keyValuePair,
			Secrets:      secrets,
			Essential:    aws.Bool(*isEssential),
			HealthCheck:  healthcheck,
			Image:        aws.String(c.Image),
			// Interactive: aws.Bool(false),//TODO
			LogConfiguration:      logConfiguration,
			Memory:                c.Memory,
			MemoryReservation:     c.MemoryReservation,
			MountPoints:           mountPoints,
			Name:                  aws.String(c.Name),
			PortMappings:          portMappings,
			LinuxParameters:       linuxParameters,
			Ulimits:               ulimits,
			Privileged:            c.Privileged,
			FirelensConfiguration: firelensConfig,
			WorkingDirectory:      c.WorkingDirectory,
			// ResourceRequirements //TODO when gpu is needed
			// VolumesFrom: //TODO volumes from other containers
			// WorkingDirectory: aws.String("/")//TODO expose to yaml
		})
	}

	// Volumes
	var volumes []ecstypes.Volume
	for _, v := range t.Volumes {
		var hostProperties *ecstypes.HostVolumeProperties
		var efsConfig *ecstypes.EFSVolumeConfiguration
		if v.Type == nil || *v.Type == HostBindMount {
			hostProperties = &ecstypes.HostVolumeProperties{
				SourcePath: v.SourcePath,
			}
		} else if *v.Type == EFSVolume {
			var efsAuthorizationConfig *ecstypes.EFSAuthorizationConfig
			if v.EFSAccessPointID != nil || v.EFSIAMAuthorizationEnabled != nil {

				efsAuthorizationConfig = &ecstypes.EFSAuthorizationConfig{
					AccessPointId: v.EFSAccessPointID,
				}
				if v.EFSIAMAuthorizationEnabled != nil && *v.EFSIAMAuthorizationEnabled {
					efsAuthorizationConfig.Iam = ecstypes.EFSAuthorizationConfigIAMEnabled
				}
			}
			efsConfig = &ecstypes.EFSVolumeConfiguration{
				FileSystemId:        v.EFSFilesSystemID,
				RootDirectory:       v.EFSRootDirectory,
				TransitEncryption:   ecstypes.EFSTransitEncryptionEnabled,
				AuthorizationConfig: efsAuthorizationConfig,
			}
		}
		volumes = append(volumes, ecstypes.Volume{
			Name:                   aws.String(v.Name),
			Host:                   hostProperties,
			EfsVolumeConfiguration: efsConfig,
		})
	}

	// Requires Compatibilities.
	var compatibilities []ecstypes.Compatibility
	for _, c := range t.RequiresCompatibilities {
		compatibilities = append(compatibilities, ecstypes.Compatibility(c))
	}

	// Tags
	var tags []ecstypes.Tag
	for k, v := range t.Tags {
		tag := ecstypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}
		tags = append(tags, tag)
	}

	input := &ecs.RegisterTaskDefinitionInput{
		Family:                  aws.String(t.Name),
		ContainerDefinitions:    containers,
		Cpu:                     aws.String(t.TaskCPU),
		Memory:                  aws.String(t.TaskMemory),
		NetworkMode:             ecstypes.NetworkMode(t.NetworkMode),
		RequiresCompatibilities: compatibilities, // nil = EC2
		TaskRoleArn:             taskRoleArn,
		ExecutionRoleArn:        taskExecutionRole,
		Volumes:                 volumes,
		Tags:                    tags,
	}

	// pid mode
	if t.PidMode != nil {
		input.PidMode = ecstypes.PidMode(*t.PidMode)
	}

	// ipc mode
	if t.IpcMode != nil {
		input.IpcMode = ecstypes.IpcMode(*t.IpcMode)
	}

	// ephemeral storage
	if t.EphemeralStorage != nil {
		input.EphemeralStorage = &ecstypes.EphemeralStorage{
			SizeInGiB: *t.EphemeralStorage,
		}
	}

	if t.RuntimePlatform != nil {
		input.RuntimePlatform = &ecstypes.RuntimePlatform{
			CpuArchitecture:       ecstypes.CPUArchitecture(t.RuntimePlatform.CpuArchitecture),
			OperatingSystemFamily: ecstypes.OSFamily(t.RuntimePlatform.OperatingSystemFamily),
		}
	}

	respRegisterTaskDef, err := ecsClient.RegisterTaskDefinition(ctx, input)
	if err != nil {
		return errors.Wrap(err, "error registering task definition")
	}

	log.WithFields(log.Fields{
		"Family":   *respRegisterTaskDef.TaskDefinition.Family,
		"Revision": respRegisterTaskDef.TaskDefinition.Revision,
	}).Info(color.Green("taskdef revision created"))

	return nil
}

// applyDiffs would just call apply() for TaskDef since the logic for
// updating a taskDiff is to create a new revision for it, which is identical
// to create the first version
func (t TaskDef) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": t.Identifier(),
		}).Info("no updates required for taskdef")
		return nil
	}

	tdDiffs, ok := diffs.(*TaskDefDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	existing := tdDiffs.AWSResource()

	// case when only tags are different.
	if !tdDiffs.diff && tdDiffs.tagsDiff {

		arn := existing.(*ecstypes.TaskDefinition).TaskDefinitionArn

		upserts := tdDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			err := awsw.NewECS(ctx, t.Context.ProviderName).AddResourceTags(ctx, *arn, upserts)
			if err != nil {
				return err
			}
		}
		if len(tdDiffs.tagDiff.Deleted) > 0 {
			err := awsw.NewECS(ctx, t.Context.ProviderName).DeleteResourceTags(ctx, *arn, tdDiffs.tagDiff.Deleted)
			if err != nil {
				return err
			}
		}

		log.WithFields(log.Fields{
			"Family":   *existing.(*ecstypes.TaskDefinition).Family,
			"Revision": existing.(*ecstypes.TaskDefinition).Revision,
		}).Info(color.Yellow("taskdef tags updated"))
		return nil
	}

	return t.apply(ctx)
}
