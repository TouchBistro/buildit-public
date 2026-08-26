package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	deduplicationScope_queue              string = "queue"
	deduplicationScope_messageGroup       string = "messageGroup"
	fifoThroughputLimit_perQueue          string = "perQueue"
	fifoThroughputLimit_perMessageGroupId string = "perMessageGroupId"
)

type SQSQueueAttributes struct {
	DelaySeconds              int    `yaml:"delaySeconds"`              // Delivery Delay
	MaxMessageBytes           int    `yaml:"maxMessageBytes"`           // Max. Message Size
	MessageRetentionSeconds   int    `yaml:"messageRetentionSeconds"`   // Message Retention Period
	ReceiveMessageWaitSeconds int    `yaml:"receiveMessageWaitSeconds"` // Receive Message Wait Time
	VisibilityTimeoutSeconds  *int   `yaml:"visibilityTimeoutSeconds"`  // Visibility Timeout, 0 is a valid value, but not the default on AWS, so make it a pointer so it can be nil
	ContentBasedDeduplication bool   `yaml:"contentBasedDeduplication"` // Content-based de-dupe, only applies to Fifo queues
	DeduplicationScope        string `yaml:"deduplicationScope"`        // only applies to Fifo queues, allowed values: queue | messageGroup
	FifoThroughputLimit       string `yaml:"fifoThroughputLimit"`       // only applies to Fifo queues, allowed values: perQueue | perMessageGroupId
}

// equals compares this SQSQueueAttributes object with the supplied other & returns a boolean
// value true, if the two are equal, else false
// TODO: @esiddiqui currently uses reflect.DeepEqual() can be improved if needed
func (q SQSQueueAttributes) equals(other SQSQueueAttributes) bool {
	return reflect.DeepEqual(q, other)
}

// SQSQueue represents an AWS SQS Queue.
type SQSQueue struct {
	BaseResource `yaml:",inline"`
	Name       string             `yaml:"-"`
	Policy     *IAMPolicyDocument `yaml:"policy"`
	Attributes SQSQueueAttributes `yaml:"attributes"`
	DependsOn  []Key              `yaml:"dependsOn"`
	Tags       map[string]string  `yaml:"tags"`
	GlobalTags map[string]string  `yaml:"-"`
	queueName  string             // Parsed from Name
	queueUrl   string             // retrieved by fetchExisting
}

// Key returns the unique key for the resource for this buildit context
func (q SQSQueue) Key() Key {
	return NewKey(q.Context.ProviderName, q.Identifier())
}

// Identifier returns the unique ID for the queue.
func (q SQSQueue) Identifier() string {
	return q.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (q *SQSQueue) Normalize(ctx context.Context) {
	_, q.queueName = ParseName(q.Name)
	// Set default values for fields where the default value is different from the zero value
	// This makes it easier to do diffing when updating
	if q.Attributes.MaxMessageBytes == 0 {
		q.Attributes.MaxMessageBytes = 262_144 // 256 KiB
	}
	if q.Attributes.MessageRetentionSeconds == 0 {
		q.Attributes.MessageRetentionSeconds = 345_600 // 4 days
	}
	if q.Attributes.VisibilityTimeoutSeconds == nil {
		q.Attributes.VisibilityTimeoutSeconds = aws.Int(30)
	}
	q.Policy.fixMapKeys()

	if q.IsFIFO() {
		if q.Attributes.DeduplicationScope == "" {
			q.Attributes.DeduplicationScope = deduplicationScope_queue
		}
		if q.Attributes.FifoThroughputLimit == "" {
			q.Attributes.FifoThroughputLimit = fifoThroughputLimit_perQueue
		}
	}

	if strings.EqualFold(q.Attributes.DeduplicationScope, deduplicationScope_queue) {
		q.Attributes.DeduplicationScope = deduplicationScope_queue
	}

	if strings.EqualFold(q.Attributes.DeduplicationScope, deduplicationScope_messageGroup) {
		q.Attributes.DeduplicationScope = deduplicationScope_messageGroup
	}

	if strings.EqualFold(q.Attributes.FifoThroughputLimit, fifoThroughputLimit_perQueue) {
		q.Attributes.FifoThroughputLimit = fifoThroughputLimit_perQueue
	}

	if strings.EqualFold(q.Attributes.FifoThroughputLimit, fifoThroughputLimit_perMessageGroupId) {
		q.Attributes.FifoThroughputLimit = fifoThroughputLimit_perMessageGroupId
	}

	// merge globalTags to queue Tags
	if q.Tags == nil {
		q.Tags = make(map[string]string)
	}

	ResourceTags(q.Tags).Merge(q.GlobalTags)
}

// Validate checks that the SQS Queue has a valid configuration.
func (q SQSQueue) Validate(ctx context.Context) error {

	var msgs []string

	// Queue name can be max 80 characters
	if len(q.queueName) > 80 {
		msg := fmt.Sprintf("name cannot be longer then 80 characters, current length: %d", len(q.queueName))
		msgs = append(msgs, msg)
	}

	attrs := q.Attributes
	if attrs.DelaySeconds < 0 || attrs.DelaySeconds > 900 {
		msg := fmt.Sprintf("attributes.delaySeconds must be between 0 and 900, current value: %d", attrs.DelaySeconds)
		msgs = append(msgs, msg)
	}
	if attrs.MaxMessageBytes < 1024 || attrs.MaxMessageBytes > 262144 {
		msg := fmt.Sprintf("attributes.maxMessageBytes must be between 1024 and 262144, current value: %d", attrs.MaxMessageBytes)
		msgs = append(msgs, msg)
	}
	if attrs.MessageRetentionSeconds < 60 || attrs.MessageRetentionSeconds > 1_209_600 {
		msg := fmt.Sprintf("attributes.messageRetentionSeconds must be between 60 and 1209600, current value: %d", attrs.MessageRetentionSeconds)
		msgs = append(msgs, msg)
	}
	if attrs.ReceiveMessageWaitSeconds < 0 || attrs.ReceiveMessageWaitSeconds > 20 {
		msg := fmt.Sprintf("attributes.receiveMessageWaitSeconds must be between 0 and 20, current value: %d", attrs.ReceiveMessageWaitSeconds)
		msgs = append(msgs, msg)
	}
	if attrs.VisibilityTimeoutSeconds == nil || *attrs.VisibilityTimeoutSeconds < 0 || *attrs.VisibilityTimeoutSeconds > 43200 {
		msg := fmt.Sprintf("attributes.visibilityTimeoutSeconds must be between 0 and 43200, current value: %d", *attrs.VisibilityTimeoutSeconds)
		msgs = append(msgs, msg)
	}
	if attrs.ContentBasedDeduplication && !q.IsFIFO() {
		msgs = append(msgs, "attributes.contentBasedDeduplication is only allowed for FIFO queues")
	}
	if attrs.DeduplicationScope != "" && !q.IsFIFO() {
		msgs = append(msgs, "attributes.deduplicatioScope is only allowed for FIFO queues")
	}
	if attrs.FifoThroughputLimit != "" && !q.IsFIFO() {
		msgs = append(msgs, "attributes.fifoThroughputLimit is only allowed for FIFO queues")
	}

	if q.IsFIFO() {

		switch attrs.DeduplicationScope {
		case deduplicationScope_queue, deduplicationScope_messageGroup:
			//no-op
		default:
			msgs = append(msgs, fmt.Sprintf("invalid value supplied for attributes.deduplicatioScope %q, only 'queue' & 'messageGroup' allowed", attrs.DeduplicationScope))
		}

		switch attrs.FifoThroughputLimit {
		case fifoThroughputLimit_perQueue, fifoThroughputLimit_perMessageGroupId:
			//no-op
		default:
			msgs = append(msgs, fmt.Sprintf("invalid value supplied for attributes.fifoThroughputLimit %q, only 'perQueue' & 'perMessageGroupId' allowed", attrs.DeduplicationScope))
		}

		if attrs.DeduplicationScope == deduplicationScope_queue && attrs.FifoThroughputLimit == fifoThroughputLimit_perMessageGroupId {
			msgs = append(msgs, fmt.Sprintf("invalid combination, attributes.deduplicatioScope= %q, attributes.fifoThroughputLimit= %q", attrs.DeduplicationScope, attrs.FifoThroughputLimit))
		}

	}

	if msgs == nil {
		return nil
	}
	return &ValidationError{
		ResourceIdentifier: q.Identifier(),
		ResourceType:       "SQSQueue",
		Messages:           msgs,
	}
}

// Apply creates/updates the SQS Queue on AWS.
func (q SQSQueue) Apply(ctx context.Context) error {
	log.Debugf("creating sqs queue %v", q.Identifier())

	diffs, err := q.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": q.Identifier(),
		}).Info("no updates required")
		return nil
	}

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", q.Identifier()).Info("sqs queue already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = q.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update sqs queue %v", q.Identifier())
		}
		return nil
	}

	return q.apply(ctx)
}

// Destroy deletes the SQS Queue from AWS.
func (q SQSQueue) Destroy(ctx context.Context) error {

	sqsClient := client.SQS(ctx, q.Context.ProviderName)
	log.WithField("Name", q.Identifier()).Info("Checking if SQS queue exists")
	queue, err := q.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "failed to check if SQS queue %s exists", q.Identifier())
	}
	if queue == nil {
		log.WithField("Name", q.Identifier()).Info("SQS queue does not exist, nothing to destroy")
		return nil
	}

	log.WithField("Name", q.Identifier()).Debug("destroying sqs queue")

	_, err = sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{
		QueueUrl: &queue.queueUrl,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to delete SQS queue %s", q.Identifier())
	}

	log.WithFields(log.Fields{
		"Name":     q.Identifier(),
		"QueueURL": queue.queueUrl,
	}).Info(color.Red("sqs queue destroyed"))

	return nil
}

type SQSQueueDiff struct {
	BaseResourceDiff

	policyDiff bool
	attrDiff   bool
	tagsDiff   bool
	tagDiff    util.TagDiffResult
}

// Compare this definition with the existing AWS object & returns diffs, or a non-nil error
func (q SQSQueue) Compare(ctx context.Context) (ResourceDiff, error) {

	existing, err := q.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", q.Identifier())
	}

	diffs := &SQSQueueDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "sqs queue does not exist")
		return diffs, nil
	}

	diffs.Resource = existing
	diff := false

	if !q.Attributes.equals(existing.Attributes) {
		diff = true
		diffs.attrDiff = true
		diffs.Messages = append(diffs.Messages, "sqs queue attributes are different")
	}

	if q.Policy.equals(existing.Policy) != Equal {
		diff = true
		diffs.policyDiff = true
		diffs.Messages = append(diffs.Messages, "sqs queue resource policy are different")
	}

	// tags
	awsTags, err := awsw.NewSQS(ctx, q.Context.ProviderName).GetQueueTags(ctx, existing.queueUrl)
	if err != nil {
		return nil, err
	}
	if tagDiff := TagDiffForContext(ctx, awsTags, q.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, TagDiffSummary(awsTags, diffs.tagDiff)...)
	}

	if !diff {
		return nil, nil
	}

	return diffs, nil
}

// apply provision a new sqs queue
func (q SQSQueue) apply(ctx context.Context) error {
	sqsClient := client.SQS(ctx, q.Context.ProviderName)
	log.WithField("Name", q.Identifier()).Debug("Creating SQS queue")

	awsAttrs, err := sqsQueueToAttrs(q)
	if err != nil {
		return errors.WithMessagef(err, "failed to create AWS queue attributes for %s", q.Identifier())
	}
	// FifoQueue isn't added by sqsQueueAttrs since it only applies to create
	// Only add the attribute if fifo, otherwise the AWS SDK will give weird errors
	if q.IsFIFO() {
		awsAttrs["FifoQueue"] = "true"
	}

	respCreateQueue, err := sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		Attributes: awsAttrs,
		QueueName:  aws.String(q.queueName),
		Tags:       q.Tags,
	})
	if err != nil {
		return errors.Wrapf(err, "failed to create SQS queue %s", q.Identifier())
	}

	log.WithFields(log.Fields{
		"Name":     q.Identifier(),
		"QueueURL": *respCreateQueue.QueueUrl,
	}).Info(color.Green("sqs queue created"))

	return nil
}

// applyDiffs updates the resource with the supplied diffs
func (q SQSQueue) applyDiffs(ctx context.Context, diffs ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": q.Identifier(),
		}).Info("no updates required for sqs queue")
		return nil
	}

	sqsDiffs, ok := diffs.(*SQSQueueDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing cert
	existing, ok := sqsDiffs.Resource.(*SQSQueue)
	if !ok {
		return errors.Errorf("invalid existing sqs queue supplied")
	}

	sqsClient := client.SQS(ctx, q.Context.ProviderName)

	// attributes & policy (policy is handled as a queue attribute)
	if sqsDiffs.attrDiff || sqsDiffs.policyDiff {
		awsAttrs, err := sqsQueueToAttrs(q)
		if err != nil {
			return errors.WithMessagef(err, "failed to create sqs queue attributes for %s", q.Identifier())
		}

		_, err = sqsClient.SetQueueAttributes(ctx, &sqs.SetQueueAttributesInput{
			Attributes: awsAttrs,
			QueueUrl:   &existing.queueUrl,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to set queue attributes %s", q.Identifier())
		}
	}

	// tags
	var err error
	if sqsDiffs.tagsDiff {
		upserts := sqsDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			err = awsw.NewSQS(ctx, q.Context.ProviderName).AddQueueTags(ctx, existing.queueUrl, upserts)
			if err != nil {
				return errors.Wrapf(err, "error updating queue tags for %v", q.Identifier())
			}
		}

		if len(sqsDiffs.tagDiff.Deleted) > 0 {
			err = awsw.NewSQS(ctx, q.Context.ProviderName).DeleteQueueTags(ctx, existing.queueUrl, sqsDiffs.tagDiff.Deleted)
			if err != nil {
				return errors.Wrapf(err, "error deleting queue tags for %v", q.Identifier())
			}
		}
	}

	log.WithFields(log.Fields{
		"Name":     q.Identifier(),
		"QueueURL": existing.queueUrl,
	}).Info(color.Yellow("sqs queue updated"))

	return nil
}

// fetchExisting fetches the url & then the attributes of the sqs queue & returns
// them in a buildit SQSQueue type
func (q SQSQueue) fetchExisting(ctx context.Context) (*SQSQueue, error) {
	sqsClient := client.SQS(ctx, q.Context.ProviderName)
	// get queue url
	outUrl, err := sqsClient.GetQueueUrl(ctx, &sqs.GetQueueUrlInput{
		QueueName: aws.String(q.queueName),
	})
	if err != nil {
		var notExistErr *sqstypes.QueueDoesNotExist
		if errors.As(err, &notExistErr) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "failed to get SQS queue url: %s", q.Identifier())
	}

	// attributes to fetch
	attrNames := []sqstypes.QueueAttributeName{
		sqstypes.QueueAttributeNameAll,
		sqstypes.QueueAttributeNameDelaySeconds,
		sqstypes.QueueAttributeNameMaximumMessageSize,
		sqstypes.QueueAttributeNameMessageRetentionPeriod,
		sqstypes.QueueAttributeNamePolicy,
		sqstypes.QueueAttributeNameReceiveMessageWaitTimeSeconds,
		sqstypes.QueueAttributeNameVisibilityTimeout,
	}

	if q.IsFIFO() {
		attrNames = append(attrNames, sqstypes.QueueAttributeNameContentBasedDeduplication)
		attrNames = append(attrNames, sqstypes.QueueAttributeNameDeduplicationScope)
		attrNames = append(attrNames, sqstypes.QueueAttributeNameFifoThroughputLimit)
	}

	outAttr, err := sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		AttributeNames: attrNames,
		QueueUrl:       outUrl.QueueUrl,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get attributes for queue %s", q.Identifier())
	}

	existing, err := sqsQueueFromAttrs(outAttr.Attributes, q.IsFIFO())
	if err != nil {
		return nil, errors.WithMessagef(err, "failed to parse attributes for queue %s", q.Identifier())
	}

	existing.queueUrl = *outUrl.QueueUrl

	return &existing, nil
}

// IsFIFO signals whether or not the SQS queue is a FIFO queue.
// A queue is FIFO if the queue name ends with '.fifo'.
func (q SQSQueue) IsFIFO() bool {
	return strings.HasSuffix(q.Name, ".fifo")
}

// sqsQueueAttrs returns a map of SQS queue attributes from the given SQSQueue.
// Note, this will not add FifoQueue, since that only applies when creating queues.
func sqsQueueToAttrs(q SQSQueue) (map[string]string, error) {
	attrs := q.Attributes
	// The default values for these attributes match the go zero values so just add them directly
	awsAttrs := map[string]string{
		string(sqstypes.QueueAttributeNameDelaySeconds):                  strconv.Itoa(attrs.DelaySeconds),
		string(sqstypes.QueueAttributeNameMaximumMessageSize):            strconv.Itoa(attrs.MaxMessageBytes),
		string(sqstypes.QueueAttributeNameMessageRetentionPeriod):        strconv.Itoa(attrs.MessageRetentionSeconds),
		string(sqstypes.QueueAttributeNameReceiveMessageWaitTimeSeconds): strconv.Itoa(attrs.ReceiveMessageWaitSeconds),
		string(sqstypes.QueueAttributeNameVisibilityTimeout):             strconv.Itoa(*attrs.VisibilityTimeoutSeconds),
	}
	// These attributes can only be set if the queue is FIFO, otherwise the AWS SDK gives weird errors
	if q.IsFIFO() {
		awsAttrs[string(sqstypes.QueueAttributeNameContentBasedDeduplication)] = strconv.FormatBool(attrs.ContentBasedDeduplication)
		awsAttrs[string(sqstypes.QueueAttributeNameDeduplicationScope)] = attrs.DeduplicationScope
		awsAttrs[string(sqstypes.QueueAttributeNameFifoThroughputLimit)] = attrs.FifoThroughputLimit
	}

	// Handle policy JSON
	if q.Policy != nil {
		policyJSON, err := json.Marshal(q.Policy)
		if err != nil {
			return nil, errors.Wrap(err, "failed to encode queue policy document as JSON")
		}
		awsAttrs[string(sqstypes.QueueAttributeNamePolicy)] = string(policyJSON)
	} else {
		// Set to empty string to use the default
		awsAttrs[string(sqstypes.QueueAttributeNamePolicy)] = ""
	}
	return awsAttrs, nil
}

// sqsQueueFromAttrs returns an SQSQueue struct from the given map of SQS queue attributes.
func sqsQueueFromAttrs(attrs map[string]string, isFifo bool) (SQSQueue, error) {
	q := SQSQueue{}

	// for k, v := range attrs {
	// 	log.Infof("%v = %v", k, v)
	// }

	// If the AWS docs are to be understood correctly these attributes should never be nil
	// since every attribute has a default value.
	delaySeconds, err := strconv.Atoi(attrs[string(sqstypes.QueueAttributeNameDelaySeconds)])
	if err != nil {
		return q, errors.Wrap(err, "failed to parse DelayedSeconds attribute")
	}
	q.Attributes.DelaySeconds = delaySeconds

	maxMessageBytes, err := strconv.Atoi(attrs[string(sqstypes.QueueAttributeNameMaximumMessageSize)])
	if err != nil {
		return q, errors.Wrap(err, "failed to parse MaximumMessageSize attribute")
	}
	q.Attributes.MaxMessageBytes = maxMessageBytes

	messageRetentionSeconds, err := strconv.Atoi(attrs[string(sqstypes.QueueAttributeNameMessageRetentionPeriod)])
	if err != nil {
		return q, errors.Wrap(err, "failed to parse MessageRetentionPeriod attribute")
	}
	q.Attributes.MessageRetentionSeconds = messageRetentionSeconds

	receiveMessageWaitSeconds, err := strconv.Atoi(attrs[string(sqstypes.QueueAttributeNameReceiveMessageWaitTimeSeconds)])
	if err != nil {
		return q, errors.Wrap(err, "failed to parse ReceiveMessageWaitTimeSeconds attribute")
	}
	q.Attributes.ReceiveMessageWaitSeconds = receiveMessageWaitSeconds

	visibilityTimeoutSeconds, err := strconv.Atoi(attrs[string(sqstypes.QueueAttributeNameVisibilityTimeout)])
	if err != nil {
		return q, errors.Wrap(err, "failed to parse VisibilityTimeout attribute")
	}
	q.Attributes.VisibilityTimeoutSeconds = &visibilityTimeoutSeconds

	if isFifo {
		contentBasedDeduplication, err := strconv.ParseBool(attrs[string(sqstypes.QueueAttributeNameContentBasedDeduplication)])
		if err != nil {
			return q, errors.Wrap(err, "failed to parse ContentBasedDeduplication attribute")
		}
		q.Attributes.ContentBasedDeduplication = contentBasedDeduplication
		q.Attributes.DeduplicationScope = attrs[string(sqstypes.QueueAttributeNameDeduplicationScope)]
		q.Attributes.FifoThroughputLimit = attrs[string(sqstypes.QueueAttributeNameFifoThroughputLimit)]
	}

	rawPolicy := attrs[string(sqstypes.QueueAttributeNamePolicy)]
	if rawPolicy != "" {
		policy, err := decodePolicyDocument(rawPolicy)
		if err != nil {
			return q, errors.Wrap(err, "failed to parse Policy attribute")
		}
		q.Policy = &policy
	}

	return q, nil
}
