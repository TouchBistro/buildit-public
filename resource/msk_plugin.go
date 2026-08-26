package resource

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kafkaconnect"
	mskTypes "github.com/aws/aws-sdk-go-v2/service/kafkaconnect/types"

	log "github.com/sirupsen/logrus"
)

// MSKPlugin represents a kafka plugin
type MSKPlugin struct {
	BaseResource `yaml:",inline"`
	Name           string            `yaml:"-"`
	Description    *string           `yaml:"description"`
	Code           Code              `yaml:"code"`
	TimeoutSeconds int               `yaml:"deploymentTimeout"`
	DependsOn      []Key             `yaml:"dependsOn"`
	Tags           map[string]string `yaml:"tags"`
	GlobalTags     map[string]string `yaml:"-"`
	ContentType    string            `yaml:"-"`
	_arn           *string           `yaml:"-"` // read from AWS only
	_state         *string           `yaml:"-"` // read from AWS only
}

type Code struct {
	Bucket        string  `yaml:"bucket,omitempty"`
	FileKey       string  `yaml:"key,omitempty"`
	ObjectVersion *string `yaml:"version,omitempty"`
	_checksum     *string `yaml:"-"` // for comparing
}

// Key returns the unique key for the resource for this buildit context
func (p MSKPlugin) Key() Key {
	return NewKey(p.Context.ProviderName, p.Identifier())
}

// Identifier returns topic and endpoint name of the kafka plugin
func (p MSKPlugin) Identifier() string {
	return p.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (p *MSKPlugin) Normalize(ctx context.Context) {
	if p.Tags == nil {
		p.Tags = make(map[string]string)
	}
	ResourceTags(p.Tags).Merge(p.GlobalTags)

	// set content type
	p.ContentType = strings.ToUpper(path.Ext(p.Code.FileKey))
	p.ContentType = p.ContentType[1:]

	// sha256
	var err error
	p.Code._checksum, err = awsw.NewS3(ctx, p.Context.ProviderName).GetObjectChecksum(ctx, p.Code.Bucket, p.Code.FileKey, p.Code.ObjectVersion)
	if err != nil {
		panic(err)
	}

	// timeout
	if p.TimeoutSeconds < 1 {
		p.TimeoutSeconds = 60 // default to 1min
	}
}

// Validate checks that the input provided is correct
func (p MSKPlugin) Validate(ctx context.Context) error {

	var errorMsgs []string

	// check code package
	if p.Code.Bucket == "" || p.Code.FileKey == "" {
		errorMsgs = append(errorMsgs, "Bucket & fileKey is required")
	}
	if p.Code._checksum == nil {
		errorMsgs = append(errorMsgs, "checksum (one of type SHA256,SHA1,CRC32,RC32C) is required for the code package")
	}

	// check contentType
	if p.ContentType != "JAR" && p.ContentType != "ZIP" {
		errorMsgs = append(errorMsgs, "fileKey must points to a ZIP or JAR file")
	}

	if errorMsgs == nil {
		return nil
	}

	return &ValidationError{
		ResourceType: "kafka plugin",
		Messages:     errorMsgs,
	}
}

// Apply creates a new kafka plugin
func (p MSKPlugin) Apply(ctx context.Context) error {
	log.Debugf("creating/updating plugin %v", p.Identifier())

	diffs, err := p.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": p.Identifier(),
		}).Info("no updates required")
		return nil
	}

	pluginDiffs, ok := diffs.(*MSKPluginDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	//if unsupported diffs found & existing resource exists...
	if diffs.AWSResource() != nil && pluginDiffs.unsupportedDiff {
		log.WithField("Name", p.Identifier()).Error("update only supported for tags")
		return errors.Wrapf(err, "unsupported update")
	}

	//if supported diffs found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", p.Identifier()).Info("plugin already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = p.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update plugin %v", p.Identifier())
		}
		return nil
	}

	return p.apply(ctx)
}

// apply provisions a new kafka plugin
func (p MSKPlugin) apply(ctx context.Context) error {
	mskClient := client.MSK(ctx, p.Context.ProviderName)
	_, err := mskClient.CreateCustomPlugin(ctx, &kafkaconnect.CreateCustomPluginInput{
		Name:        aws.String(p.Name),
		Description: p.Description,
		ContentType: mskTypes.CustomPluginContentType(p.ContentType),
		Location: &mskTypes.CustomPluginLocation{
			S3Location: &mskTypes.S3Location{
				BucketArn:     aws.String(fmt.Sprintf("arn:aws:s3:::%v", p.Code.Bucket)),
				FileKey:       aws.String(p.Code.FileKey),
				ObjectVersion: p.Code.ObjectVersion,
			},
		},
		Tags: p.Tags,
	})

	if err != nil {
		return errors.Wrapf(err, "error creating kafka plugin %v", p.Identifier())
	}

	err = p.waitForDeploy(ctx)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"Name": p.Identifier(),
	}).Info(color.Green("kafka plugin created"))

	return nil
}

// applyDiffs applies supported diffs to an existing kafka plugin
func (p MSKPlugin) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": p.Identifier(),
		}).Info("no updates required for plugin")
		return nil
	}

	pluginDiffs, ok := diffs.(*MSKPluginDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	existing, ok := pluginDiffs.Resource.(*MSKPlugin)
	if !ok {
		return errors.Errorf("cannot retrieve existing plugin")
	}

	mskClient := awsw.NewMSK(ctx, p.Context.ProviderName)
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
		"Name": p.Identifier(),
	}).Info(color.Yellow("kafka plugin updated"))

	return nil
}

// Wait waits for the deployment of the plugin to be complete.
// It does this by continuously polling AWS until it sees that the plugin state is ACTIVE
func (p MSKPlugin) waitForDeploy(ctx context.Context) error {

	start := time.Now()

	// DISCUSS(@maintainer): Should this be configurable?
	checkInterval := 3 * time.Second
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutSeconds)*time.Second)
	defer cancel()

	for {
		// Wait for either the context to be done or the sleep duration.
		// This adds a wait period in between checks so we can give AWS a chance to finish the deployment before checking again.
		if err := util.SleepWithContext(ctx, checkInterval); err != nil {
			return err
		}

		existing, err := p.fetchExisting(ctx)
		if err != nil {
			return errors.Wrapf(err, "error finding kafka plugin: %v", p.Identifier())
		}

		if existing == nil || *existing._state != string(mskTypes.CustomPluginStateActive) {
			log.Infof("Deployment of msk plugin %v is not yet complete)", color.Cyan(p.Name))
			continue
		} else {
			log.Infof(color.Green("Deployment of msk plugin %v is complete"), p.Name)
			break
		}
	}

	elapsed := time.Since(start)
	log.Infof("msk custom plugin %s successfully deployed in %v", p.Name, elapsed)
	return nil
}

// kafka plugin diff
type MSKPluginDiff struct {
	BaseResourceDiff
	unsupportedDiff bool
	tagsDiff        bool
	tagDiff         util.TagDiffResult
}

// Compare fetches the existing kafka plugin & if it exists returns nil, else returns the diffs
func (p MSKPlugin) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := p.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", p.Identifier())
	}

	diffs := &MSKPluginDiff{}

	diff := false

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "kafka plugin does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	// comparing description
	if util.Coalesce(p.Description, "") != util.Coalesce(existing.Description, "") {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("unsupportedDiff: description diff found %v -> %v", existing.Description, p.Description))
		diffs.unsupportedDiff = true
	}

	// comparing code
	if *p.Code._checksum != *existing.Code._checksum {
		diff = true
		diffs.Messages = append(diffs.Messages, ("unsupportedDiff: code package diff found"))
		diffs.unsupportedDiff = true
	}

	// tags
	if tagDiff := TagDiffForContext(ctx, existing.Tags, p.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(existing.Tags, diffs.tagDiff)...)
	}

	if !diff {
		return nil, nil
	}

	// return
	return diffs, nil
}

// Destroy removes the kafka plugin
func (p MSKPlugin) Destroy(ctx context.Context) error {
	log.Debugf("destroying kafka plugin: %v", p.Identifier())

	existing, err := p.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding kafka plugin: %v", p.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": p.Identifier(),
		}).Info("kafka plugin does not exist, nothing to destroy, skippping")
		return nil
	}

	mskClient := client.MSK(ctx, p.Context.ProviderName)

	_, err = mskClient.DeleteCustomPlugin(ctx, &kafkaconnect.DeleteCustomPluginInput{
		CustomPluginArn: existing._arn,
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting kafka plugin: %v", p.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": p.Identifier(),
	}).Info(color.Red("kafka plugin destroyed"))

	return nil
}

// fetchExisting returns the existing kafka plugin details if found
func (p MSKPlugin) fetchExisting(ctx context.Context) (*MSKPlugin, error) {
	mskClient := client.MSK(ctx, p.Context.ProviderName)
	nextToken := ""
	for {
		out, err := mskClient.ListCustomPlugins(ctx, &kafkaconnect.ListCustomPluginsInput{
			NamePrefix: aws.String(p.Name),
			NextToken:  aws.String(nextToken),
		})

		if err != nil {
			return nil, errors.Wrapf(err, "error listing kafka plugin")
		}

		if len(out.CustomPlugins) == 0 {
			return nil, nil
		}

		for _, plugin := range out.CustomPlugins {
			if p.Name == *plugin.Name {
				out, err := mskClient.DescribeCustomPlugin(ctx, &kafkaconnect.DescribeCustomPluginInput{
					CustomPluginArn: plugin.CustomPluginArn,
				})
				if err != nil {
					return nil, errors.Wrapf(err, "error describing plugin %v", p.Name)
				}

				// tags
				tags, err := mskClient.ListTagsForResource(ctx, &kafkaconnect.ListTagsForResourceInput{
					ResourceArn: plugin.CustomPluginArn,
				})
				if err != nil {
					return nil, errors.Wrapf(err, "error getting tags for plugin %v", p.Name)
				}

				// bucket
				if out.LatestRevision == nil {
					return nil, errors.Wrapf(err, "error getting latest revision for plugin %v", p.Name)
				}
				bucket := strings.TrimPrefix(*out.LatestRevision.Location.S3Location.BucketArn, "arn:aws:s3:::")

				// file content
				sha, err := awsw.NewS3(ctx, p.Context.ProviderName).GetObjectChecksum(ctx, bucket, *out.LatestRevision.Location.S3Location.FileKey, out.LatestRevision.Location.S3Location.ObjectVersion)
				if err != nil {
					return nil, errors.Wrapf(err, "error getting checksum for object %v:%v", bucket, *out.LatestRevision.Location.S3Location.FileKey)
				}
				if sha == nil {
					return nil, fmt.Errorf("checksum not found %v:%v", bucket, *out.LatestRevision.Location.S3Location.FileKey)
				}

				existing := &MSKPlugin{
					Name:        util.Coalesce(out.Name, ""),
					Description: out.Description,
					Code: Code{
						_checksum: sha,
					},
					Tags:   tags.Tags,
					_arn:   out.CustomPluginArn,
					_state: (*string)(&out.CustomPluginState),
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
