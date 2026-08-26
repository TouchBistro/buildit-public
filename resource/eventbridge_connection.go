package resource

import (
	"context"
	"fmt"
	"reflect"

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

// EventBridgeApiConnection contains the Connection Info for an Api Destination
type EventBridgeApiConnection struct {
	BaseResource `yaml:",inline"`
	Arn                  string                           `yaml:"-"`
	Name                 string                           `yaml:"name"`
	Description          *string                          `yaml:"description"`
	ConnectionParameters *EventBridgeConnectionParameters `yaml:"connectionParameters"`
	Tags                 map[string]string                `yaml:"tags"`
	GlobalTags           map[string]string                `yaml:"-"`
	DependsOn            []Key                            `yaml:"dependsOn"`
}

// EventBridgeConnectionParameters contains the connection auth & additional parameters
type EventBridgeConnectionParameters struct {
	ApiKeyName  *string                          `yaml:"apiKeyName" json:"api_key_name,omitempty"`
	ApiKeyValue *string                          `yaml:"apiKeyValue" json:"api_key_value,omitempty"`
	Parameters  *EventBridgeInvocationParameters `yaml:"invocationParameters" json:"invocation_http_parameters,omitempty"`
}

// EventBridgeInvocationParameters contains the header, body or querystring parameters
type EventBridgeInvocationParameters struct {
	HeaderParameters      []EventBridgeParameter `yaml:"headerParameters" json:"header_parameters,omitempty"`
	BodyParameters        []EventBridgeParameter `yaml:"bodyParameters" json:"body_parameters,omitempty"`
	QueryStringParameters []EventBridgeParameter `yaml:"queryStringParameters" json:"query_string_parameters,omitempty"`
}

// EventBridgeParameter represents a single header, body or query string parameters key, value o
type EventBridgeParameter struct {
	Key           *string `yaml:"key" json:"key,omitempty"`
	Value         *string `yaml:"value" json:"value,omitempty"`
	Secret        *string `yaml:"secret"`
	IsValueSecret bool    `yaml:"-" json:"is_value_secret,omitempty"`
}

// Key returns the unique key for the resource for this buildit context
func (c EventBridgeApiConnection) Key() Key {
	return NewKey(c.Context.ProviderName, c.Identifier())
}

// Identifier returns the name for the connection
func (c EventBridgeApiConnection) Identifier() string {
	return c.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (c *EventBridgeApiConnection) Normalize(ctx context.Context) {

	if c.ConnectionParameters != nil {

		// api-key value is always replaced from a secrets manager secret, value supplied as secretName:key
		if c.ConnectionParameters.ApiKeyValue != nil {
			val, err := awsw.NewSecretsManager(ctx, c.Context.ProviderName).GetValueBySecretId(ctx, *c.ConnectionParameters.ApiKeyValue)
			if err != nil {
				panic(err)
			}
			c.ConnectionParameters.ApiKeyValue = val
		}

		// if any of the parameters are supplied as secret, replace the value from the secrets
		if c.ConnectionParameters.Parameters != nil {

			for n, hp := range c.ConnectionParameters.Parameters.HeaderParameters {
				hpp := &hp
				if hpp.Secret != nil {
					val, err := awsw.NewSecretsManager(ctx, c.Context.ProviderName).GetValueBySecretId(ctx, *hpp.Secret)
					if err != nil {
						panic(err)
					}
					hpp.Value = val
					hpp.Secret = nil // nilling it so we can use reflect.DeepEquals comparison in Compare()
					hpp.IsValueSecret = true
					c.ConnectionParameters.Parameters.HeaderParameters[n] = *hpp
				}
			}
			for n, bp := range c.ConnectionParameters.Parameters.BodyParameters {
				bpp := &bp
				if bpp.Secret != nil {
					val, err := awsw.NewSecretsManager(ctx, c.Context.ProviderName).GetValueBySecretId(ctx, *bpp.Secret)
					if err != nil {
						panic(err)
					}
					bpp.Value = val
					bpp.Secret = nil // nilling it so we can use reflect.DeepEquals comparison in Compare()
					bpp.IsValueSecret = true
					c.ConnectionParameters.Parameters.BodyParameters[n] = *bpp
				}
			}
			for n, qp := range c.ConnectionParameters.Parameters.QueryStringParameters {
				qpp := &qp
				if qpp.Secret != nil {
					val, err := awsw.NewSecretsManager(ctx, c.Context.ProviderName).GetValueBySecretId(ctx, *qpp.Secret)
					if err != nil {
						panic(err)
					}
					qpp.Value = val
					qpp.Secret = nil // nilling it so we can use reflect.DeepEquals comparison in Compare()
					qpp.IsValueSecret = true
					c.ConnectionParameters.Parameters.QueryStringParameters[n] = *qpp
				}
			}
		}
	}
}

// Validate checks that the input provided is correct
func (c EventBridgeApiConnection) Validate(ctx context.Context) error {

	var errMessages []string

	if c.ConnectionParameters == nil {
		errMessages = append(errMessages, "connection parameters (auth & parameters) are required")
	} else {

		if c.ConnectionParameters.ApiKeyName == nil || c.ConnectionParameters.ApiKeyValue == nil {
			errMessages = append(errMessages, "only api key auth is supported by buildit; and api key and value is required")
		}

		if c.ConnectionParameters.Parameters != nil {
			for _, hp := range c.ConnectionParameters.Parameters.HeaderParameters {
				if hp.Key == nil || hp.Value == nil {
					errMessages = append(errMessages, "http header parameter key & value are required")
				}
			}
			for _, bp := range c.ConnectionParameters.Parameters.BodyParameters {
				if bp.Key == nil || bp.Value == nil {
					errMessages = append(errMessages, "http body parameter key and value are required")
				}
			}
			for _, qp := range c.ConnectionParameters.Parameters.QueryStringParameters {
				if qp.Key == nil || qp.Value == nil {
					errMessages = append(errMessages, "http query string parameter key and value are required")
				}
			}
		}
	}

	if errMessages == nil {
		return nil
	}

	return &ValidationError{
		ResourceIdentifier: c.Identifier(),
		ResourceType:       "EventBridgeConnection",
		Messages:           errMessages,
	}
}

// Apply provisions or updates an eventbridge connection & the underlying api connection
func (c EventBridgeApiConnection) Apply(ctx context.Context) error {
	log.Debugf("creating eventbridge connection %v", c.Name)

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
		log.WithField("Name", c.Identifier()).Info("eventbridge connection already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}

		err = c.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update eventbridge connection %v", c.Identifier())
		}
		return nil
	}

	return c.apply(ctx)
}

// Destroy deletes an eventbridge api destination & connection
func (c EventBridgeApiConnection) Destroy(ctx context.Context) error {

	log.Debugf("destroying eventbridge connection %v", c.Identifier())

	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding eventbridge conneciton %v", c.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("eventbridge connection does not exist, nothing to destroy, skippping ")
		return nil
	}

	ebClient := client.EventBridge(ctx, c.Context.ProviderName)
	_, err = ebClient.DeleteConnection(ctx, &eventbridge.DeleteConnectionInput{
		Name: aws.String(c.Name),
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting eventbridge connection %v", c.Name)
	}

	log.WithFields(log.Fields{
		"Name": c.Name,
	}).Info(color.Red("eventbridge connection destroyed"))

	return nil
}

// EventBridgeApiConnectionDiff represents the diff between the config & aws resource
type EventBridgeApiConnectionDiff struct {
	BaseResourceDiff

	descriptionDiff bool
	paramsDiff      bool
}

// Compare fetches the resource from AWS and compares with this config, returns a diff object
// or returns an error if something goes wrong
func (c EventBridgeApiConnection) Compare(ctx context.Context) (ResourceDiff, error) {
	existing, err := c.fetchExisting(ctx)
	if err != nil {
		return nil, err
	}

	diffsFound := false
	diffs := &EventBridgeApiConnectionDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "eventbridge connection does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	if util.CoalesceComparable(c.Description, "") != util.CoalesceComparable(existing.Description, "") {
		diffsFound = true
		diffs.descriptionDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("description is different for eventbridge connection %q", c.Name))
	}

	// api key
	if util.CoalesceComparable(c.ConnectionParameters.ApiKeyName, "") != util.CoalesceComparable(existing.ConnectionParameters.ApiKeyName, "") {
		diffsFound = true
		diffs.paramsDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("api key name is different for eventbridge connection %q", c.Name))
	}

	if util.CoalesceComparable(c.ConnectionParameters.ApiKeyValue, "") != util.CoalesceComparable(existing.ConnectionParameters.ApiKeyValue, "") {
		diffsFound = true
		diffs.paramsDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("api key value is different for eventbridge connection %q", c.Name))
	}

	// header parameters
	if (c.ConnectionParameters.Parameters == nil && existing.ConnectionParameters.Parameters != nil) ||
		(c.ConnectionParameters.Parameters != nil && existing.ConnectionParameters.Parameters == nil) {
		diffsFound = true
		diffs.paramsDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("api connection parameters are different for eventbridge connection %q", c.Name))
	}

	if c.ConnectionParameters.Parameters != nil && existing.ConnectionParameters.Parameters != nil {

		if !reflect.DeepEqual(c.ConnectionParameters.Parameters.HeaderParameters, existing.ConnectionParameters.Parameters.HeaderParameters) {
			diffsFound = true
			diffs.paramsDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("api connection header parameters are different for eventbridge connection %q", c.Name))
		}

		if !reflect.DeepEqual(c.ConnectionParameters.Parameters.BodyParameters, existing.ConnectionParameters.Parameters.BodyParameters) {
			diffsFound = true
			diffs.paramsDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("api connection body parameters are different for eventbridge connection %q", c.Name))
		}

		if !reflect.DeepEqual(c.ConnectionParameters.Parameters.QueryStringParameters, existing.ConnectionParameters.Parameters.QueryStringParameters) {
			diffsFound = true
			diffs.paramsDiff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("api connection query string parameters are different for eventbridge connection %q", c.Name))
		}
	}

	if !diffsFound {
		return nil, nil
	}

	return diffs, nil
}

// getConnectHttpParameters converts the connection http parameters to *eventbridge/type/ConnectionHttpParameters
func (c EventBridgeApiConnection) getConnectHttpParameters(ctx context.Context) *eventbridgetypes.ConnectionHttpParameters {

	var headerParms []eventbridgetypes.ConnectionHeaderParameter
	var bodyParms []eventbridgetypes.ConnectionBodyParameter
	var queryStringParms []eventbridgetypes.ConnectionQueryStringParameter

	if c.ConnectionParameters.Parameters != nil {

		for _, hp := range c.ConnectionParameters.Parameters.HeaderParameters {
			headerParms = append(headerParms, eventbridgetypes.ConnectionHeaderParameter{
				Key:           hp.Key,
				Value:         hp.Value,
				IsValueSecret: hp.IsValueSecret,
			})
		}
		for _, bp := range c.ConnectionParameters.Parameters.BodyParameters {
			bodyParms = append(bodyParms, eventbridgetypes.ConnectionBodyParameter{
				Key:           bp.Key,
				Value:         bp.Value,
				IsValueSecret: bp.IsValueSecret,
			})
		}
		for _, qp := range c.ConnectionParameters.Parameters.QueryStringParameters {
			queryStringParms = append(queryStringParms, eventbridgetypes.ConnectionQueryStringParameter{
				Key:           qp.Key,
				Value:         qp.Value,
				IsValueSecret: qp.IsValueSecret,
			})
		}
		return &eventbridgetypes.ConnectionHttpParameters{
			HeaderParameters:      headerParms,
			BodyParameters:        bodyParms,
			QueryStringParameters: queryStringParms,
		}
	}

	return nil
}

// apply creates a new eventbridge connection
func (c EventBridgeApiConnection) apply(ctx context.Context) error {

	log.Debugf("creating api destination %v", c.Name)

	authParms := &eventbridgetypes.CreateConnectionAuthRequestParameters{
		ApiKeyAuthParameters: &eventbridgetypes.CreateConnectionApiKeyAuthRequestParameters{
			ApiKeyName:  c.ConnectionParameters.ApiKeyName,
			ApiKeyValue: c.ConnectionParameters.ApiKeyValue,
		},
		InvocationHttpParameters: c.getConnectHttpParameters(ctx),
	}

	ebClient := client.EventBridge(ctx, c.Context.ProviderName)
	outc, err := ebClient.CreateConnection(ctx, &eventbridge.CreateConnectionInput{
		Name:              aws.String(c.Name),
		Description:       c.Description,
		AuthorizationType: eventbridgetypes.ConnectionAuthorizationTypeApiKey,
		AuthParameters:    authParms,
	})

	if err != nil {
		return errors.Wrapf(err, "error creating api connection %v", c.Name)
	}

	log.WithFields(log.Fields{
		"Name":  c.Name,
		"State": string(outc.ConnectionState),
	}).Info(color.Green("eventbridge connection created"))

	return nil
}

// applyDiffs updates the resource with the supplied diffs
func (c EventBridgeApiConnection) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": c.Identifier(),
		}).Info("no updates required for eventbridge connection")
		return nil
	}

	connDiffs, ok := diffs.(*EventBridgeApiConnectionDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing sg
	_, ok = connDiffs.Resource.(*EventBridgeApiConnection)
	if !ok {
		return errors.Errorf("cannot retrieve existing eventbridge connection")
	}

	var err error
	client := client.EventBridge(ctx, c.Context.ProviderName)

	updateInput := &eventbridge.UpdateConnectionInput{
		Name: aws.String(c.Name),
	}

	if connDiffs.descriptionDiff {
		updateInput.Description = c.Description
	}

	// api key
	if connDiffs.paramsDiff {
		updateInput.AuthorizationType = eventbridgetypes.ConnectionAuthorizationTypeApiKey // only supported method
		updateInput.AuthParameters = &eventbridgetypes.UpdateConnectionAuthRequestParameters{
			ApiKeyAuthParameters: &eventbridgetypes.UpdateConnectionApiKeyAuthRequestParameters{
				ApiKeyName:  c.ConnectionParameters.ApiKeyName,
				ApiKeyValue: c.ConnectionParameters.ApiKeyValue,
			},
			InvocationHttpParameters: c.getConnectHttpParameters(ctx),
		}
	}
	_, err = client.UpdateConnection(ctx, updateInput)
	if err != nil {
		return err
	}
	log.WithField("Name", c.Identifier()).Info(color.Yellow("eventbridge connection updated"))

	return nil
}

// fetchExisting fetches the existing eventbridge connection or error
func (c EventBridgeApiConnection) fetchExisting(ctx context.Context) (*EventBridgeApiConnection, error) {

	ebClient := client.EventBridge(ctx, c.Context.ProviderName)

	// fetch connection info
	var apiconn *EventBridgeApiConnection
	outc, err := ebClient.ListConnections(ctx, &eventbridge.ListConnectionsInput{
		NamePrefix: aws.String(c.Name),
	})

	if err != nil {
		return nil, errors.Wrapf(err, "error fetching api connection %v", c.Name)
	}

	if len(outc.Connections) != 0 {
		for _, conn := range outc.Connections {
			if c.Name == *conn.Name {
				outcd, err := ebClient.DescribeConnection(ctx, &eventbridge.DescribeConnectionInput{
					Name: aws.String(c.Name),
				})

				if err != nil {
					return nil, errors.Wrapf(err, "error fetching api connection %v", c.Name)
				}

				connParms := &EventBridgeConnectionParameters{}
				if outcd.SecretArn != nil {
					err = awsw.NewSecretsManager(ctx, c.Context.ProviderName).GetValueAsJson(ctx, *outcd.SecretArn, connParms)
					if err != nil {
						return nil, err
					}
				}

				apiconn = &EventBridgeApiConnection{
					Arn:                  *outcd.ConnectionArn,
					Name:                 *outcd.Name,
					Description:          outcd.Description,
					ConnectionParameters: connParms,
				}
			}
		}
	}
	return apiconn, nil
}
