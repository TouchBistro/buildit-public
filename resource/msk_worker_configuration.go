package resource

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kafkaconnect"

	log "github.com/sirupsen/logrus"
)

// MSKWorkerConfiguration represents a worker configuration
type MSKWorkerConfiguration struct {
	BaseResource `yaml:",inline"`
	Name                  string            `yaml:"-"`
	Description           string            `yaml:"description"`
	Secrets               map[string]string `yaml:"secrets"`
	Content               map[string]string `yaml:"content"`
	DependsOn             []Key             `yaml:"dependsOn"`
	Tags                  map[string]string `yaml:"tags"`
	GlobalTags            map[string]string `yaml:"-"`
	PropertiesFileContent string            `yaml:"-"`
	_arn                  *string           `yaml:"-"` // read from AWS only
}

// Key returns the unique key for the resource for this buildit context
func (c MSKWorkerConfiguration) Key() Key {
	return NewKey(c.Context.ProviderName, c.Identifier())
}

// Identifier returns topic and endpoint name of the worker configuration
func (c MSKWorkerConfiguration) Identifier() string {
	return c.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (c *MSKWorkerConfiguration) Normalize(ctx context.Context) {
	if c.Tags == nil {
		c.Tags = make(map[string]string)
	}
	ResourceTags(c.Tags).Merge(c.GlobalTags)

	// secrets
	for k, v := range c.Secrets {
		val, err := awsw.NewSecretsManager(ctx, c.Context.ProviderName).GetValueBySecretId(ctx, v)
		if err != nil {
			panic(err)
		}
		c.Secrets[k] = *val
	}

	// merge secrets into config
	if c.Content == nil {
		c.Content = make(map[string]string)
	}
	for k, v := range c.Secrets {
		c.Content[k] = v
	}

	// convert content to Base64 encoded
	if err := c.contentToPropertiesFileContent(); err != nil {
		panic(err)
	}
}

// Validate checks that the input provided is correct
func (c MSKWorkerConfiguration) Validate(ctx context.Context) error {

	var errorMsgs []string

	if c.Content == nil {
		errorMsgs = append(errorMsgs, "content is required")
	}

	if errorMsgs == nil {
		return nil
	}

	return &ValidationError{
		ResourceType: "worker configuration",
		Messages:     errorMsgs,
	}
}

// Apply creates a new worker configuration
func (c MSKWorkerConfiguration) Apply(ctx context.Context) error {
	log.Debugf("creating/updating worker configuration %v", c.Identifier())

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

	pluginDiffs, ok := diffs.(*MSKWorkerConfigurationDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	//if unsupported diffs found & existing resource exists...
	if diffs.AWSResource() != nil && pluginDiffs.unsupportedDiff {
		log.WithField("Name", c.Identifier()).Error("update only supported for tags")
		return errors.Wrapf(err, "unsupported update")
	}

	//if supported diffs found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", c.Identifier()).Info("worker configuration already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = c.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update worker configuration %v", c.Identifier())
		}
		return nil
	}

	return c.apply(ctx)
}

// apply provisions a new worker configuration
func (c MSKWorkerConfiguration) apply(ctx context.Context) error {
	mskClient := client.MSK(ctx, c.Context.ProviderName)
	_, err := mskClient.CreateWorkerConfiguration(ctx, &kafkaconnect.CreateWorkerConfigurationInput{
		Name:                  aws.String(c.Name),
		Description:           aws.String(c.Description),
		PropertiesFileContent: aws.String(c.PropertiesFileContent),
		Tags:                  c.Tags,
	})

	if err != nil {
		return errors.Wrapf(err, "error creating worker configuration %v", c.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": c.Identifier(),
	}).Info(color.Green("worker configuration created"))

	return nil
}

// applyDiffs applies supported diffs to an existin worker configuration
func (c MSKWorkerConfiguration) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("no updates required for worker configuration")
		return nil
	}

	pluginDiffs, ok := diffs.(*MSKWorkerConfigurationDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	existing, ok := pluginDiffs.Resource.(*MSKWorkerConfiguration)
	if !ok {
		return errors.Errorf("cannot retrieve existing worker configuration")
	}

	mskClient := awsw.NewMSK(ctx, c.Context.ProviderName)
	if pluginDiffs.tagsDiff {
		upserts := pluginDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			err := mskClient.AddResourceTags(ctx, *existing._arn, upserts)
			if err != nil {
				return err
			}
		}

		if len(pluginDiffs.tagDiff.Deleted) > 0 {
			err := mskClient.DeleteResourceTags(ctx, *existing._arn, pluginDiffs.tagDiff.Deleted)
			if err != nil {
				return err
			}
		}
	}

	log.WithFields(log.Fields{
		"Name": c.Identifier(),
	}).Info(color.Yellow("worker configuration updated"))

	return nil
}

// worker configuration diff
type MSKWorkerConfigurationDiff struct {
	BaseResourceDiff
	unsupportedDiff bool
	tagsDiff        bool
	tagDiff         util.TagDiffResult
}

// Compare fetches the existing worker configuration & if it exists returns nil, else returns the diffs
func (c MSKWorkerConfiguration) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", c.Identifier())
	}

	diffs := &MSKWorkerConfigurationDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "worker configuration does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	diff := false

	// comparing description
	if c.Description != existing.Description {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: description will be updated %v -> %v", existing.Description, c.Description))
		diffs.unsupportedDiff = true
	}

	// comparing PropertiesFileContent
	if c.PropertiesFileContent != existing.PropertiesFileContent {
		diff = true
		diffs.Messages = append(diffs.Messages, "unsupportedDiff: content will be updated")
		diffs.unsupportedDiff = true
	}

	// tags
	if tagDiff := TagDiffForContext(ctx, existing.Tags, c.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(existing.Tags, diffs.tagDiff)...)
	}

	// diff is found on existing resource
	if !diff {
		return nil, nil
	}

	// return
	return diffs, nil
}

// Destroy removes the worker configuration
func (c MSKWorkerConfiguration) Destroy(ctx context.Context) error {
	log.Debugf("destroying worker configuration: %v", c.Identifier())

	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding worker configuration: %v", c.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("worker configuration does not exist, nothing to destroy, skippping")
		return nil
	}

	mskClient := client.MSK(ctx, c.Context.ProviderName)

	_, err = mskClient.DeleteWorkerConfiguration(ctx, &kafkaconnect.DeleteWorkerConfigurationInput{
		WorkerConfigurationArn: existing._arn,
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting worker configuration: %v", c.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": c.Identifier(),
	}).Info(color.Red("worker configuration destroyed"))

	return nil
}

// fetchExisting returns the existing worker configuration details if found
func (c MSKWorkerConfiguration) fetchExisting(ctx context.Context) (*MSKWorkerConfiguration, error) {
	mskClient := client.MSK(ctx, c.Context.ProviderName)
	nextToken := ""
	for {
		out, err := mskClient.ListWorkerConfigurations(ctx, &kafkaconnect.ListWorkerConfigurationsInput{
			NamePrefix: aws.String(c.Name),
			NextToken:  aws.String(nextToken),
		})

		if err != nil {
			return nil, errors.Wrapf(err, "error listing worker configuration")
		}

		if len(out.WorkerConfigurations) == 0 {
			return nil, nil
		}

		for _, conf := range out.WorkerConfigurations {
			if c.Name == *conf.Name {
				out, err := mskClient.DescribeWorkerConfiguration(ctx, &kafkaconnect.DescribeWorkerConfigurationInput{
					WorkerConfigurationArn: conf.WorkerConfigurationArn,
				})
				if err != nil {
					return nil, errors.Wrapf(err, "error describing worker configuration %v", c.Name)
				}

				tags, err := mskClient.ListTagsForResource(ctx, &kafkaconnect.ListTagsForResourceInput{
					ResourceArn: conf.WorkerConfigurationArn,
				})
				if err != nil {
					return nil, errors.Wrapf(err, "error listing tags for worker configuration %v", c.Name)
				}

				existing := &MSKWorkerConfiguration{
					Name:                  util.Coalesce(out.Name, ""),
					Description:           util.Coalesce(out.Description, ""),
					PropertiesFileContent: *out.LatestRevision.PropertiesFileContent,
					Tags:                  tags.Tags,
					_arn:                  out.WorkerConfigurationArn,
				}
				return existing, nil
			}
		}
		if out.NextToken == nil {
			break
		}
		nextToken = *out.NextToken
	}
	return nil, nil
}

func (c *MSKWorkerConfiguration) contentToPropertiesFileContent() error {
	// Sort keys for consistent output
	var keys []string
	for k := range c.Content {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build properties format
	var builder strings.Builder
	for _, k := range keys {
		builder.WriteString(fmt.Sprintf("%v=%v\n", k, c.Content[k]))
	}

	propertiesString := builder.String()

	// Base64 encode the properties file
	c.PropertiesFileContent = base64.StdEncoding.EncodeToString([]byte(propertiesString))
	return nil
}
