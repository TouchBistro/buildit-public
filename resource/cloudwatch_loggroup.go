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
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	types "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

const (
	DefaultLogRetention = 731
)

// CWLogGroup represents a CloudWatch LogGroup
type CWLogGroup struct {
	BaseResource `yaml:",inline"`
	Name          string                   `yaml:"-"`
	Retention     *int32                   `yaml:"retention"`
	MetricFilters []CWLogGroupMetricFilter `yaml:"metricFilters"`
	GlobalTags    map[string]string        `yaml:"-"`
	Tags          map[string]string        `yaml:"tags"`
	DependsOn     []Key                    `yaml:"dependsOn"`
	Arn           *string                  // filled by fetchExisting()
}

// CWLogGroupMetricFilters represents a CloudWatch LogGroup Metric Filter
type CWLogGroupMetricFilter struct {
	Name                  string                                 `yaml:"name"`
	Pattern               string                                 `yaml:"pattern"`
	MetricTransformations []CWLogGroupMetricFilterTransformation `yaml:"metricTransformations"`
}

// equals returns true if this metric filter is the same as other, else false
func (mf CWLogGroupMetricFilter) equals(other CWLogGroupMetricFilter) bool {

	if mf.Name != other.Name || mf.Pattern != other.Pattern {
		return false
	}

	if len(mf.MetricTransformations) != len(other.MetricTransformations) {
		return false
	} else {
		for n, t := range mf.MetricTransformations {
			t2 := other.MetricTransformations[n]

			if t.Name != t2.Name {
				return false
			}

			if t.Namespace != t2.Namespace {
				return false
			}

			if util.CoalesceComparable(t.Unit, "None") != util.CoalesceComparable(t2.Unit, "None") {
				return false
			}

			if t.Value != t2.Value {
				return false
			}

			if t.DefaultValue != t2.DefaultValue {
				return false
			}

			if len(t.Dimensions) != len(t2.Dimensions) {
				return false
			} else {
				for k, v := range t.Dimensions {
					if t2v, ok := t2.Dimensions[k]; !ok {
						return false
					} else {
						if v != t2v {
							return false
						}
					}
				}
			}
		}
	}
	return true
}

// CWLogGroupMetricFilterTransformations represents a CloudWatch LogGroup Metric Filter Transformation
type CWLogGroupMetricFilterTransformation struct {
	Name         string            `yaml:"name"`
	Namespace    string            `yaml:"namespace"`
	Value        string            `yaml:"value"`
	DefaultValue float64           `yaml:"defaultValue"`
	Dimensions   map[string]string `yaml:"dimensions"`
	Unit         *string           `yaml:"unit"`
}

// Key returns the unique key for the resource for this buildit context
func (c CWLogGroup) Key() Key {
	return NewKey(c.Context.ProviderName, c.Identifier())
}

// Identifier returns name of the cloudwatch log group
func (c CWLogGroup) Identifier() string {
	return c.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (c *CWLogGroup) Normalize(ctx context.Context) {

	if c.Retention == nil {
		c.Retention = aws.Int32(DefaultLogRetention)
	}

	// merge global tags to resource tags
	if c.Tags == nil {
		c.Tags = make(map[string]string)
	}
	ResourceTags(c.Tags).Merge(c.GlobalTags)
}

// Validate checks that the input provided is correct
func (c CWLogGroup) Validate(ctx context.Context) error {

	var errMessages []string

	if c.Retention != nil {
		switch *c.Retention {
		case 1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1827, 3653:
		default:
			msg := "invalid value for retention (days), can be one of 1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1827, 3653"
			errMessages = append(errMessages, msg)
		}
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: c.Identifier(),
		ResourceType:       "CloudWatch LogGroup",
		Messages:           errMessages,
	}
}

// Apply creates a new cloudwatch log group and any log metric filters
func (c CWLogGroup) Apply(ctx context.Context) error {
	log.Debugf("creating cloudwatch log group %v", c.Identifier())

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
		log.WithField("Name", c.Identifier()).Info("cloudwatch log group already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = c.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update cloudwatch log group %v", c.Identifier())
		}
		return nil
	}

	return c.apply(ctx)
}

// Destroy removes the cloudwatch log group and any log metric filters
func (c CWLogGroup) Destroy(ctx context.Context) error {

	cwlClient := client.CloudWatchLogs(ctx, c.Context.ProviderName)

	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding existing cloudwatch log group %v", c.Name)
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": c.Name,
		}).Info("cloudWatch log group does not exist, nothing to destroy, skipping")
		return nil
	}

	_, err = cwlClient.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(c.Name),
	})

	if err != nil {
		return errors.Wrapf(err, "error destroying existing cloudwatch log group %v", c.Name)
	}

	log.WithFields(log.Fields{
		"Name": c.Name,
	}).Info(color.Red("cloudwatch loggroup deleted"))
	return nil
}

// fetchExisting returns the existing cloudwatch log group details if found
func (c CWLogGroup) fetchExisting(ctx context.Context) (*CWLogGroup, error) {

	cwlClient := client.CloudWatchLogs(ctx, c.Context.ProviderName)
	done := false
	var token *string

	for !done {
		out, err := cwlClient.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{
			Limit:              aws.Int32(50),
			LogGroupNamePrefix: aws.String(c.Name),
			NextToken:          token,
		})

		if err != nil {
			return nil, errors.Wrapf(err, "error describing logs groups %v", c.Name)
		}

		for _, group := range out.LogGroups {
			if *group.LogGroupName == c.Name {

				arn := *group.Arn
				if strings.HasSuffix(arn, ":*") { //if the existing arn returned by describeLogGroup API has an `:*` at the end..
					arn = arn[:strings.LastIndex(arn, ":")] //strip out everything including & after the last ":"
				}
				grp := &CWLogGroup{
					Name:      *group.LogGroupName,
					Retention: group.RetentionInDays,
					Arn:       aws.String(arn),
				}
				// fet metric filters
				outf, err := cwlClient.DescribeMetricFilters(ctx, &cloudwatchlogs.DescribeMetricFiltersInput{
					LogGroupName: group.LogGroupName,
				})

				if err != nil {
					return nil, errors.Wrapf(err, "error fetching cloudwatch log group filers for %v", c.Identifier())
				}

				if len(outf.MetricFilters) > 0 {
					grp.MetricFilters = make([]CWLogGroupMetricFilter, 0)
					for _, f := range outf.MetricFilters {
						transformations := make([]CWLogGroupMetricFilterTransformation, 0)
						for _, t := range f.MetricTransformations {
							transformations = append(transformations, CWLogGroupMetricFilterTransformation{
								Name:         *t.MetricName,
								Namespace:    *t.MetricNamespace,
								Value:        *t.MetricValue,
								DefaultValue: *t.DefaultValue,
								Unit:         aws.String(string(t.Unit)),
								Dimensions:   t.Dimensions,
							})
						}
						grp.MetricFilters = append(grp.MetricFilters, CWLogGroupMetricFilter{
							Name:                  *f.FilterName,
							Pattern:               *f.FilterPattern,
							MetricTransformations: transformations,
						})
					}
				}

				return grp, nil
			}
		}
		token = out.NextToken
		done = token == nil
	}
	return nil, nil
}

type CWLogGroupDiff struct {
	BaseResourceDiff

	diffRetention     bool
	metricfiltersDiff bool

	tagsDiff bool
	tagDiff  util.TagDiffResult
}

// Compare fetches the existing resource from AWS & compares, returns the diffs
func (c CWLogGroup) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", c.Identifier())
	}

	diffs := &CWLogGroupDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "cloudwatch log group does not exist")
		return diffs, nil
	}

	diffs.Resource = existing
	log.Debugf("updating cloudwatch log group for %v", c.Identifier())
	found := false

	// retention
	if util.CoalesceComparable(c.Retention, 0) != util.CoalesceComparable(existing.Retention, 0) {
		found = true
		diffs.diffRetention = true
		diffs.Messages = append(diffs.Messages, "retention is not the same")
	}

	// metric filters
	if len(c.MetricFilters) > len(existing.MetricFilters) {
		found = true
		diffs.metricfiltersDiff = true
		diffs.Messages = append(diffs.Messages, "additional metric filters defined than what exist")
	} else if len(existing.MetricFilters) > len(c.MetricFilters) {
		found = true
		diffs.metricfiltersDiff = true
		diffs.Messages = append(diffs.Messages, "less metric filters defined than what exist")
	} else {
		for n, f := range c.MetricFilters {
			if !f.equals(existing.MetricFilters[n]) {
				found = true
				diffs.metricfiltersDiff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("metric filter at index %v is not the same", n))
			}
		}
	}

	// tags
	awsTags, err := awsw.NewCloudWatchLogs(ctx, c.Context.ProviderName).GetResourceTags(ctx, *existing.Arn)
	if err != nil {
		return nil, err
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, c.Tags); tagDiff.HasChanges() {
		found = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if !found {
		return nil, nil
	}

	return diffs, nil
}

// apply creates a new cloudwatch log group and any log metric filters
func (c CWLogGroup) apply(ctx context.Context) error {

	log.Debugf("creating cloudwatch log group for %v", c.Name)

	// tags
	tags := make(map[string]string)
	for k, v := range c.GlobalTags {
		tags[k] = v
	}

	// kms
	cwlClient := client.CloudWatchLogs(ctx, c.Context.ProviderName)
	_, err := cwlClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: aws.String(c.Name),
		Tags:         tags,
	})

	if err != nil {
		return errors.Wrapf(err, "error creating cloudwatch log group %v", c.Name)
	}

	// metric filters
	err = c.putFilterMetrics(ctx, cwlClient)

	if err != nil {
		return errors.Wrapf(err, "error adding log metric filters to log group %v", c.Name)
	}

	// Retention Policy
	if c.Retention != nil {
		_, err = cwlClient.PutRetentionPolicy(ctx, &cloudwatchlogs.PutRetentionPolicyInput{
			LogGroupName:    aws.String(c.Name),
			RetentionInDays: c.Retention,
		})

		if err != nil {
			return errors.Wrapf(err, "error adding log metric retention policy to log group %v", c.Name)
		}
	}

	log.WithFields(log.Fields{
		"Name":       c.Name,
		"NumFilters": len(c.MetricFilters),
	}).Info(color.Green("cloudWatch loggroup created"))

	return nil
}

// applyDiffs applies diff to the existing resourece
func (c CWLogGroup) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("no updates required for cloudwatch log group")
		return nil
	}

	logDiffs, ok := diffs.(*CWLogGroupDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing log group
	existing, ok := logDiffs.Resource.(*CWLogGroup)
	if !ok {
		return errors.Errorf("cannot retrieve existing cloudwatch log group")
	}

	// apply updates
	cwlClient := client.CloudWatchLogs(ctx, c.Context.ProviderName)

	if logDiffs.diffRetention {
		_, err := cwlClient.PutRetentionPolicy(ctx, &cloudwatchlogs.PutRetentionPolicyInput{
			LogGroupName:    aws.String(existing.Name),
			RetentionInDays: c.Retention,
		})

		if err != nil {
			return errors.Wrapf(err, "error updating log retention policy for group %v", existing.Name)
		}
	}

	// metric filtes
	// to simplify the updates, if diffs are found on metric filters, we delete all existing filter &
	// recreate all defined filters
	if logDiffs.metricfiltersDiff {
		// delete existing metric filters
		for _, filter := range existing.MetricFilters {
			_, err := cwlClient.DeleteMetricFilter(ctx, &cloudwatchlogs.DeleteMetricFilterInput{
				LogGroupName: aws.String(existing.Name),
				FilterName:   aws.String(filter.Name),
			})

			if err != nil {
				return errors.Wrapf(err, "error deleting existing cloudwatch log group filer metric for %v/%v", c.Identifier(), filter.Name)
			}
		}

		err := c.putFilterMetrics(ctx, cwlClient)
		if err != nil {
			return errors.Wrapf(err, "error adding log metric filters to log group %v", c.Name)
		}
	}

	// tags
	if logDiffs.tagsDiff {
		upserts := logDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			err := awsw.NewCloudWatchLogs(ctx, c.Context.ProviderName).AddResourceTags(ctx, *existing.Arn, upserts)
			if err != nil {
				return errors.Wrapf(err, "error updating cloudwatch loggroup tags for %v", c.Identifier())
			}
		}

		if len(logDiffs.tagDiff.Deleted) > 0 {
			err := awsw.NewCloudWatchLogs(ctx, c.Context.ProviderName).DeleteResourceTags(ctx, *existing.Arn, logDiffs.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error deleting cloudwatch loggroup tags for %v", c.Identifier())
			}
		}
	}

	log.WithFields(log.Fields{
		"Name": c.Identifier(),
	}).Info(color.Yellow("cloudwatch log group updated"))

	return nil
}

// putFilterMetrics adds the specified filter metrics to the cloudwatch log group
func (c CWLogGroup) putFilterMetrics(ctx context.Context, cwlClient *cloudwatchlogs.Client) error {

	for _, filter := range c.MetricFilters {
		var transformations []types.MetricTransformation
		for _, trans := range filter.MetricTransformations {
			if trans.Unit == nil {
				trans.Unit = aws.String(string(types.StandardUnitNone))
			}
			transformation := types.MetricTransformation{
				MetricName:      aws.String(trans.Name),
				MetricNamespace: aws.String(trans.Namespace),
				MetricValue:     aws.String(trans.Value),
				DefaultValue:    aws.Float64(trans.DefaultValue),
				Dimensions:      trans.Dimensions,
				Unit:            types.StandardUnit(*trans.Unit),
			}
			transformations = append(transformations, transformation)
		}

		_, err := cwlClient.PutMetricFilter(ctx, &cloudwatchlogs.PutMetricFilterInput{
			FilterName:            aws.String(filter.Name),
			LogGroupName:          aws.String(c.Name),
			FilterPattern:         aws.String(filter.Pattern),
			MetricTransformations: transformations,
		})

		if err != nil {
			return errors.Wrapf(err, "error putting log metric filter %v for cloud watch group %v", filter.Name, c.Identifier())
		}
	}
	return nil
}
