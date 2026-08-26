package resource

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kafkaconnect"
	mskTypes "github.com/aws/aws-sdk-go-v2/service/kafkaconnect/types"

	log "github.com/sirupsen/logrus"
)

const (
	// CapacityType
	Autoscaling = "AUTOSCALING"
	Provisioned = "PROVISIONED"
)

// MSKConnector represents a msk connector
type MSKConnector struct {
	BaseResource `yaml:",inline"`
	Name                    string            `yaml:"-"`
	Description             *string           `yaml:"description"`
	Type                    *string           `yaml:"type"`
	Capacity                Capacity          `yaml:"capacity"`
	Secrets                 map[string]string `yaml:"secrets"`
	ConnectorConfiguration  map[string]string `yaml:"connectorConfiguration"`
	Cluster                 string            `yaml:"cluster"`
	KafkaConnectVersion     string            `yaml:"kafkaConnectVersion"`
	Plugin                  string            `yaml:"plugin"`
	WorkerConfiguration     *string           `yaml:"workerConfiguration"`
	Role                    string            `yaml:"role"`
	LogGroup                *string           `yaml:"logGroup"`
	TimeoutSeconds          int               `yaml:"deploymentTimeout"`
	DependsOn               []Key             `yaml:"dependsOn"`
	Tags                    map[string]string `yaml:"tags"`
	GlobalTags              map[string]string `yaml:"-"`
	_arn                    *string           `yaml:"-"` // read from AWS only
	_state                  *string           `yaml:"-"` // read from AWS only
	_pluginArn              *string           `yaml:"-"` // read from AWS only
	_workerConfigurationArn *string           `yaml:"-"` // read from AWS only
	_roleArn                *string           `yaml:"-"` // read from AWS only
	_currentVer             *string           `yaml:"-"` // read from AWS only
	_clusterInfo            ClusterInfo       `yaml:"-"` // read from AWS only
}

type Capacity struct {
	McuCount              int32  `yaml:"mcuCount"`
	WorkerCount           *int32 `yaml:"workerCount"`           // only for provisioned type
	MaxWorkerCount        *int32 `yaml:"maxWorkerCount"`        // only for autoScaling type
	MinWorkerCount        *int32 `yaml:"minWorkerCount"`        // only for autoScaling type
	ScaleInCpuPercentage  *int32 `yaml:"scaleInCpuPercentage"`  // only for autoScaling type
	ScaleOutCpuPercentage *int32 `yaml:"scaleOutCpuPercentage"` // only for autoScaling type
}

func (c Capacity) String() string {
	return fmt.Sprintf("Capacity{McuCount:%v, WorkerCount:%v, MaxWorkerCount:%v, MinWorkerCount:%v, ScaleInCpuPercentage:%v, ScaleOutCpuPercentage:%v}",
		c.McuCount,
		util.CoalesceInt32(c.WorkerCount, 0),
		util.CoalesceInt32(c.MaxWorkerCount, 0),
		util.CoalesceInt32(c.MinWorkerCount, 0),
		util.CoalesceInt32(c.ScaleInCpuPercentage, 0),
		util.CoalesceInt32(c.ScaleOutCpuPercentage, 0),
	)
}

type ClusterInfo struct {
	BootstrapServers        *string                                       // read from AWS only
	SecurityGroups          []string                                      // read from AWS only
	Subnets                 []string                                      // read from AWS only
	ClientAuthType          mskTypes.KafkaClusterClientAuthenticationType // read from AWS only
	EncryptionInTransitType mskTypes.KafkaClusterEncryptionInTransitType  // read from AWS only
}

// Key returns the unique key for the resource for this buildit context
func (c MSKConnector) Key() Key {
	return NewKey(c.Context.ProviderName, c.Identifier())
}

// Identifier returns topic and endpoint name of the c
func (c MSKConnector) Identifier() string {
	return c.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (c *MSKConnector) Normalize(ctx context.Context) {
	// tags
	if c.Tags == nil {
		c.Tags = make(map[string]string)
	}
	ResourceTags(c.Tags).Merge(c.GlobalTags)

	// capacity type
	if c.Type == nil {
		c.Type = aws.String(Autoscaling)
	}
	c.Type = aws.String(strings.ToUpper(*c.Type))

	// secrets
	for k, v := range c.Secrets {
		val, err := awsw.NewSecretsManager(ctx, c.Context.ProviderName).GetValueBySecretId(ctx, v)
		if err != nil {
			log.Debugf("Provider: %v", c.Context.ProviderName)
			log.Debugf("Secret Id: %v-> %v", k, v)
			log.Debugf("error occurred %v", err.Error())
			panic(err)
		}
		c.Secrets[k] = *val
	}

	// merge secrets into config
	if c.ConnectorConfiguration == nil {
		c.ConnectorConfiguration = make(map[string]string)
	}
	for k, v := range c.Secrets {
		c.ConnectorConfiguration[k] = v
	}

	// timeout
	if c.TimeoutSeconds < 1 {
		c.TimeoutSeconds = 900 // default to 15min
	}

	// set resource arns
	if c.WorkerConfiguration != nil {
		if err := c.setWorkerConfigurationArn(ctx); err != nil {
			panic(err)
		}
	}
	if err := c.setPluginArn(ctx); err != nil {
		panic(err)
	}
	if err := c.setClusterInfo(ctx); err != nil {
		panic(err)
	}
	roleArn, err := awsw.NewIAM(ctx, c.Context.ProviderName).RoleArnForName(ctx, c.Role)
	if err != nil {
		panic(err)
	}
	c._roleArn = roleArn

	if c.Type != nil && *c.Type == Autoscaling {
		c.Capacity.WorkerCount = nil
	}
}

// Validate checks that the input provided is correct
func (c MSKConnector) Validate(ctx context.Context) error {
	var errorMsgs []string

	if c.Cluster == "" {
		errorMsgs = append(errorMsgs, "cluster is required")
	}

	if c.Capacity.McuCount == 0 {
		errorMsgs = append(errorMsgs, "mcuCount is required")
	}

	if c.Plugin == "" {
		errorMsgs = append(errorMsgs, "plugin is required")
	}

	switch *c.Type {
	case Autoscaling:
		if c.Capacity.MaxWorkerCount == nil || c.Capacity.MinWorkerCount == nil || c.Capacity.ScaleInCpuPercentage == nil || c.Capacity.ScaleOutCpuPercentage == nil {
			errorMsgs = append(errorMsgs, "for autoscaling msk connector: maxWorkerCount, minWorkerCount, scaleInCpuPercentage, and scaleOutCpuPercentage are required")
		}
	case Provisioned:
		if c.Capacity.WorkerCount == nil {
			errorMsgs = append(errorMsgs, "for provisioned msk connector: workerCount is required")
		}
	default:
		// invalid msk connector type
		errorMsgs = append(errorMsgs, fmt.Sprintf("invalid type %v, must be 'AUTOSCALING' or 'PROVISIONED'", *c.Type))
	}

	if errorMsgs == nil {
		return nil
	}

	return &ValidationError{
		ResourceType: "msk connector",
		Messages:     errorMsgs,
	}
}

// Apply creates a new msk connector
func (c MSKConnector) Apply(ctx context.Context) error {
	log.Debugf("creating/updating msk connector %v", c.Identifier())

	// set resource arns
	var err error
	if c.WorkerConfiguration != nil {
		err = c.setWorkerConfigurationArn(ctx)
	}
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to get arn for resource: %s", *c.WorkerConfiguration))
	}
	if err = c.setPluginArn(ctx); err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to get arn for resource: %s", c.Plugin))
	}
	if err = c.setClusterInfo(ctx); err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to get cluster info for: %s", c.Cluster))
	}
	roleArn, err := awsw.NewIAM(ctx, c.Context.ProviderName).RoleArnForName(ctx, c.Role)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to get arn for resource: %s", c.Role))
	}
	c._roleArn = roleArn

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

	connDiffs, ok := diffs.(*MSKConnectorDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// if unsupported diffs found & existing resource exists...
	if diffs.AWSResource() != nil && connDiffs.unsupportedDiff {
		log.WithField("Name", c.Identifier()).Error("update only supported for tags, msk connector configuration and capacity")
		return errors.Wrapf(err, "unsupported update")
	}

	// if supported diffs found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", c.Identifier()).Info("msk connector already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = c.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update msk connector %v", c.Identifier())
		}
		return nil
	}

	return c.apply(ctx)
}

// apply provisions a new msk connector
func (c MSKConnector) apply(ctx context.Context) error {
	var capacity mskTypes.Capacity
	if *c.Type == Autoscaling {
		capacity = mskTypes.Capacity{
			AutoScaling: &mskTypes.AutoScaling{
				McuCount:       c.Capacity.McuCount,
				MinWorkerCount: *c.Capacity.MinWorkerCount,
				MaxWorkerCount: *c.Capacity.MaxWorkerCount,
				ScaleInPolicy: &mskTypes.ScaleInPolicy{
					CpuUtilizationPercentage: *c.Capacity.ScaleInCpuPercentage,
				},
				ScaleOutPolicy: &mskTypes.ScaleOutPolicy{
					CpuUtilizationPercentage: *c.Capacity.ScaleOutCpuPercentage,
				},
			},
		}
	} else {
		capacity = mskTypes.Capacity{
			ProvisionedCapacity: &mskTypes.ProvisionedCapacity{
				McuCount:    c.Capacity.McuCount,
				WorkerCount: *c.Capacity.WorkerCount,
			},
		}
	}

	cluster := mskTypes.KafkaCluster{
		ApacheKafkaCluster: &mskTypes.ApacheKafkaCluster{
			BootstrapServers: c._clusterInfo.BootstrapServers,
			Vpc: &mskTypes.Vpc{
				Subnets:        c._clusterInfo.Subnets,
				SecurityGroups: c._clusterInfo.SecurityGroups,
			},
		},
	}

	logDelivery := mskTypes.LogDelivery{
		WorkerLogDelivery: &mskTypes.WorkerLogDelivery{
			CloudWatchLogs: &mskTypes.CloudWatchLogsLogDelivery{
				Enabled:  c.LogGroup != nil,
				LogGroup: aws.String(util.Coalesce(c.LogGroup, "")),
			},
		},
	}

	var workerConfig *mskTypes.WorkerConfiguration
	if c.WorkerConfiguration != nil {
		workerConfig = &mskTypes.WorkerConfiguration{
			WorkerConfigurationArn: c._workerConfigurationArn,
			Revision:               1,
		}
	}

	mskClient := client.MSK(ctx, c.Context.ProviderName)
	_, err := mskClient.CreateConnector(ctx, &kafkaconnect.CreateConnectorInput{
		ConnectorName:          aws.String(c.Name),
		ConnectorDescription:   c.Description,
		Capacity:               &capacity,
		ConnectorConfiguration: c.ConnectorConfiguration,
		KafkaCluster:           &cluster,
		KafkaClusterClientAuthentication: &mskTypes.KafkaClusterClientAuthentication{
			AuthenticationType: c._clusterInfo.ClientAuthType,
		},
		KafkaClusterEncryptionInTransit: &mskTypes.KafkaClusterEncryptionInTransit{
			EncryptionType: c._clusterInfo.EncryptionInTransitType,
		},
		KafkaConnectVersion: aws.String(c.KafkaConnectVersion),
		Plugins: []mskTypes.Plugin{
			{
				CustomPlugin: &mskTypes.CustomPlugin{
					CustomPluginArn: c._pluginArn,
					Revision:        1,
				},
			},
		},
		ServiceExecutionRoleArn: c._roleArn,
		LogDelivery:             &logDelivery,
		WorkerConfiguration:     workerConfig,
		Tags:                    c.Tags,
	})
	if err != nil {
		return errors.Wrapf(err, "error creating msk connector %v", c.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": c.Identifier(),
	}).Info(color.Green("msk connector created"))

	return nil
}

// applyDiffs applies supported diffs to an existing msk connector
func (c MSKConnector) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("no updates required for msk connector")
		return nil
	}

	connDiffs, ok := diffs.(*MSKConnectorDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	existing, ok := connDiffs.Resource.(*MSKConnector)
	if !ok {
		return errors.Errorf("cannot retrieve existing msk connector")
	}

	mskClient := awsw.NewMSK(ctx, c.Context.ProviderName)

	var err error
	// Update types are mutually exclusive. Exactly one of the following is required: capacity, connectorConfiguration.
	// Update connector configuration
	if connDiffs.connectorConfigurationDiff {
		_, err := mskClient.UpdateConnector(ctx, &kafkaconnect.UpdateConnectorInput{
			ConnectorArn:           existing._arn,
			CurrentVersion:         existing._currentVer,
			ConnectorConfiguration: c.ConnectorConfiguration,
		})
		if err != nil {
			return err
		}
	}

	// Update connector capacity
	if connDiffs.capacityDiff {
		// if a configuration update was ran prior we need to fetch the latest version
		if connDiffs.connectorConfigurationDiff {
			// have to wait for configuration update to finish first before applying capacity update
			err = c.waitForUpdate(ctx)
			if err != nil {
				return err
			}
			existing, err = c.fetchExisting(ctx)
			if err != nil {
				return err
			}
		}
		var capacityUpdate mskTypes.CapacityUpdate
		if *c.Type == Autoscaling {
			capacityUpdate = mskTypes.CapacityUpdate{
				AutoScaling: &mskTypes.AutoScalingUpdate{
					McuCount:       c.Capacity.McuCount,
					MinWorkerCount: *c.Capacity.MinWorkerCount,
					MaxWorkerCount: *c.Capacity.MaxWorkerCount,
					ScaleInPolicy: &mskTypes.ScaleInPolicyUpdate{
						CpuUtilizationPercentage: *c.Capacity.ScaleInCpuPercentage,
					},
					ScaleOutPolicy: &mskTypes.ScaleOutPolicyUpdate{
						CpuUtilizationPercentage: *c.Capacity.ScaleOutCpuPercentage,
					},
				},
			}
		} else {
			capacityUpdate = mskTypes.CapacityUpdate{
				ProvisionedCapacity: &mskTypes.ProvisionedCapacityUpdate{
					McuCount:    c.Capacity.McuCount,
					WorkerCount: *c.Capacity.WorkerCount,
				},
			}
		}

		_, err = mskClient.UpdateConnector(ctx, &kafkaconnect.UpdateConnectorInput{
			ConnectorArn:   existing._arn,
			CurrentVersion: existing._currentVer,
			Capacity:       &capacityUpdate,
		})
		if err != nil {
			return err
		}
	}

	if connDiffs.tagsDiff {
		upserts := connDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			err := mskClient.AddResourceTags(ctx, *existing._arn, upserts)
			if err != nil {
				return err
			}
		}

		if len(connDiffs.tagDiff.Deleted) > 0 {
			err := mskClient.DeleteResourceTags(ctx, *existing._arn, connDiffs.tagDiff.Deleted)
			if err != nil {
				return err
			}
		}
	}

	log.WithFields(log.Fields{
		"Name": c.Identifier(),
	}).Info(color.Yellow("msk connector updated"))

	return nil
}

// Wait waits for the deployment of the connector to be complete.
// It does this by continuously polling AWS until it sees that the connector state is RUNNING
func (c MSKConnector) Wait(ctx context.Context) error {
	start := time.Now()

	// DISCUSS(@maintainer): Should this be configurable?
	checkInterval := 15 * time.Second
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()

	for {
		// Wait for either the context to be done or the sleep duration.
		// This adds a wait period in between checks so we can give AWS a chance to finish the deployment before checking again.
		if err := util.SleepWithContext(ctx, checkInterval); err != nil {
			return err
		}

		existing, err := c.fetchExisting(ctx)
		if err != nil {
			return errors.Wrapf(err, "error finding msk connector: %v", c.Identifier())
		}

		if existing != nil && *existing._state == string(mskTypes.ConnectorStateRunning) {
			log.Infof(color.Green("Deployment of msk connector %v is complete"), c.Name)
			break
		} else if existing != nil && *existing._state == string(mskTypes.ConnectorStateFailed) {
			log.Infof(color.Red("Deployment of msk connector %v failed"), c.Name)
			return fmt.Errorf("deployment of msk connector %v failed", c.Name)
		} else {
			log.Infof("Deployment of msk connector %v is not yet complete)", color.Cyan(c.Name))
			continue
		}
	}

	elapsed := time.Since(start)
	log.Infof("msk custom connector %s successfully deployed in %v", c.Name, elapsed)
	return nil
}

// waitForUpdate waits for the update of the connector to be complete.
// It does this by continuously polling AWS until it sees that the connector state is RUNNING
func (c MSKConnector) waitForUpdate(ctx context.Context) error {
	// DISCUSS(@maintainer): Should this be configurable?
	checkInterval := 5 * time.Second
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()

	for {
		// Wait for either the context to be done or the sleep duration.
		// This adds a wait period in between checks so we can give AWS a chance to finish the deployment before checking again.
		if err := util.SleepWithContext(ctx, checkInterval); err != nil {
			return err
		}

		existing, err := c.fetchExisting(ctx)
		if err != nil {
			return errors.Wrapf(err, "error finding msk connector: %v", c.Identifier())
		}

		if existing != nil && *existing._state == string(mskTypes.ConnectorStateRunning) {
			log.Infof(color.Yellow("Msk connector %v configuration updated"), c.Name)
			break
		} else if existing != nil && *existing._state == string(mskTypes.ConnectorStateFailed) {
			log.Infof(color.Red("Msk connector %v configuration update failed"), c.Name)
			return fmt.Errorf("configuration update of msk connector %v failed", c.Name)
		} else {
			log.Infof("Msk connector %v configuration update is not yet complete)", color.Cyan(c.Name))
			continue
		}
	}

	return nil
}

// waitUntilConnectorInactive waits untils the msk connector is removed. Currently this
// function will wait 15s between each check & perform a maximum of 40 checks, so the maximum wait time before
// this function exits with a failure is 600s or 10m
func (c MSKConnector) waitForDeletion(ctx context.Context) error {
	done := false
	retries := retriesRemaining

	for !done {
		log.Debugf("waiting %v seconds to check connector status", waitInSeconds)
		time.Sleep(time.Duration(waitInSeconds) * time.Second)

		existing, err := c.fetchExisting(ctx)
		if err != nil {
			return errors.Wrap(err, "error fetching connector details")
		}

		// if service is not found, return success
		if existing == nil {
			log.Infof("connector %v has been removed", c.Name)
			return nil
		}

		retries--
		done = retries == 0
	}

	return errors.New("couldln't verify connector was deleted within specified time")
}

// msk connector diff
type MSKConnectorDiff struct {
	BaseResourceDiff
	unsupportedDiff            bool
	capacityDiff               bool
	connectorConfigurationDiff bool
	tagsDiff                   bool
	tagDiff                    util.TagDiffResult
}

// Compare fetches the existing msk connector & if it exists returns nil, else returns the diffs
func (c MSKConnector) Compare(ctx context.Context) (ResourceDiff, error) {
	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", c.Identifier())
	}

	diffs := &MSKConnectorDiff{}

	diff := false

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "msk connector does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	// comparing capacity
	if util.Coalesce(c.Type, "") != util.Coalesce(existing.Type, "") || !reflect.DeepEqual(c.Capacity, existing.Capacity) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("capacity will be updated %s:%v -> %s:%v", *existing.Type, existing.Capacity, *c.Type, c.Capacity))
		diffs.capacityDiff = true
	}

	// msk connector configuration
	if !util.StringMap(c.ConnectorConfiguration).Equals(existing.ConnectorConfiguration) {
		diff = true
		diffs.Messages = append(diffs.Messages, "msk connector configuration diff found")
		diffs.connectorConfigurationDiff = true
	}

	// tags
	if tagDiff := TagDiffForContext(ctx, existing.Tags, c.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(existing.Tags, diffs.tagDiff)...)
	}

	// UNSUPPORTED
	// comparing kafka cluster
	if !util.SliceElementsEqual(strings.Split(*c._clusterInfo.BootstrapServers, ","), strings.Split(*existing._clusterInfo.BootstrapServers, ",")) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: kafka cluster bootstrap servers %s -> %s", *existing._clusterInfo.BootstrapServers, *c._clusterInfo.BootstrapServers))
		diffs.unsupportedDiff = true
	}

	if !util.SliceElementsEqual(c._clusterInfo.SecurityGroups, existing._clusterInfo.SecurityGroups) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: kafka cluster security group %v -> %v", existing._clusterInfo.SecurityGroups, c._clusterInfo.SecurityGroups))
		diffs.unsupportedDiff = true
	}

	if !util.SliceElementsEqual(c._clusterInfo.Subnets, existing._clusterInfo.Subnets) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: kafka cluster subnets %v -> %v", existing._clusterInfo.Subnets, c._clusterInfo.Subnets))
		diffs.unsupportedDiff = true
	}

	if c._clusterInfo.ClientAuthType != existing._clusterInfo.ClientAuthType {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: kafka cluster auth type %s -> %s", existing._clusterInfo.ClientAuthType, c._clusterInfo.ClientAuthType))
		diffs.unsupportedDiff = true
	}

	if c._clusterInfo.EncryptionInTransitType != existing._clusterInfo.EncryptionInTransitType {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: kafka cluster encryption in transit %s -> %s", existing._clusterInfo.EncryptionInTransitType, c._clusterInfo.EncryptionInTransitType))
		diffs.unsupportedDiff = true
	}

	// comparing description
	if util.Coalesce(c.Description, "") != util.Coalesce(existing.Description, "") {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: description diff %s -> %s", util.Coalesce(existing.Description, ""), util.Coalesce(c.Description, "")))
		diffs.unsupportedDiff = true
	}

	// kafka connect version
	if c.KafkaConnectVersion != existing.KafkaConnectVersion {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: kafka connect version diff %s -> %s", existing.KafkaConnectVersion, c.KafkaConnectVersion))
		diffs.unsupportedDiff = true
	}

	// plugin
	if util.Coalesce(c._pluginArn, "") != util.Coalesce(existing._pluginArn, "") {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: plugin diff %s -> %s", util.Coalesce(existing._pluginArn, ""), util.Coalesce(c._pluginArn, "")))
		diffs.unsupportedDiff = true
	}

	// worker config
	if util.Coalesce(c._workerConfigurationArn, "") != util.Coalesce(existing._workerConfigurationArn, "") {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: worker configuration diff %s -> %s", util.Coalesce(existing._workerConfigurationArn, ""), util.Coalesce(c._workerConfigurationArn, "")))
		diffs.unsupportedDiff = true
	}

	// service execution role
	if util.Coalesce(c._roleArn, "") != util.Coalesce(existing._roleArn, "") {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: execution role diff %s -> %s", util.Coalesce(existing._roleArn, ""), util.Coalesce(c._roleArn, "")))
		diffs.unsupportedDiff = true
	}

	// log group
	if util.Coalesce(c.LogGroup, "") != util.Coalesce(existing.LogGroup, "") {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: log group diff %s -> %s", util.Coalesce(existing.LogGroup, ""), util.Coalesce(c.LogGroup, "")))
		diffs.unsupportedDiff = true
	}

	if !diff {
		return nil, nil
	}

	// return
	return diffs, nil
}

// Destroy removes the msk connector
func (c MSKConnector) Destroy(ctx context.Context) error {
	log.Debugf("destroying connector: %v", c.Identifier())

	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding msk connector: %v", c.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("msk connector does not exist, nothing to destroy, skippping")
		return nil
	}

	mskClient := client.MSK(ctx, c.Context.ProviderName)

	if _, err = mskClient.DeleteConnector(ctx, &kafkaconnect.DeleteConnectorInput{
		ConnectorArn: existing._arn,
	}); err != nil {
		return errors.Wrapf(err, "error deleting msk connector: %v", c.Identifier())
	}

	if err = c.waitForDeletion(ctx); err != nil {
		return errors.Wrapf(err, "error waiting for msk connector deletion: %v", c.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": c.Identifier(),
	}).Info(color.Red("msk connector destroyed"))

	return nil
}

// fetchExisting returns the existing msk connector details if found
func (c MSKConnector) fetchExisting(ctx context.Context) (*MSKConnector, error) {
	mskClient := client.MSK(ctx, c.Context.ProviderName)
	nextToken := ""
	var found *mskTypes.ConnectorSummary
	for {
		out, err := mskClient.ListConnectors(ctx, &kafkaconnect.ListConnectorsInput{
			ConnectorNamePrefix: aws.String(c.Name),
			NextToken:           aws.String(nextToken),
		})
		if err != nil {
			return nil, errors.Wrapf(err, "error listing msk connector")
		}

		if len(out.Connectors) == 0 {
			return nil, nil
		}

		for _, conn := range out.Connectors {
			if c.Name == *conn.ConnectorName {
				found = &conn
				break
			}
		}
		if out.NextToken == nil || found != nil {
			break
		}
		nextToken = *out.NextToken
	}
	if found == nil {
		return nil, nil
	}

	// if found create a MSKConnector obj
	out, err := mskClient.DescribeConnector(ctx, &kafkaconnect.DescribeConnectorInput{
		ConnectorArn: found.ConnectorArn,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error describing msk connector %v", c.Name)
	}

	// tags
	tags, err := mskClient.ListTagsForResource(ctx, &kafkaconnect.ListTagsForResourceInput{
		ResourceArn: found.ConnectorArn,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error listing tags for msk connector %v", c.Name)
	}

	// capacity
	capacityType := Autoscaling
	if out.Capacity.AutoScaling == nil {
		capacityType = Provisioned
	}
	var cap Capacity
	if capacityType == Autoscaling {
		cap.McuCount = out.Capacity.AutoScaling.McuCount
		cap.MaxWorkerCount = &out.Capacity.AutoScaling.MaxWorkerCount
		cap.MinWorkerCount = &out.Capacity.AutoScaling.MinWorkerCount
		cap.ScaleInCpuPercentage = &out.Capacity.AutoScaling.ScaleInPolicy.CpuUtilizationPercentage
		cap.ScaleOutCpuPercentage = &out.Capacity.AutoScaling.ScaleOutPolicy.CpuUtilizationPercentage
	} else {
		cap.McuCount = out.Capacity.ProvisionedCapacity.McuCount
		cap.WorkerCount = &out.Capacity.ProvisionedCapacity.WorkerCount
	}

	cluster := ClusterInfo{
		BootstrapServers:        out.KafkaCluster.ApacheKafkaCluster.BootstrapServers,
		SecurityGroups:          out.KafkaCluster.ApacheKafkaCluster.Vpc.SecurityGroups,
		Subnets:                 out.KafkaCluster.ApacheKafkaCluster.Vpc.Subnets,
		ClientAuthType:          out.KafkaClusterClientAuthentication.AuthenticationType,
		EncryptionInTransitType: out.KafkaClusterEncryptionInTransit.EncryptionType,
	}

	var workerConfigArn *string
	if out.WorkerConfiguration != nil {
		workerConfigArn = out.WorkerConfiguration.WorkerConfigurationArn
	}

	// log delivery
	logGroup := util.Coalesce(out.LogDelivery.WorkerLogDelivery.CloudWatchLogs.LogGroup, "")

	existing := &MSKConnector{
		Name:                    util.Coalesce(out.ConnectorName, ""),
		Description:             out.ConnectorDescription,
		Type:                    &capacityType,
		Capacity:                cap,
		ConnectorConfiguration:  out.ConnectorConfiguration,
		KafkaConnectVersion:     util.Coalesce(out.KafkaConnectVersion, ""),
		_pluginArn:              out.Plugins[0].CustomPlugin.CustomPluginArn,
		_workerConfigurationArn: workerConfigArn,
		_roleArn:                out.ServiceExecutionRoleArn,
		LogGroup:                &logGroup,
		Tags:                    tags.Tags,
		_arn:                    out.ConnectorArn,
		_state:                  (*string)(&out.ConnectorState),
		_currentVer:             out.CurrentVersion,
		_clusterInfo:            cluster,
	}

	return existing, nil
}

func (c *MSKConnector) setPluginArn(ctx context.Context) error {
	mskClient := client.MSK(ctx, c.Context.ProviderName)
	nextToken := ""
	for {
		out, err := mskClient.ListCustomPlugins(ctx, &kafkaconnect.ListCustomPluginsInput{
			NamePrefix: aws.String(c.Plugin),
			NextToken:  aws.String(nextToken),
		})
		if err != nil {
			return errors.Wrapf(err, "error listing plugin")
		}

		if len(out.CustomPlugins) == 0 {
			return nil
		}

		for _, plugin := range out.CustomPlugins {
			if c.Plugin == *plugin.Name {
				c._pluginArn = plugin.CustomPluginArn
				return nil
			}
		}
		if out.NextToken == nil {
			break
		}
		nextToken = *out.NextToken
	}
	return nil
}

func (c *MSKConnector) setWorkerConfigurationArn(ctx context.Context) error {
	mskClient := client.MSK(ctx, c.Context.ProviderName)
	nextToken := ""
	for {
		out, err := mskClient.ListWorkerConfigurations(ctx, &kafkaconnect.ListWorkerConfigurationsInput{
			NamePrefix: aws.String(*c.WorkerConfiguration),
			NextToken:  aws.String(nextToken),
		})
		if err != nil {
			return errors.Wrapf(err, "error listing worker configuration")
		}

		if len(out.WorkerConfigurations) == 0 {
			return nil
		}

		for _, conf := range out.WorkerConfigurations {
			if *c.WorkerConfiguration == *conf.Name {
				c._workerConfigurationArn = conf.WorkerConfigurationArn
				return nil
			}
		}
		if out.NextToken == nil {
			break
		}
		nextToken = *out.NextToken
	}
	return nil
}

func (c *MSKConnector) setClusterInfo(ctx context.Context) error {
	kafkaClient := client.Kafka(ctx, c.Context.ProviderName)
	nextToken := ""
	var arn *string
	for {
		out, err := kafkaClient.ListClustersV2(ctx, &kafka.ListClustersV2Input{
			ClusterNameFilter: aws.String(c.Cluster),
			NextToken:         aws.String(nextToken),
		})
		if err != nil {
			return errors.Wrapf(err, "error listing kafka cluster")
		}

		if len(out.ClusterInfoList) == 0 {
			return nil
		}

		for _, cl := range out.ClusterInfoList {
			if c.Cluster == *cl.ClusterName {
				arn = cl.ClusterArn
				break
			}
		}
		if out.NextToken == nil || arn != nil {
			break
		}
		nextToken = *out.NextToken
	}
	if arn == nil {
		return fmt.Errorf("Resource not found %v", c.Cluster)
	}

	brokers, err := kafkaClient.GetBootstrapBrokers(ctx, &kafka.GetBootstrapBrokersInput{
		ClusterArn: arn,
	})
	if err != nil {
		return errors.Wrapf(err, "error getting bootstrap servers")
	}

	desc, err := kafkaClient.DescribeCluster(ctx, &kafka.DescribeClusterInput{
		ClusterArn: arn,
	})
	if err != nil {
		return errors.Wrapf(err, "error describing cluster")
	}

	auth := mskTypes.KafkaClusterClientAuthenticationTypeIam
	if *desc.ClusterInfo.ClientAuthentication.Unauthenticated.Enabled {
		auth = mskTypes.KafkaClusterClientAuthenticationTypeNone
	}

	encryption := mskTypes.KafkaClusterEncryptionInTransitTypeTls
	if desc.ClusterInfo.EncryptionInfo.EncryptionInTransit.ClientBroker == "PLAINTEXT" {
		encryption = mskTypes.KafkaClusterEncryptionInTransitTypePlaintext
	}

	bootstrapServers := brokers.BootstrapBrokerString
	if auth == mskTypes.KafkaClusterClientAuthenticationTypeIam && encryption == mskTypes.KafkaClusterEncryptionInTransitTypeTls {
		bootstrapServers = brokers.BootstrapBrokerStringSaslIam
	}
	if auth == mskTypes.KafkaClusterClientAuthenticationTypeNone && encryption == mskTypes.KafkaClusterEncryptionInTransitTypeTls {
		bootstrapServers = brokers.BootstrapBrokerStringTls
	}

	c._clusterInfo = ClusterInfo{
		BootstrapServers:        bootstrapServers,
		SecurityGroups:          desc.ClusterInfo.BrokerNodeGroupInfo.SecurityGroups,
		Subnets:                 desc.ClusterInfo.BrokerNodeGroupInfo.ClientSubnets,
		ClientAuthType:          auth,
		EncryptionInTransitType: encryption,
	}

	return nil
}
