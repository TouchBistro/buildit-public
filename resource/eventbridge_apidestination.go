package resource

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	eventbridgetypes "github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
)

// EventBridgeApiDestination represents eventbridge API Destination
type EventBridgeApiDestination struct {
	BaseResource `yaml:",inline"`
	Arn            string  `yaml:"-"` // loaded when read from aws
	Name           string  `yaml:"name"`
	Description    *string `yaml:"description"`
	Method         *string `yaml:"method"`
	Endpoint       *string `yaml:"endpoint"`
	InvocationRate *int32  `yaml:"invocationRateLimitPerSecond"`
	ConnectionName string  `yaml:"connectionName"`
	DependsOn      []Key   `yaml:"dependsOn"`
}

// Key returns the unique key for the resource for this buildit context
func (d EventBridgeApiDestination) Key() Key {
	return NewKey(d.Context.ProviderName, d.Identifier())
}

// Identifier returns the name for the API destination
func (d EventBridgeApiDestination) Identifier() string {
	return d.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (d *EventBridgeApiDestination) Normalize(ctx context.Context) {

	if d.Method == nil {
		d.Method = aws.String(string(eventbridgetypes.ApiDestinationHttpMethodGet))
	} else {
		d.Method = aws.String(strings.ToUpper(*d.Method))
	}

	if d.InvocationRate == nil {
		d.InvocationRate = aws.Int32(0)
	}
}

// Validate checks that the input provided is correct
func (d EventBridgeApiDestination) Validate(ctx context.Context) error {

	var errMessages []string

	switch *d.Method {
	case string(eventbridgetypes.ApiDestinationHttpMethodDelete),
		string(eventbridgetypes.ApiDestinationHttpMethodGet),
		string(eventbridgetypes.ApiDestinationHttpMethodHead),
		string(eventbridgetypes.ApiDestinationHttpMethodOptions),
		string(eventbridgetypes.ApiDestinationHttpMethodPatch),
		string(eventbridgetypes.ApiDestinationHttpMethodPost),
		string(eventbridgetypes.ApiDestinationHttpMethodPut):
		// no-op
	default:
		msg := fmt.Sprintf("invalid method %q supplied", *d.Method)
		errMessages = append(errMessages, msg)
	}

	if d.Endpoint == nil {
		errMessages = append(errMessages, "a valid endpoint url must be supplied")
	} else {
		uri, err := url.Parse(*d.Endpoint)
		if err != nil {
			errMessages = append(errMessages, "a valid endpoint url must be supplied, %q is invalid format", *d.Endpoint)
		}
		if strings.ToLower(uri.Scheme) != "https" {
			errMessages = append(errMessages, "supplied endpoint url must use https schema, %q is invalid", strings.ToLower(uri.Scheme))
		}
	}

	if *d.InvocationRate <= 0 {
		errMessages = append(errMessages, "a non-zero value for invocationRateLimitPerSecond must be supplied ")
	}

	if len(d.ConnectionName) == 0 {
		errMessages = append(errMessages, "the api connection must be supplied ")
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: d.Identifier(),
		ResourceType:       "EventBridgeApiDestination",
		Messages:           errMessages,
	}
}

// Apply provisions or updates an eventbridge api destination & the underlying api connection
func (d EventBridgeApiDestination) Apply(ctx context.Context) error {
	log.Debugf("creating eventbridge api destination %v", d.Name)

	diffs, err := d.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": d.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", d.Identifier()).Info("eventbridge api destination already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}

		err = d.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update eventbridge api destination %v", d.Identifier())
		}
		return nil
	}

	return d.apply(ctx)
}

// Destroy deletes an eventbridge api destination & connection
func (d EventBridgeApiDestination) Destroy(ctx context.Context) error {

	log.Debugf("destroying api destination %v", d.Identifier())

	existing, err := d.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding eventbridge api destination %v", d.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": d.Identifier(),
		}).Info("eventbridge api destination does not exist, nothing to destroy, skippping ")
		return nil
	}

	ebClient := client.EventBridge(ctx, d.Context.ProviderName)

	_, err = ebClient.DeleteApiDestination(ctx, &eventbridge.DeleteApiDestinationInput{
		Name: aws.String(d.Name),
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting api destination  %v", d.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": d.Name,
	}).Info(color.Red("eventbridge api destination destroyed"))

	return nil
}

// EventBridgeApiDestinationDiff represents the diff between the config & aws resource
type EventBridgeApiDestinationDiff struct {
	BaseResourceDiff

	descriptionDiff bool
	methodDiff      bool
	endpointDiff    bool
	rateDiff        bool
	connectionDiff  bool
}

// Compare fetches the resource from AWS and compares with this config, returns a diff object
// or returns an error if something goes wrong
func (d EventBridgeApiDestination) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := d.fetchExisting(ctx)
	if err != nil {
		return nil, err
	}

	diffsFound := false
	diffs := &EventBridgeApiDestinationDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "eventbridge api destination does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	if util.CoalesceComparable(d.Description, "") != util.CoalesceComparable(existing.Description, "") {
		diffsFound = true
		diffs.descriptionDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("description is different for destination %q", d.Name))
	}

	if util.CoalesceComparable(d.Endpoint, "") != util.CoalesceComparable(existing.Endpoint, "") {
		diffsFound = true
		diffs.endpointDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("endpoint is different for destination %q", d.Name))
	}

	if util.CoalesceComparable(d.Method, "") != util.CoalesceComparable(existing.Method, "") {
		diffsFound = true
		diffs.methodDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("method is different for destination %q", d.Name))
	}

	if util.CoalesceComparable(d.InvocationRate, 0) != util.CoalesceComparable(existing.InvocationRate, 0) {
		diffsFound = true
		diffs.rateDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("invocation rate per second is different for destination %q", d.Name))
	}

	if d.ConnectionName != existing.ConnectionName {
		diffsFound = true
		diffs.connectionDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("api connection is different for destination %q", d.Name))
	}

	if !diffsFound {
		return nil, nil
	}

	return diffs, nil
}

// apply creates a new eventbridge api destination & connection
func (d EventBridgeApiDestination) apply(ctx context.Context) error {

	log.Debugf("creating api destination %v", d.Name)

	connectionArn, err := awsw.NewEventBridge(ctx, d.Context.ProviderName).ConnectionArnForName(ctx, d.ConnectionName)
	if err != nil {
		return err
	}

	ebClient := client.EventBridge(ctx, d.Context.ProviderName)
	out, err := ebClient.CreateApiDestination(ctx, &eventbridge.CreateApiDestinationInput{
		Name:                         aws.String(d.Name),
		Description:                  d.Description,
		ConnectionArn:                connectionArn,
		HttpMethod:                   eventbridgetypes.ApiDestinationHttpMethod(*d.Method),
		InvocationEndpoint:           d.Endpoint,
		InvocationRateLimitPerSecond: d.InvocationRate,
	})

	if err != nil {
		return err
	}

	apidestState := out.ApiDestinationState

	log.WithFields(log.Fields{
		"Name":  d.Name,
		"State": string(apidestState),
	}).Info(color.Green("eventbridge api destination created"))

	return nil
}

// applyDiffs updates the resource with the supplied diffs
func (d EventBridgeApiDestination) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": d.Identifier(),
		}).Info("no updates required for eventbridge api destination")
		return nil
	}

	adDiffs, ok := diffs.(*EventBridgeApiDestinationDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing sg
	_, ok = adDiffs.Resource.(*EventBridgeApiDestination)
	if !ok {
		return errors.Errorf("cannot retrieve existing eventbridge api destination")
	}

	var err error
	client := client.EventBridge(ctx, d.Context.ProviderName)

	updateInput := &eventbridge.UpdateApiDestinationInput{
		Name: aws.String(d.Name),
	}

	if adDiffs.descriptionDiff {
		updateInput.Description = d.Description
	}

	if adDiffs.endpointDiff {
		updateInput.InvocationEndpoint = d.Endpoint
	}

	if adDiffs.methodDiff {
		updateInput.HttpMethod = eventbridgetypes.ApiDestinationHttpMethod(*d.Method)
	}

	if adDiffs.rateDiff {
		updateInput.InvocationRateLimitPerSecond = d.InvocationRate
	}

	if adDiffs.connectionDiff {
		arn, err := awsw.NewEventBridge(ctx, d.Context.ProviderName).ConnectionArnForName(ctx, d.ConnectionName)
		if err != nil {
			return err
		}
		updateInput.ConnectionArn = arn
	}

	_, err = client.UpdateApiDestination(ctx, updateInput)
	if err != nil {
		return err
	}
	log.WithField("Name", d.Identifier()).Info(color.Yellow("eventbridge api destination updated"))

	return nil
}

// fetchExisting fetches the existing Api Destination or error
func (d EventBridgeApiDestination) fetchExisting(ctx context.Context) (*EventBridgeApiDestination, error) {

	ebClient := client.EventBridge(ctx, d.Context.ProviderName)
	outl, err := ebClient.ListApiDestinations(ctx, &eventbridge.ListApiDestinationsInput{
		NamePrefix: aws.String(d.Name),
	})

	if err != nil {
		return nil, errors.Wrapf(err, "error fetching api destination %v", d.Name)
	}

	if len(outl.ApiDestinations) == 0 {
		return nil, nil
	}

	out, err := ebClient.DescribeApiDestination(ctx, &eventbridge.DescribeApiDestinationInput{
		Name: aws.String(d.Name),
	})

	if err != nil {
		return nil, errors.Wrapf(err, "error fetching api destination %v", d.Name)
	}

	connectionName, err := awsw.NewEventBridge(ctx, d.Context.ProviderName).ConnectionNameForArn(ctx, *out.ConnectionArn)
	if err != nil {
		return nil, err
	}

	if connectionName == nil {
		return nil, errors.Errorf("cannot find eventbridge connection for arn %v", *out.ConnectionArn)
	}

	dest := &EventBridgeApiDestination{
		Arn:            *out.ApiDestinationArn,
		Name:           *out.Name,
		Description:    out.Description,
		Method:         (*string)(&out.HttpMethod),
		Endpoint:       out.InvocationEndpoint,
		InvocationRate: out.InvocationRateLimitPerSecond,
		ConnectionName: *connectionName,
	}

	return dest, nil
}
