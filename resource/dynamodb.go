package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	log "github.com/sirupsen/logrus"
)

// DynamoDB represents a dynamodb table
type DynamoDB struct {
	BaseResource `yaml:",inline"`
	Name                      string                          `yaml:"-"`
	AttributeDefinitions      []AttributeDefinition           `yaml:"attributeDefinitions"`
	KeySchema                 []KeySchema                     `yaml:"keySchema"`
	ProvisionedThroughput     *ProvisionedThroughput          `yaml:"provisionedThroughput"` // for PROVISIONED billing mode
	DeletionProtectionEnabled *bool                           `yaml:"deletionProtection"`
	GlobalTableRegions        []string                        `yaml:"globalTableRegions"` // replication regions for global table, also enable streaming
	GlobalSecondaryIndexes    map[string]GlobalSecondaryIndex `yaml:"globalSecondaryIndexes"`
	ServerSideEncryption      *bool                           `yaml:"serverSideEncryption"` // enable or disable serverside encryption
	ResourcePolicy            *IAMPolicyDocument              `yaml:"resourcePolicy"`
	UpdateTimeout             int                             `yaml:"updateTimeout"`
	TTLAttribute              *string                         `yaml:"ttlAttribute"` // name of the TTL attribute for items in the table
	DependsOn                 []Key                           `yaml:"dependsOn"`
	Tags                      map[string]string               `yaml:"tags"`
	GlobalTags                map[string]string               `yaml:"-"`
	_arn                      string                          `yaml:"-"` // read from AWS only
	_billingMode              types.BillingMode               `yaml:"-"` // infer from provisioned throughput
}

type AttributeDefinition struct {
	Name          string `yaml:"name"`
	AttributeType string `yaml:"type"`
}

type KeySchema struct {
	Name    string `yaml:"name"`
	KeyType string `yaml:"type"`
}

type Projection struct {
	ProjectionType   string   `yaml:"type"`
	NonKeyAttributes []string `yaml:"nonKeyAttributes"`
}

type ProvisionedThroughput struct {
	ReadCapacityUnits  *int64 `yaml:"readCapacityUnits"`
	WriteCapacityUnits *int64 `yaml:"writeCapacityUnits"`
}

type GlobalSecondaryIndex struct {
	KeySchema  []KeySchema `yaml:"keySchema"`
	Projection Projection  `yaml:"projection"`
}

// globalSecondaryIndexEquals checks if two GlobalSecondaryIndex values are equal
func globalSecondaryIndexEquals(a GlobalSecondaryIndex, b GlobalSecondaryIndex) bool {
	if !util.SliceElementsEqual(a.KeySchema, b.KeySchema) {
		return false
	}
	if a.Projection.ProjectionType != b.Projection.ProjectionType {
		return false
	}
	if !util.SliceElementsEqual(a.Projection.NonKeyAttributes, b.Projection.NonKeyAttributes) {
		return false
	}
	return true
}

type GlobalSecondaryIndexMap map[string]GlobalSecondaryIndex

// Key returns the unique key for the resource for this buildit context
func (d DynamoDB) Key() Key {
	return NewKey(d.Context.ProviderName, d.Identifier())
}

// Identifier returns the name of the dynamodb table
func (d DynamoDB) Identifier() string {
	return d.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (d *DynamoDB) Normalize(ctx context.Context) {
	if d.Tags == nil {
		d.Tags = make(map[string]string)
	}
	ResourceTags(d.Tags).Merge(d.GlobalTags)

	// make sure keys are string
	if d.ResourcePolicy != nil {
		d.ResourcePolicy.fixMapKeys()
	}

	// deletion protection
	if d.DeletionProtectionEnabled == nil {
		d.DeletionProtectionEnabled = aws.Bool(true)
	}

	// server side encryption
	if d.ServerSideEncryption == nil {
		d.ServerSideEncryption = aws.Bool(true)
	}

	// timeout
	if d.UpdateTimeout < 1 {
		d.UpdateTimeout = 900 // default to 15min
	}

	// billing mode
	d._billingMode = types.BillingModePayPerRequest
	if d.ProvisionedThroughput != nil {
		d._billingMode = types.BillingModeProvisioned
	}
}

// Validate checks that the input provided is correct
func (d DynamoDB) Validate(ctx context.Context) error {

	var errorMsgs []string

	// attribute definitions
	if len(d.AttributeDefinitions) == 0 {
		errorMsgs = append(errorMsgs, "attributeDefinitions must be provided")
	}
	for _, attr := range d.AttributeDefinitions {
		if attr.Name == "" {
			errorMsgs = append(errorMsgs, "attribute name must be provided")
		}
		switch types.ScalarAttributeType(attr.AttributeType) {
		case types.ScalarAttributeTypeS, types.ScalarAttributeTypeN, types.ScalarAttributeTypeB:
		default:
			//invalid attribute type
			errorMsgs = append(errorMsgs, fmt.Sprintf("attributeDefinitions %v: invalid attribute type", attr.Name))
		}
	}

	// key schema
	if len(d.KeySchema) == 0 {
		errorMsgs = append(errorMsgs, "keySchema must be provided")
	}
	for _, schema := range d.KeySchema {
		if schema.Name == "" {
			errorMsgs = append(errorMsgs, "key name must be provided")
		}
		switch types.KeyType(schema.KeyType) {
		case types.KeyTypeHash, types.KeyTypeRange:
		default:
			//invalid key type
			errorMsgs = append(errorMsgs, fmt.Sprintf("keySchema %v: invalid key type", schema.Name))
		}
	}

	// capacity
	// type is PROVISIONED
	if d.ProvisionedThroughput != nil {
		if d.ProvisionedThroughput.WriteCapacityUnits == nil || d.ProvisionedThroughput.ReadCapacityUnits == nil {
			errorMsgs = append(errorMsgs, "provisionedThroughput must specify both readCapacityUnits and writeCapacityUnits")
		}
		if len(d.GlobalSecondaryIndexes) > 0 {
			errorMsgs = append(errorMsgs, "global secondary index currently not supported when billingMode is PROVISIONED")
		}
	}

	// global secondary indexes
	for k, v := range d.GlobalSecondaryIndexes {
		switch types.ProjectionType(v.Projection.ProjectionType) {
		case types.ProjectionTypeAll, types.ProjectionTypeKeysOnly, types.ProjectionTypeInclude:
		default:
			//invalid projection type
			errorMsgs = append(errorMsgs, fmt.Sprintf("globalSecondaryIndexes %v: invalid projection type", k))
		}
		if len(v.Projection.NonKeyAttributes) == 0 && v.Projection.ProjectionType == "INCLUDE" {
			errorMsgs = append(errorMsgs, fmt.Sprintf("globalSecondaryIndexes %v: at least one non-key attribute must be specified for projection type INCLUDE", k))
		}
		if len(v.Projection.NonKeyAttributes) > 0 && v.Projection.ProjectionType != "INCLUDE" {
			errorMsgs = append(errorMsgs, fmt.Sprintf("globalSecondaryIndexes %v: projectionType is %v, but NonKeyAttributes is specified", k, v.Projection.ProjectionType))
		}
	}

	if errorMsgs == nil {
		return nil
	}

	return &ValidationError{
		ResourceType: "dynamodb table",
		Messages:     errorMsgs,
	}
}

// Apply creates a new dynamodb table
func (d DynamoDB) Apply(ctx context.Context) error {
	log.Debugf("creating dynamodb table %v", d.Identifier())

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
		log.WithField("Name", d.Identifier()).Info("dynamodb table already exists, updating")
		err = d.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update dynamodb table %v", d.Identifier())
		}
		return nil
	}

	return d.apply(ctx)
}

// apply provisions a new dynamodb table
func (d DynamoDB) apply(ctx context.Context) error {
	// attribute definitions
	var attrDefs []types.AttributeDefinition
	for _, attr := range d.AttributeDefinitions {
		attrDefs = append(attrDefs, types.AttributeDefinition{
			AttributeName: aws.String(attr.Name),
			AttributeType: types.ScalarAttributeType(attr.AttributeType),
		})
	}

	// key schema
	var keySchema []types.KeySchemaElement
	for _, ks := range d.KeySchema {
		keySchema = append(keySchema, types.KeySchemaElement{
			AttributeName: aws.String(ks.Name),
			KeyType:       types.KeyType(ks.KeyType),
		})
	}

	// global secondary indexes
	var gsis []types.GlobalSecondaryIndex
	for k, v := range d.GlobalSecondaryIndexes {
		var gsiKeySchema []types.KeySchemaElement
		for _, ks := range v.KeySchema {
			gsiKeySchema = append(gsiKeySchema, types.KeySchemaElement{
				AttributeName: aws.String(ks.Name),
				KeyType:       types.KeyType(ks.KeyType),
			})
		}
		gsis = append(gsis, types.GlobalSecondaryIndex{
			IndexName: aws.String(k),
			KeySchema: gsiKeySchema,
			Projection: &types.Projection{
				ProjectionType:   types.ProjectionType(v.Projection.ProjectionType),
				NonKeyAttributes: v.Projection.NonKeyAttributes,
			},
		})
	}

	// provisioned throughput and billing mode
	var provisionedThroughput *types.ProvisionedThroughput
	if d.ProvisionedThroughput != nil {
		provisionedThroughput = &types.ProvisionedThroughput{
			ReadCapacityUnits:  d.ProvisionedThroughput.ReadCapacityUnits,
			WriteCapacityUnits: d.ProvisionedThroughput.WriteCapacityUnits,
		}
	}

	// resource policy
	var policy *string
	if d.ResourcePolicy != nil {
		policyJSON, err := json.Marshal(d.ResourcePolicy)
		if err != nil {
			return errors.Wrapf(err, "failed to encode policy document as JSON: %s", d.Identifier())
		}
		policy = aws.String(string(policyJSON))
	}

	//tags
	var tags []types.Tag
	for k, v := range d.Tags {
		tags = append(tags, types.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}

	dynamoClient := client.DynamoDB(ctx, d.Context.ProviderName)
	_, err := dynamoClient.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:                 aws.String(d.Name),
		AttributeDefinitions:      attrDefs,
		KeySchema:                 keySchema,
		BillingMode:               d._billingMode,
		DeletionProtectionEnabled: d.DeletionProtectionEnabled,
		GlobalSecondaryIndexes:    gsis,
		ProvisionedThroughput:     provisionedThroughput,
		SSESpecification: &types.SSESpecification{
			Enabled: d.ServerSideEncryption,
		},
		ResourcePolicy: policy,
		Tags:           tags,
	})
	if err != nil {
		return errors.Wrapf(err, "error creating dynamodb table %v", d.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": d.Identifier(),
	}).Info(color.Green("dynamodb table created"))

	// wait for table to become active
	if err := d.waitForUpdate(ctx); err != nil {
		return err
	}

	// create replicas
	var toAdd []types.ReplicationGroupUpdate
	for _, r := range d.GlobalTableRegions {
		toAdd = append(toAdd, types.ReplicationGroupUpdate{
			Create: &types.CreateReplicationGroupMemberAction{
				RegionName: aws.String(r),
			},
		})
	}
	if len(toAdd) > 0 {
		_, err = dynamoClient.UpdateTable(ctx, &dynamodb.UpdateTableInput{
			TableName:      aws.String(d.Name),
			ReplicaUpdates: toAdd,
		})
		if err != nil {
			return errors.Wrapf(err, "error creating dynamodb table %v replicas", d.Identifier())
		}
		if err := d.waitForUpdate(ctx); err != nil {
			return err
		}
	}

	// ttl specification
	if d.TTLAttribute != nil {
		_, err = dynamoClient.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
			TableName: aws.String(d.Name),
			TimeToLiveSpecification: &types.TimeToLiveSpecification{
				AttributeName: aws.String(*d.TTLAttribute),
				Enabled:       aws.Bool(true),
			},
		})
		if err != nil {
			return errors.Wrapf(err, "error updating dynamodb table %v ttl specification", d.Identifier())
		}
	}

	return nil
}

// applyDiffs applies supported diffs to an existing dynamodb table
func (d DynamoDB) applyDiffs(ctx context.Context, diffs ResourceDiff) error {
	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": d.Identifier(),
		}).Info("no updates required for dynamodb table")
		return nil
	}

	dynamoDiff, ok := diffs.(*DynamoDBDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	existing, ok := dynamoDiff.Resource.(*DynamoDB)
	if !ok {
		return errors.Errorf("cannot retrieve existing dynamodb table")
	}

	// check invalid diff
	if dynamoDiff.keyDiff {
		return errors.Errorf("key schema cannot be modified for a dynamodb table")
	}

	if len(dynamoDiff.gsisToUpdate) > 0 {
		return errors.Errorf("update to existing global secondary indexes not supported")
	}

	// attribute definitions
	var attrDefs []types.AttributeDefinition
	for _, attr := range d.AttributeDefinitions {
		attrDefs = append(attrDefs, types.AttributeDefinition{
			AttributeName: aws.String(attr.Name),
			AttributeType: types.ScalarAttributeType(attr.AttributeType),
		})
	}

	dynamoClient := awsw.NewDynamoDB(ctx, d.Context.ProviderName)

	// only 1 of these 3 operations are allowed at a time:
	// Modify the provisioned throughput settings of the table.
	// Remove a global secondary index from the table.
	// Create a new global secondary index on the table. After the index begins backfilling, you can use UpdateTable to perform other operations.
	// we need to apply them one at a time

	// throughput update
	if dynamoDiff.throughputDiff {
		var provisionedThroughput *types.ProvisionedThroughput
		if d.ProvisionedThroughput != nil {
			provisionedThroughput = &types.ProvisionedThroughput{
				ReadCapacityUnits:  d.ProvisionedThroughput.ReadCapacityUnits,
				WriteCapacityUnits: d.ProvisionedThroughput.WriteCapacityUnits,
			}
		}
		_, err := dynamoClient.UpdateTable(ctx, &dynamodb.UpdateTableInput{
			TableName:             aws.String(d.Name),
			BillingMode:           d._billingMode,
			ProvisionedThroughput: provisionedThroughput,
		})
		if err != nil {
			return errors.Wrapf(err, "error updating dynamodb table %v billing mode/throughput", d.Identifier())
		}
		if err := d.waitForUpdate(ctx); err != nil {
			return err
		}
	}

	// Server-Side Encryption modification must be the only operation in the request
	if dynamoDiff.sseDiff {
		_, err := dynamoClient.UpdateTable(ctx, &dynamodb.UpdateTableInput{
			TableName: aws.String(d.Name),
			SSESpecification: &types.SSESpecification{
				Enabled: d.ServerSideEncryption,
			},
		})
		if err != nil {
			return errors.Wrapf(err, "error updating dynamodb table %v sse specification", d.Identifier())
		}
		if err := d.waitForUpdate(ctx); err != nil {
			return err
		}
	}

	// deletion protection modification must be the only operation in the request
	if dynamoDiff.deletionProtectionDiff {
		_, err := dynamoClient.UpdateTable(ctx, &dynamodb.UpdateTableInput{
			TableName:                 aws.String(d.Name),
			DeletionProtectionEnabled: d.DeletionProtectionEnabled,
		})
		if err != nil {
			return errors.Wrapf(err, "error updating dynamodb table %v deletion protection", d.Identifier())
		}
		if err := d.waitForUpdate(ctx); err != nil {
			return err
		}
	}

	// global secondary indexes
	// can create or delete only one global secondary index per UpdateTable operation
	// delete gsis
	for k := range dynamoDiff.gsisToDelete {
		_, err := dynamoClient.UpdateTable(ctx, &dynamodb.UpdateTableInput{
			TableName:            aws.String(d.Name),
			AttributeDefinitions: attrDefs,
			GlobalSecondaryIndexUpdates: []types.GlobalSecondaryIndexUpdate{{
				Delete: &types.DeleteGlobalSecondaryIndexAction{
					IndexName: aws.String(k),
				},
			},
			},
		})
		if err != nil {
			return errors.Wrapf(err, "error deleting secondary index %v from dynamodb table %v", k, d.Identifier())
		}
		if err = d.waitForIndex(ctx); err != nil {
			return err
		}
	}

	// create gsis
	for k, v := range dynamoDiff.gsisToAdd {
		var keySchema []types.KeySchemaElement
		for _, ks := range v.KeySchema {
			keySchema = append(keySchema, types.KeySchemaElement{
				AttributeName: aws.String(ks.Name),
				KeyType:       types.KeyType(ks.KeyType),
			})
		}
		_, err := dynamoClient.UpdateTable(ctx, &dynamodb.UpdateTableInput{
			TableName:            aws.String(d.Name),
			AttributeDefinitions: attrDefs,
			GlobalSecondaryIndexUpdates: []types.GlobalSecondaryIndexUpdate{{
				Create: &types.CreateGlobalSecondaryIndexAction{
					IndexName: aws.String(k),
					KeySchema: keySchema,
					Projection: &types.Projection{
						ProjectionType:   types.ProjectionType(v.Projection.ProjectionType),
						NonKeyAttributes: v.Projection.NonKeyAttributes,
					},
				},
			},
			},
		})
		if err != nil {
			return errors.Wrapf(err, "error creating secondary index %v on dynamodb table %v", k, d.Identifier())
		}
		if err = d.waitForIndex(ctx); err != nil {
			return err
		}
	}

	// resource policy
	if dynamoDiff.policyDiff {
		if d.ResourcePolicy != nil {
			policyJSON, err := json.Marshal(d.ResourcePolicy)
			if err != nil {
				return errors.Wrapf(err, "failed to encode policy document as JSON: %s", d.Identifier())
			}
			_, err = dynamoClient.PutResourcePolicy(ctx, &dynamodb.PutResourcePolicyInput{
				ResourceArn: aws.String(existing._arn),
				Policy:      aws.String(string(policyJSON)),
			})
			if err != nil {
				return errors.Wrapf(err, "error updating resource policy for dynamodb table %v", d.Identifier())
			}
		} else {
			_, err := dynamoClient.DeleteResourcePolicy(ctx, &dynamodb.DeleteResourcePolicyInput{
				ResourceArn: aws.String(existing._arn),
			})
			if err != nil {
				return errors.Wrapf(err, "error deleting resource policy for dynamodb table %v", d.Identifier())
			}
		}
	}

	// global table regions
	var replications []types.ReplicationGroupUpdate
	for _, r := range dynamoDiff.globalRegionsToAdd {
		replications = append(replications, types.ReplicationGroupUpdate{
			Create: &types.CreateReplicationGroupMemberAction{
				RegionName: aws.String(r),
			},
		})
	}
	for _, r := range dynamoDiff.globalRegionsToDelete {
		replications = append(replications, types.ReplicationGroupUpdate{
			Delete: &types.DeleteReplicationGroupMemberAction{
				RegionName: aws.String(r),
			},
		})
	}

	if len(replications) > 0 {
		_, err := dynamoClient.UpdateTable(ctx, &dynamodb.UpdateTableInput{
			TableName:      aws.String(d.Name),
			ReplicaUpdates: replications,
		})
		if err != nil {
			return errors.Wrapf(err, "error updating dynamodb table %v replicas", d.Identifier())
		}
		if err := d.waitForUpdate(ctx); err != nil {
			return err
		}
	}

	// ttl specification
	if dynamoDiff.ttlDiff {
		var ttlSpec types.TimeToLiveSpecification
		if d.TTLAttribute != nil {
			// could disable and wait but it could take more than 1 hour to propagate so just error out for now
			if existing.TTLAttribute != nil {
				return errors.New("TimeToLive is active on a different AttributeName, to change the TTL attribute, first disable TTL on the table and then re-enable it with the new attribute name")
			}
			ttlSpec = types.TimeToLiveSpecification{
				AttributeName: d.TTLAttribute,
				Enabled:       aws.Bool(true),
			}
		} else {
			ttlSpec = types.TimeToLiveSpecification{
				Enabled: aws.Bool(false),
			}
		}
		_, err := dynamoClient.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
			TableName:               aws.String(d.Name),
			TimeToLiveSpecification: &ttlSpec,
		})
		if err != nil {
			return errors.Wrapf(err, "error updating ttl specification for dynamodb table %v", d.Identifier())
		}
		// not waiting for this one cause it may take up to 1 hour to enable
	}

	// tags
	if dynamoDiff.tagsDiff {
		upserts := dynamoDiff.tagDiff.Upserts()
		if len(upserts) > 0 {
			err := dynamoClient.AddResourceTags(ctx, existing._arn, upserts)
			if err != nil {
				return errors.Wrapf(err, "error adding dynamodb table tags for %v", d.Identifier())
			}
		}

		if len(dynamoDiff.tagDiff.Deleted) > 0 {
			err := dynamoClient.DeleteResourceTags(ctx, existing._arn, dynamoDiff.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error deleting dynamodb table tags for %v", d.Identifier())
			}
		}
	}

	log.WithFields(log.Fields{
		"Name": d.Identifier(),
	}).Info(color.Yellow("dynamodb table updated"))

	return nil
}

// dynamodb table diff
type DynamoDBDiff struct {
	BaseResourceDiff
	attrDiff               bool
	keyDiff                bool
	throughputDiff         bool
	deletionProtectionDiff bool
	sseDiff                bool
	policyDiff             bool
	globalRegionsDiff      bool
	globalRegionsToAdd     []string
	globalRegionsToDelete  []string
	gsisDiff               bool
	gsisToAdd              GlobalSecondaryIndexMap
	gsisToDelete           GlobalSecondaryIndexMap
	gsisToUpdate           GlobalSecondaryIndexMap
	ttlDiff                bool
	tagsDiff               bool
	tagDiff                util.TagDiffResult
}

// Compare fetches the existing dynamodb table & if it exists returns nil, else returns the diffs
func (d DynamoDB) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := d.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", d.Identifier())
	}

	diffs := &DynamoDBDiff{}

	diff := false

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "dynamodb table does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	// attribute definitions
	if !util.SliceElementsEqual(d.AttributeDefinitions, existing.AttributeDefinitions) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("attribute definitions will be updated %v -> %v", existing.AttributeDefinitions, d.AttributeDefinitions))
		diffs.attrDiff = true
	}

	// key schema
	if !util.SliceElementsEqual(d.KeySchema, existing.KeySchema) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("key schema will be updated %v -> %v", existing.KeySchema, d.KeySchema))
		diffs.keyDiff = true
	}

	// deletion protection
	if util.CoalesceComparable(d.DeletionProtectionEnabled, true) != util.CoalesceComparable(existing.DeletionProtectionEnabled, true) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("deletion protection will be updated %v -> %v", util.CoalesceComparable(existing.DeletionProtectionEnabled, true), util.CoalesceComparable(d.DeletionProtectionEnabled, true)))
		diffs.deletionProtectionDiff = true
	}

	// server side encryption
	if util.CoalesceComparable(d.ServerSideEncryption, true) != util.CoalesceComparable(existing.ServerSideEncryption, true) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("sse will be updated %v -> %v", util.CoalesceComparable(existing.ServerSideEncryption, true), util.CoalesceComparable(d.ServerSideEncryption, true)))
		diffs.sseDiff = true
	}

	// policy diff
	if d.ResourcePolicy.equals(existing.ResourcePolicy) != Equal {
		diff = true
		diffs.policyDiff = true
		diffs.Messages = append(diffs.Messages, "resource policy will be updated")
	}

	// global table regions
	if !util.SliceElementsEqual(d.GlobalTableRegions, existing.GlobalTableRegions) {
		diff = true
		diffs.globalRegionsDiff = true
		diffs.globalRegionsToAdd, diffs.globalRegionsToDelete = util.Convert(existing.GlobalTableRegions, d.GlobalTableRegions)
		if len(diffs.globalRegionsToAdd) > 0 {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v global table regions to be added - %v", len(diffs.globalRegionsToAdd), diffs.globalRegionsToAdd))
		}
		if len(diffs.globalRegionsToDelete) > 0 {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v global table regions to be removed - %v", len(diffs.globalRegionsToDelete), diffs.globalRegionsToDelete))
		}
	}

	// global secondary indexes
	if !util.MapEquals(d.GlobalSecondaryIndexes, existing.GlobalSecondaryIndexes, globalSecondaryIndexEquals) {
		diff = true
		diffs.gsisDiff = true
		diffs.gsisToAdd, diffs.gsisToDelete, diffs.gsisToUpdate = util.MapConvert(existing.GlobalSecondaryIndexes, d.GlobalSecondaryIndexes, globalSecondaryIndexEquals)
		if len(diffs.gsisToAdd) > 0 {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v global secondary indexes to be added - %v", len(diffs.gsisToAdd), diffs.gsisToAdd))
		}
		if len(diffs.gsisToDelete) > 0 {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v global secondary indexes to be deleted - %v", len(diffs.gsisToDelete), diffs.gsisToDelete))
		}
		if len(diffs.gsisToUpdate) > 0 {
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("global secondary indexes update not supported - %v diffs found", len(diffs.gsisToUpdate)))
		}
	}

	// comparing throughput
	if d._billingMode != existing._billingMode || (d._billingMode == "PROVISIONED" && !reflect.DeepEqual(*d.ProvisionedThroughput, *existing.ProvisionedThroughput)) { // should not be nil
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("provisioned throughput will be updated %v -> %v", *existing.ProvisionedThroughput, *d.ProvisionedThroughput))
		diffs.throughputDiff = true
	}

	// comparing ttl specification
	if util.Coalesce(d.TTLAttribute, "") != util.Coalesce(existing.TTLAttribute, "") {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("ttl attribute will be updated %v -> %v", existing.TTLAttribute, d.TTLAttribute))
		diffs.ttlDiff = true
	}

	// tags
	if tagDiff := TagDiffForContext(ctx, existing.Tags, d.Tags); tagDiff.HasChanges() {
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

// Destroy removes the dynamodb table
func (d DynamoDB) Destroy(ctx context.Context) error {
	log.Debugf("destroying dynamodb table: %v", d.Identifier())

	existing, err := d.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding dynamodb table: %v", d.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": d.Identifier(),
		}).Info("dynamodb table does not exist, nothing to destroy, skipping")
		return nil
	}

	dynamoClient := client.DynamoDB(ctx, d.Context.ProviderName)

	// delete all replicas first
	var toDelete []types.ReplicationGroupUpdate
	for _, r := range existing.GlobalTableRegions {
		toDelete = append(toDelete, types.ReplicationGroupUpdate{
			Delete: &types.DeleteReplicationGroupMemberAction{
				RegionName: aws.String(r),
			},
		})
	}
	if len(toDelete) > 0 {
		_, err = dynamoClient.UpdateTable(ctx, &dynamodb.UpdateTableInput{
			TableName:      aws.String(d.Name),
			ReplicaUpdates: toDelete,
		})
		if err != nil {
			return errors.Wrapf(err, "error deleting dynamodb table %v replicas", d.Identifier())
		}
		if err := d.waitForReplicaDelete(ctx); err != nil {
			return err
		}
	}

	_, err = dynamoClient.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(existing.Name),
	})

	if err != nil {
		return errors.Wrapf(err, "error deleting dynamodb table: %v", d.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": d.Identifier(),
	}).Info(color.Red("dynamodb table destroyed"))

	return nil
}

// fetchExisting returns the existing dynamodb table details if found
func (d DynamoDB) fetchExisting(ctx context.Context) (*DynamoDB, error) {
	dynamoClient := client.DynamoDB(ctx, d.Context.ProviderName)
	out, err := dynamoClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(d.Name),
	})

	var rnfe *types.ResourceNotFoundException
	if err != nil {
		// if table not found, then return nil obj
		if errors.As(err, &rnfe) {
			return nil, nil
		}
		// for other err's return err
		return nil, err
	}

	tableDesc := out.Table

	// attribute definitions
	var attrDef []AttributeDefinition
	for _, attr := range tableDesc.AttributeDefinitions {
		attrDef = append(attrDef, AttributeDefinition{
			Name:          util.Coalesce(attr.AttributeName, ""),
			AttributeType: string(attr.AttributeType),
		})
	}

	// key schema
	var keySchema []KeySchema
	for _, ks := range tableDesc.KeySchema {
		keySchema = append(keySchema, KeySchema{
			Name:    util.Coalesce(ks.AttributeName, ""),
			KeyType: string(ks.KeyType),
		})
	}

	// provisioned throughput
	var throughput *ProvisionedThroughput
	if tableDesc.ProvisionedThroughput != nil {
		throughput = &ProvisionedThroughput{
			ReadCapacityUnits:  tableDesc.ProvisionedThroughput.ReadCapacityUnits,
			WriteCapacityUnits: tableDesc.ProvisionedThroughput.WriteCapacityUnits,
		}
	}

	// global secondary indexes
	gsis := make(map[string]GlobalSecondaryIndex)
	for _, gsi := range tableDesc.GlobalSecondaryIndexes {
		var gsiKeySchema []KeySchema
		for _, ks := range gsi.KeySchema {
			gsiKeySchema = append(gsiKeySchema, KeySchema{
				Name:    util.Coalesce(ks.AttributeName, ""),
				KeyType: string(ks.KeyType),
			})
		}
		projectionType := "ALL"
		if gsi.Projection != nil {
			projectionType = string(gsi.Projection.ProjectionType)
		}
		gsis[util.Coalesce(gsi.IndexName, "")] = GlobalSecondaryIndex{
			KeySchema: gsiKeySchema,
			Projection: Projection{
				ProjectionType:   projectionType,
				NonKeyAttributes: gsi.Projection.NonKeyAttributes,
			},
		}
	}

	// resource policy
	policyOut, err := dynamoClient.GetResourcePolicy(ctx, &dynamodb.GetResourcePolicyInput{
		ResourceArn: tableDesc.TableArn,
	})
	var pnfe *types.PolicyNotFoundException
	var policy *IAMPolicyDocument
	if err != nil && !errors.As(err, &pnfe) {
		return nil, errors.Wrapf(err, "error fetching resource policy for dynamodb table: %v", d.Identifier())
	}
	if err == nil {
		policyDoc, err := decodePolicyDocument(util.Coalesce(policyOut.Policy, ""))
		if err != nil {
			return nil, err
		}
		policy = &policyDoc
	}

	// tags
	tags := make(map[string]string)
	next := aws.String("")
	for {
		out, err := dynamoClient.ListTagsOfResource(ctx, &dynamodb.ListTagsOfResourceInput{
			ResourceArn: tableDesc.TableArn,
			NextToken:   next,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "error fetching tags for dynamodb table: %v", d.Identifier())
		}
		for _, t := range out.Tags {
			tags[*t.Key] = util.Coalesce(t.Value, "")
		}
		if out.NextToken == nil {
			break
		}
		next = out.NextToken
	}

	// global table
	var regions []string
	for _, r := range tableDesc.Replicas {
		regions = append(regions, *r.RegionName)
	}

	// sse status
	sse := false
	if tableDesc.SSEDescription != nil {
		sse = tableDesc.SSEDescription.Status == types.SSEStatusEnabled
	}

	// ttl attribute
	ttlOut, err := dynamoClient.DescribeTimeToLive(ctx, &dynamodb.DescribeTimeToLiveInput{
		TableName: tableDesc.TableName,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching ttl specification for dynamodb table: %v", d.Identifier())
	}
	var ttlAttribute *string
	if ttlOut.TimeToLiveDescription != nil && (ttlOut.TimeToLiveDescription.TimeToLiveStatus == types.TimeToLiveStatusEnabled || ttlOut.TimeToLiveDescription.TimeToLiveStatus == types.TimeToLiveStatusEnabling) {
		ttlAttribute = ttlOut.TimeToLiveDescription.AttributeName
	}
	existing := &DynamoDB{
		Name:                      util.Coalesce(tableDesc.TableName, ""),
		AttributeDefinitions:      attrDef,
		KeySchema:                 keySchema,
		ProvisionedThroughput:     throughput,
		DeletionProtectionEnabled: tableDesc.DeletionProtectionEnabled,
		GlobalSecondaryIndexes:    gsis,
		ServerSideEncryption:      aws.Bool(sse),
		ResourcePolicy:            policy,
		GlobalTableRegions:        regions,
		TTLAttribute:              ttlAttribute,
		Tags:                      tags,
		_arn:                      *tableDesc.TableArn,
		_billingMode:              tableDesc.BillingModeSummary.BillingMode,
	}

	return existing, nil
}

// waitForUpdate waits for the update of the table to be complete.
// It does this by continuously polling AWS until it sees that the table state is ACTIVE
func (d DynamoDB) waitForUpdate(ctx context.Context) error {

	// DISCUSS(@maintainer): Should this be configurable?
	checkInterval := 15 * time.Second
	ctx, cancel := context.WithTimeout(ctx, time.Duration(d.UpdateTimeout)*time.Second)
	defer cancel()

	dynamodbClient := client.DynamoDB(ctx, d.Context.ProviderName)

	for {
		// Wait for either the context to be done or the sleep duration.
		// This adds a wait period in between checks so we can give AWS a chance to finish the deployment before checking again.
		if err := util.SleepWithContext(ctx, checkInterval); err != nil {
			return err
		}

		desc, err := dynamodbClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(d.Name),
		})
		if err != nil {
			return errors.Wrapf(err, "error finding dynamodb table: %v", d.Identifier())
		}
		// Table has to be ACTIVE and not in the middle of an SSE update
		if desc.Table.TableStatus == types.TableStatusActive && (desc.Table.SSEDescription == nil || desc.Table.SSEDescription.Status != types.SSEStatusUpdating) {
			log.Infof(color.Yellow("DynamoDB table %v updated"), d.Name)
			break
		}
		log.Infof("DynamoDB table %v update is not yet complete", color.Cyan(d.Name))
	}

	return nil
}

// waitForIndex waits for all the global secondary indexes to be ACTIVE.
// It does this by continuously polling AWS until it sees that the global indexes state is ACTIVE
func (d DynamoDB) waitForIndex(ctx context.Context) error {

	// DISCUSS(@maintainer): Should this be configurable?
	checkInterval := 15 * time.Second
	ctx, cancel := context.WithTimeout(ctx, time.Duration(d.UpdateTimeout)*time.Second)
	defer cancel()

	dynamodbClient := client.DynamoDB(ctx, d.Context.ProviderName)

	// GSI take up to 2 minutes to show up in the DescribeTable call after the UpdateTable call returns
	if err := util.SleepWithContext(ctx, 120*time.Second); err != nil {
		return err
	}

	for {
		out, err := dynamodbClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(d.Name),
		})
		if err != nil {
			return errors.Wrapf(err, "error finding dynamodb table: %v", d.Identifier())
		}

		allActive := true
		for _, gsi := range out.Table.GlobalSecondaryIndexes {
			if gsi.IndexStatus != types.IndexStatusActive {
				allActive = false
				log.Infof("DynamoDB table %v global secondary index %v is in %v state", color.Cyan(d.Name), color.Cyan(*gsi.IndexName), color.Cyan(string(gsi.IndexStatus)))
				break
			}
		}

		if allActive {
			break
		}
		log.Infof("DynamoDB table %v global secondary index update is not yet complete", color.Cyan(d.Name))

		// Wait for either the context to be done or the sleep duration.
		// This adds a wait period in between checks so we can give AWS a chance to finish the deployment before checking again.
		if err := util.SleepWithContext(ctx, checkInterval); err != nil {
			return err
		}
	}

	return nil
}

// waitForReplicaDelete waits for the all replicas to be deleted.
// It does this by continuously polling AWS until it sees that the table state is ACTIVE
func (d DynamoDB) waitForReplicaDelete(ctx context.Context) error {

	// DISCUSS(@maintainer): Should this be configurable?
	checkInterval := 15 * time.Second
	ctx, cancel := context.WithTimeout(ctx, time.Duration(d.UpdateTimeout)*time.Second)
	defer cancel()

	dynamodbClient := client.DynamoDB(ctx, d.Context.ProviderName)
	for {
		// Wait for either the context to be done or the sleep duration.
		// This adds a wait period in between checks so we can give AWS a chance to finish the deployment before checking again.
		if err := util.SleepWithContext(ctx, checkInterval); err != nil {
			return err
		}

		desc, err := dynamodbClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(d.Name),
		})
		if err != nil {
			return errors.Wrapf(err, "error finding dynamodb table: %v", d.Identifier())
		}
		// Check if table is ACTIVE and all replicas have been deleted
		if desc.Table.TableStatus == types.TableStatusActive && len(desc.Table.Replicas) == 0 {
			log.Infof(color.Red("DynamoDB table %v replicas deleted"), d.Name)
			break
		}

		log.Infof("DynamoDB table %v replica deletion is not yet complete", color.Cyan(d.Name))
	}

	return nil
}
