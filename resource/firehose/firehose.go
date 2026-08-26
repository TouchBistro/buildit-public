package firehose

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/firehose"
	firehosetypes "github.com/aws/aws-sdk-go-v2/service/firehose/types"
	"github.com/pkg/errors"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"

	log "github.com/sirupsen/logrus"
)

// Key returns the unique key for the resource for this buildit context.
func (r FirehoseDeliveryStream) Key() resource.Key {
	return resource.NewKey(r.Context.ProviderName, r.Identifier())
}

// Identifier returns the unique id for the resource.
func (r FirehoseDeliveryStream) Identifier() string {
	return r.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields.
func (r *FirehoseDeliveryStream) Normalize(ctx context.Context) {
	if r.StreamType == "" || strings.EqualFold(r.StreamType, string(firehosetypes.DeliveryStreamTypeDirectPut)) {
		r.StreamType = string(firehosetypes.DeliveryStreamTypeDirectPut)
	}

	if r.Destination != nil {
		r.Destination.normalize(ctx, r.Context.ProviderName)
	}
	if r.IcebergDestination != nil {
		r.IcebergDestination.normalize(ctx, r.Context.ProviderName)
	}

	if r.Tags == nil {
		r.Tags = make(map[string]string)
	}
	resource.ResourceTags(r.Tags).Merge(r.GlobalTags)
}

// Validate the FirehoseDeliveryStream input.
func (r FirehoseDeliveryStream) Validate(ctx context.Context) error {
	var errorMsgs []string

	if r.Identifier() == "" {
		errorMsgs = append(errorMsgs, "firehose delivery stream name cannot be empty")
	}

	if !strings.EqualFold(r.StreamType, string(firehosetypes.DeliveryStreamTypeDirectPut)) {
		errorMsgs = append(errorMsgs, fmt.Sprintf("only %q delivery streams are supported", firehosetypes.DeliveryStreamTypeDirectPut))
	}

	if r.Destination == nil && r.IcebergDestination == nil {
		errorMsgs = append(errorMsgs, "either extendedS3Destination or icebergDestination must be defined")
	} else if r.Destination != nil && r.IcebergDestination != nil {
		errorMsgs = append(errorMsgs, "only one of extendedS3Destination or icebergDestination can be defined")
	}

	if r.Destination != nil {
		errorMsgs = append(errorMsgs, r.Destination.validate()...)
	}
	if r.IcebergDestination != nil {
		errorMsgs = append(errorMsgs, r.IcebergDestination.validate()...)
	}

	if len(errorMsgs) == 0 {
		return nil
	}

	return &resource.ValidationError{
		ResourceIdentifier: r.Identifier(),
		ResourceType:       "FirehoseDeliveryStream",
		Messages:           errorMsgs,
	}
}

// Apply builds or updates the firehose delivery stream.
func (r FirehoseDeliveryStream) Apply(ctx context.Context) error {
	diffs, err := r.Compare(ctx)
	if err != nil {
		return err
	}

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("no updates required")
		return nil
	}

	if diffs.AWSResource() != nil {
		log.Debugf("updating firehose delivery stream %v", r.Identifier())
		return r.applyDiffs(ctx, diffs)
	}

	log.Debugf("creating firehose delivery stream %v", r.Identifier())
	return r.apply(ctx)
}

// Destroy removes the firehose delivery stream.
func (r FirehoseDeliveryStream) Destroy(ctx context.Context) error {
	log.Debugf("destroying firehose delivery stream %v", r.Identifier())

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding firehose delivery stream %v", r.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("firehose delivery stream does not exist, nothing to destroy, skipping")
		return nil
	}

	fhClient := client.Firehose(ctx, r.Context.ProviderName)
	_, err = fhClient.DeleteDeliveryStream(ctx, &firehose.DeleteDeliveryStreamInput{
		AllowForceDelete:   aws.Bool(true),
		DeliveryStreamName: aws.String(r.Identifier()),
	})
	if err != nil {
		return errors.Wrapf(err, "error deleting firehose delivery stream %v", r.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Red("firehose delivery stream destroyed"))
	return nil
}

// Compare fetches the existing firehose delivery stream and if it exists, checks if this
// resource is equal to the corresponding AWS firehose delivery stream.
func (r FirehoseDeliveryStream) Compare(ctx context.Context) (resource.ResourceDiff, error) {
	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", r.Identifier())
	}

	diffs := &FirehoseDeliveryStreamDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "firehose delivery stream does not exist")
		return diffs, nil
	}

	diffs.Resource = existing
	if !strings.EqualFold(existing.StreamType, r.StreamType) {
		return nil, errors.Errorf("changing firehose delivery stream type requires recreation (current %q, desired %q)", existing.StreamType, r.StreamType)
	}

	if r.Destination != nil || existing.Destination != nil {
		destDiffs := diffExtendedS3Destination(existing.Destination, r.Destination)
		if len(destDiffs) > 0 {
			diffs.destinationDiff = true
			diffs.Messages = append(diffs.Messages, destDiffs...)
		}
	}

	if r.IcebergDestination != nil || existing.IcebergDestination != nil {
		destDiffs := diffIcebergDestination(existing.IcebergDestination, r.IcebergDestination)
		if len(destDiffs) > 0 {
			diffs.destinationDiff = true
			diffs.Messages = append(diffs.Messages, destDiffs...)
		}
	}

	if tagDiff := resource.TagDiffForContext(ctx, existing.Tags, r.Tags); tagDiff.HasChanges() {
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, resource.TagDiffSummary(existing.Tags, diffs.tagDiff)...)
	}

	if len(diffs.Messages) == 0 {
		return nil, nil
	}

	return diffs, nil
}

// fetchExisting fetches firehose delivery stream if exists.
func (r FirehoseDeliveryStream) fetchExisting(ctx context.Context) (*FirehoseDeliveryStream, error) {
	fhClient := client.Firehose(ctx, r.Context.ProviderName)
	out, err := fhClient.DescribeDeliveryStream(ctx, &firehose.DescribeDeliveryStreamInput{
		DeliveryStreamName: aws.String(r.Identifier()),
	})
	if err != nil {
		var notFound *firehosetypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error describing firehose delivery stream %v", r.Identifier())
	}

	if len(out.DeliveryStreamDescription.Destinations) == 0 {
		return nil, errors.Errorf("delivery stream %v has no destinations defined", r.Identifier())
	}

	dest := out.DeliveryStreamDescription.Destinations[0]
	if dest.ExtendedS3DestinationDescription == nil && dest.IcebergDestinationDescription == nil {
		return nil, errors.Errorf("delivery stream %v uses unsupported destination type", r.Identifier())
	}

	existing := FirehoseDeliveryStream{
		BaseResource:            r.BaseResource,
		Name:                    r.Identifier(),
		StreamType:              string(out.DeliveryStreamDescription.DeliveryStreamType),
		deliveryStreamVersionID: out.DeliveryStreamDescription.VersionId,
		destinationID:           dest.DestinationId,
	}

	if dest.ExtendedS3DestinationDescription != nil {
		existing.Destination = firehoseDestinationFromDescription(dest.ExtendedS3DestinationDescription)
		// Perform reverse lookups
		iam := awsw.NewIAM(ctx, r.Context.ProviderName)
		roleName, err := iam.RoleNameForArn(ctx, aws.ToString(dest.ExtendedS3DestinationDescription.RoleARN))
		if err != nil {
			log.Warnf("failed to resolve role name for %s: %v", aws.ToString(dest.ExtendedS3DestinationDescription.RoleARN), err)
		} else {
			existing.Destination.Role = aws.ToString(roleName)
		}
		existing.Destination.Bucket = bucketNameFromArn(aws.ToString(dest.ExtendedS3DestinationDescription.BucketARN))
	}

	if dest.IcebergDestinationDescription != nil {
		existing.IcebergDestination = icebergDestinationFromDescription(dest.IcebergDestinationDescription)
		// Perform reverse lookups
		iam := awsw.NewIAM(ctx, r.Context.ProviderName)
		roleName, err := iam.RoleNameForArn(ctx, aws.ToString(dest.IcebergDestinationDescription.RoleARN))
		if err != nil {
			log.Warnf("failed to resolve role name for %s: %v", aws.ToString(dest.IcebergDestinationDescription.RoleARN), err)
		} else {
			existing.IcebergDestination.Role = aws.ToString(roleName)
		}

		if existing.IcebergDestination.S3Configuration.BucketArn != nil {
			existing.IcebergDestination.S3Configuration.Bucket = bucketNameFromArn(aws.ToString(existing.IcebergDestination.S3Configuration.BucketArn))
		}
		if existing.IcebergDestination.S3Configuration.RoleArn != nil {
			roleName, err := iam.RoleNameForArn(ctx, aws.ToString(existing.IcebergDestination.S3Configuration.RoleArn))
			if err != nil {
				log.Warnf("failed to resolve role name for %s: %v", aws.ToString(existing.IcebergDestination.S3Configuration.RoleArn), err)
			} else {
				existing.IcebergDestination.S3Configuration.Role = aws.ToString(roleName)
			}
		}
	}

	awsTags, err := r.fetchTags(ctx)
	if err != nil {
		return nil, err
	}
	existing.Tags = awsTags

	return &existing, nil
}

func bucketNameFromArn(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 6 {
		return arn
	}
	return parts[len(parts)-1]
}

// apply provisions a new firehose delivery stream.
func (r FirehoseDeliveryStream) apply(ctx context.Context) error {
	fhClient := client.Firehose(ctx, r.Context.ProviderName)

	// Refactoring apply to handle mutually exclusive destinations
	var err error
	if r.IcebergDestination != nil {
		_, err = fhClient.CreateDeliveryStream(ctx, &firehose.CreateDeliveryStreamInput{
			DeliveryStreamName:              aws.String(r.Identifier()),
			DeliveryStreamType:              firehosetypes.DeliveryStreamType(r.StreamType),
			IcebergDestinationConfiguration: r.IcebergDestination.toAWSConfiguration(),
			Tags:                            toFirehoseTags(r.Tags),
		})
	} else {
		_, err = fhClient.CreateDeliveryStream(ctx, &firehose.CreateDeliveryStreamInput{
			DeliveryStreamName:                 aws.String(r.Identifier()),
			DeliveryStreamType:                 firehosetypes.DeliveryStreamType(r.StreamType),
			ExtendedS3DestinationConfiguration: r.Destination.toAWSConfiguration(),
			Tags:                               toFirehoseTags(r.Tags),
		})
	}
	if err != nil {
		return errors.Wrapf(err, "failed to create firehose delivery stream %v", r.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Green("firehose delivery stream created"))

	return nil
}

// applyDiffs applies diffs to an existing firehose delivery stream.
func (r FirehoseDeliveryStream) applyDiffs(ctx context.Context, diffs resource.ResourceDiff) error {
	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("no updates required for firehose delivery stream")
		return nil
	}

	fhDiffs, ok := diffs.(*FirehoseDeliveryStreamDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	existing, ok := fhDiffs.Resource.(*FirehoseDeliveryStream)
	if !ok {
		return errors.Errorf("cannot retrieve existing firehose delivery stream")
	}

	fhClient := client.Firehose(ctx, r.Context.ProviderName)

	if fhDiffs.destinationDiff {
		if existing.deliveryStreamVersionID == nil || existing.destinationID == nil {
			return errors.New("missing delivery stream version or destination id for update")
		}

		input := &firehose.UpdateDestinationInput{
			CurrentDeliveryStreamVersionId: existing.deliveryStreamVersionID,
			DeliveryStreamName:             aws.String(r.Identifier()),
			DestinationId:                  existing.destinationID,
		}

		if r.Destination != nil {
			input.ExtendedS3DestinationUpdate = r.Destination.toAWSUpdate()
		} else if r.IcebergDestination != nil {
			input.IcebergDestinationUpdate = r.IcebergDestination.toAWSUpdate()
		} else {
			return errors.New("neither Destination nor IcebergDestination set for update")
		}

		_, err := fhClient.UpdateDestination(ctx, input)
		if err != nil {
			return errors.Wrapf(err, "failed to update firehose delivery stream %v", r.Identifier())
		}
	}

	if fhDiffs.tagsDiff {
		upserts := fhDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			_, err := fhClient.TagDeliveryStream(ctx, &firehose.TagDeliveryStreamInput{
				DeliveryStreamName: aws.String(r.Identifier()),
				Tags:               toFirehoseTags(upserts),
			})
			if err != nil {
				return errors.Wrapf(err, "error updating firehose delivery stream tags for %v", r.Identifier())
			}
		}

		if len(fhDiffs.tagDiff.Deleted) > 0 {
			_, err := fhClient.UntagDeliveryStream(ctx, &firehose.UntagDeliveryStreamInput{
				DeliveryStreamName: aws.String(r.Identifier()),
				TagKeys:            fhDiffs.tagDiff.DeletedKeys(),
			})
			if err != nil {
				return errors.Wrapf(err, "error deleting firehose delivery stream tags for %v", r.Identifier())
			}
		}
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Yellow("firehose delivery stream updated"))

	return nil
}

func toFirehoseTags(tags map[string]string) []firehosetypes.Tag {
	var awsTags []firehosetypes.Tag
	keys := util.StringMap(tags).Keys()
	for _, k := range keys {
		awsTags = append(awsTags, firehosetypes.Tag{
			Key:   aws.String(k),
			Value: aws.String(tags[k]),
		})
	}
	return awsTags
}

func (r FirehoseDeliveryStream) fetchTags(ctx context.Context) (map[string]string, error) {
	fhClient := client.Firehose(ctx, r.Context.ProviderName)
	tags := make(map[string]string)

	var startKey *string
	for {
		out, err := fhClient.ListTagsForDeliveryStream(ctx, &firehose.ListTagsForDeliveryStreamInput{
			DeliveryStreamName:   aws.String(r.Identifier()),
			ExclusiveStartTagKey: startKey,
		})
		if err != nil {
			return nil, errors.Wrapf(err, "error listing tags for firehose delivery stream %v", r.Identifier())
		}

		for _, t := range out.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}

		if !aws.ToBool(out.HasMoreTags) || len(out.Tags) == 0 {
			break
		}

		startKey = out.Tags[len(out.Tags)-1].Key
	}

	return tags, nil
}
