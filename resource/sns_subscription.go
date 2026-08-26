package resource

import (
	"context"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/goutils/color"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"

	log "github.com/sirupsen/logrus"
)

// SNSSubscription represents a SNS subscription rule; only Lambda subcribers are supported
type SNSSubscription struct {
	BaseResource `yaml:",inline"`
	Name         string `yaml:"-"`
	Protocol     string `yaml:"protocol"`
	TopicName    string `yaml:"topicName"`
	EndpointName string `yaml:"endpointName"`
	TopicArn     *string
	EndpointArn  *string
	DependsOn    []Key `yaml:"dependsOn"`
}

// Key returns the unique key for the resource for this buildit context
func (s SNSSubscription) Key() Key {
	return NewKey(s.Context.ProviderName, s.Identifier())
}

// Identifier returns topic and endpoint name of the SNS subscription
func (s SNSSubscription) Identifier() string {
	return s.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (s *SNSSubscription) Normalize(ctx context.Context) {
	if len(s.Protocol) == 0 {
		s.Protocol = "lambda"
	}

	// topic arn
	topic, err := awsw.NewSNS(ctx, s.Context.ProviderName).TopicArnForName(ctx, s.TopicName)
	if err != nil {
		panic(err)
	}
	if topic == nil {
		panic(errors.Errorf("cannot find topic %v ", s.TopicName))
	}
	s.TopicArn = topic

	// endpoint arn
	name, qualifier := SplitNameQualifier(s.EndpointName)
	endpoint, err := awsw.NewLambda(ctx, s.Context.ProviderName).LambdaArnForNameAndQualifier(ctx, name, qualifier)
	if err != nil {
		panic(err)
	}
	if endpoint == nil {
		panic(errors.Errorf("cannot find endpoint %v ", s.EndpointName))
	}
	s.EndpointArn = endpoint
}

// Validate checks that the input provided is correct
func (s SNSSubscription) Validate(ctx context.Context) error {

	var errMessages []string

	// topic name not found
	if len(s.TopicName) == 0 {
		errMessages = append(errMessages, "Have to supply a topic name")
	}
	// endpoint name not found
	if len(s.EndpointName) == 0 {
		errMessages = append(errMessages, "Have to supply an endpoint name")
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceType: "SNS Subscription",
		Messages:     errMessages,
	}
}

// Apply creates a new SNS subscription
func (s SNSSubscription) Apply(ctx context.Context) error {
	diffs, err := s.Compare(ctx)
	if err != nil {
		return err
	}

	//create if diff is found
	if diffs != nil {
		log.Debugf("creating subscription: %v", s.Identifier())
		err = s.apply(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to create subscription: %v", s.Identifier())
		}
		return nil
	}

	log.WithFields(log.Fields{
		"Topic":    s.TopicName,
		"Endpoint": s.EndpointName,
	}).Info("no updates required")
	return nil

}

// apply provisions a new SNS subscription & it's targets
func (s SNSSubscription) apply(ctx context.Context) error {
	snsClient := client.SNS(ctx, s.Context.ProviderName)
	_, err := snsClient.Subscribe(ctx, &sns.SubscribeInput{
		Protocol: aws.String(s.Protocol),
		TopicArn: aws.String(*s.TopicArn),
		Endpoint: aws.String(*s.EndpointArn),
	})

	if err != nil {
		return errors.Wrapf(err, "error creating or updating SNS subscription %v", s.Identifier())
	}

	log.WithFields(log.Fields{
		"Topic":    s.TopicName,
		"Endpoint": s.EndpointName,
	}).Info(color.Green("SNS subscription created"))

	return nil
}

// SNS subscription diff
type SNSSubscriptionDiff struct {
	BaseResourceDiff
}

// Compare fetches the existing SNS subscription & if it exists returns nil, else returns the diffs
func (s SNSSubscription) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := s.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching existing SNS subscription %v", s.Identifier())
	}

	diffs := &SNSSubscriptionDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "SNS subscription does not exist")
		return diffs, nil
	}

	return nil, nil
}

// Destroy removes the SNS subscription
func (s SNSSubscription) Destroy(ctx context.Context) error {
	log.Debugf("destroying SNS subcription: %v", s.Identifier())

	existing, err := s.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding SNS subscription: %v", s.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Topic":    s.TopicName,
			"Endpoint": s.EndpointName,
		}).Info("SNS subscription rule does not exist, nothing to destroy, skippping ")
		return nil
	}

	snsClient := client.SNS(ctx, s.Context.ProviderName)

	//now delete the rule
	_, err = snsClient.Unsubscribe(ctx, &sns.UnsubscribeInput{
		SubscriptionArn: aws.String(*existing.SubscriptionArn),
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting SNS subscription: %v", s.Identifier())
	}

	log.WithFields(log.Fields{
		"Topic":    s.TopicName,
		"Endpoint": s.EndpointName,
	}).Info(color.Red("SNS subscription destroyed"))

	return nil
}

// fetchExisting returns the existing subscription rule details if found, and set TopicArn and EndpointArn
func (s SNSSubscription) fetchExisting(ctx context.Context) (*snstypes.Subscription, error) {
	snsClient := awsw.NewSNS(ctx, s.Context.ProviderName)
	nextToken := ""
	for {
		out, err := snsClient.ListSubscriptionsByTopic(ctx, &sns.ListSubscriptionsByTopicInput{
			TopicArn:  aws.String(*s.TopicArn),
			NextToken: aws.String(nextToken),
		})

		if err != nil {
			return nil, errors.Wrapf(err, "error while listing SNS subscription")
		}

		if len(out.Subscriptions) == 0 {
			return nil, nil
		}

		for _, sub := range out.Subscriptions {
			if *s.EndpointArn == *sub.Endpoint {
				return &sub, nil
			}
		}
		if out.NextToken == nil {
			break
		}
		nextToken = *out.NextToken
	}
	return nil, nil
}
