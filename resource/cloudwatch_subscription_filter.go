package resource

import (
	"context"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/goutils/color"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwlTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"

	log "github.com/sirupsen/logrus"
)

// CWSubscriptionFilter represents a cloudWatch subscription filter rule; only Lambda subcribers are supported
type CWSubscriptionFilter struct {
	BaseResource `yaml:",inline"`
	Name           string  `yaml:"-"`
	FilterName     *string `yaml:"filterName"`
	Destination    string  `yaml:"destination"` //Only support Lambda for now
	FilterPattern  string  `yaml:"filterPattern"`
	LogGroup       string  `yaml:"logGroup"`
	DestinationArn *string
	DependsOn      []Key `yaml:"dependsOn"`
}

// Key returns the unique key for the resource for this buildit context
func (c CWSubscriptionFilter) Key() Key {
	return NewKey(c.Context.ProviderName, c.Identifier())
}

// Identifier returns topic and endpoint name of the cloudWatch subscription filter
func (c CWSubscriptionFilter) Identifier() string {
	return c.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (c *CWSubscriptionFilter) Normalize(ctx context.Context) {
	if c.FilterName == nil {
		c.FilterName = aws.String(c.Identifier())
	}

	err := c.setEndpointArn(ctx)
	if err != nil {
		panic(err)
	}

}

// Validate checks that the input provided is correct
func (c CWSubscriptionFilter) Validate(ctx context.Context) error {

	var errMessages []string

	if len(c.Destination) == 0 {
		errMessages = append(errMessages, "destination must be supplied")
	}
	if len(c.LogGroup) == 0 {
		errMessages = append(errMessages, "logGroup must be supplied")
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceType: "cloudWatch subscription filter",
		Messages:     errMessages,
	}
}

// Apply creates a new cloudWatch subscription filter
func (c CWSubscriptionFilter) Apply(ctx context.Context) error {
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
		log.WithField("Name", c.Identifier()).Info("subscription filter already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}

		err = c.applyDiffs(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to update eventbridge rule %v", c.Identifier())
		}
		return nil
	}

	return c.apply(ctx)
}

// apply provisions a new cloudWatch subscription filter
func (c CWSubscriptionFilter) apply(ctx context.Context) error {
	cwlClient := client.CloudWatchLogs(ctx, c.Context.ProviderName)
	_, err := cwlClient.PutSubscriptionFilter(ctx, &cloudwatchlogs.PutSubscriptionFilterInput{
		DestinationArn: aws.String(*c.DestinationArn),
		FilterName:     c.FilterName,
		FilterPattern:  aws.String(c.FilterPattern),
		LogGroupName:   aws.String(c.LogGroup),
	})

	if err != nil {
		return errors.Wrapf(err, "error creating cloudWatch subscription filter %v", c.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": c.Identifier(),
	}).Info(color.Green("cloudWatch subscription filter created"))

	return nil
}

// applyDiffs update an existing cloudWatch subscription filter
func (c CWSubscriptionFilter) applyDiffs(ctx context.Context) error {
	cwlClient := client.CloudWatchLogs(ctx, c.Context.ProviderName)
	_, err := cwlClient.PutSubscriptionFilter(ctx, &cloudwatchlogs.PutSubscriptionFilterInput{
		DestinationArn: aws.String(*c.DestinationArn),
		FilterName:     c.FilterName,
		FilterPattern:  aws.String(c.FilterPattern),
		LogGroupName:   aws.String(c.LogGroup),
	})

	if err != nil {
		return errors.Wrapf(err, "error updating cloudWatch subscription filter %v", c.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": c.Identifier(),
	}).Info(color.Yellow("cloudWatch subscription filter updated"))

	return nil
}

// cloudWatch subscription filter diff
type CWSubscriptionFilterDiff struct {
	BaseResourceDiff
	FilterPattern  bool
	DestinationArn bool
}

// Compare fetches the existing cloudWatch subscription filter & if it exists returns nil, else returns the diffs
func (c CWSubscriptionFilter) Compare(ctx context.Context) (ResourceDiff, error) {
	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching existing cloudWatch subscription filter %v", c.Identifier())
	}

	diffs := &CWSubscriptionFilterDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "cloudWatch subscription filter does not exist")
		return diffs, nil
	}

	diff := false
	diffs.Resource = existing

	// filter pattern
	if c.FilterPattern != *existing.FilterPattern {
		diff = true
		diffs.FilterPattern = true
		diffs.Messages = append(diffs.Messages, "cloudWatch subscription filter pattern is not the same")
	}

	if *c.DestinationArn != *existing.DestinationArn {
		diff = true
		diffs.DestinationArn = true
		diffs.Messages = append(diffs.Messages, "cloudWatch subscription destination arn is not the same")
	}

	if diff {
		return diffs, nil
	}

	return nil, nil
}

// Destroy removes the cloudWatch subscription filter
func (c CWSubscriptionFilter) Destroy(ctx context.Context) error {
	log.Debugf("destroying SNS subcription: %v", c.Identifier())

	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding cloudWatch subscription filter: %v", c.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("cloudWatch subscription filter rule does not exist, nothing to destroy, skippping ")
		return nil
	}

	cwlClient := client.CloudWatchLogs(ctx, c.Context.ProviderName)

	_, err = cwlClient.DeleteSubscriptionFilter(ctx, &cloudwatchlogs.DeleteSubscriptionFilterInput{
		FilterName:   c.FilterName,
		LogGroupName: aws.String(c.LogGroup),
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting cloudWatch subscription filter: %v", c.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": c.Identifier(),
	}).Info(color.Red("cloudWatch subscription filter destroyed"))

	return nil
}

// return the ARN of the provided endpoint
func (c *CWSubscriptionFilter) setEndpointArn(ctx context.Context) error {
	name, qualifier := SplitNameQualifier(c.Destination)
	arn, err := awsw.NewLambda(ctx, c.Context.ProviderName).LambdaArnForNameAndQualifier(ctx, name, qualifier)
	if err != nil {
		return errors.Wrapf(err, "error finding endpoint: %v", c.Destination)
	}
	if arn == nil {
		return errors.Errorf("cannot find endpoint %v ", c.Destination)
	}
	c.DestinationArn = arn
	return nil
}

// fetchExisting returns the existing subscription rule details if found, and set TopicArn and EndpointArn
func (c CWSubscriptionFilter) fetchExisting(ctx context.Context) (*cwlTypes.SubscriptionFilter, error) {
	cwlClient := awsw.NewCloudWatchLogs(ctx, c.Context.ProviderName)

	// can return at most 2 filters
	out, err := cwlClient.DescribeSubscriptionFilters(ctx, &cloudwatchlogs.DescribeSubscriptionFiltersInput{
		LogGroupName:     aws.String(c.LogGroup),
		FilterNamePrefix: c.FilterName,
	})

	if err != nil {
		return nil, errors.Wrapf(err, "error listing cloudWatch subscription filter")
	}

	if len(out.SubscriptionFilters) == 0 {
		return nil, nil
	}

	for _, sub := range out.SubscriptionFilters {
		if *c.FilterName == *sub.FilterName {
			return &sub, nil
		}
	}
	return nil, nil
}
