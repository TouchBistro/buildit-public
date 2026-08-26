package awsw

import (
	"context"

	"github.com/TouchBistro/buildit/client"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kafkaconnect"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type MSK struct {
	*kafkaconnect.Client
}

// NewMSK creates a new instance of MSK wrapper
func NewMSK(ctx context.Context, providerName string) MSK {
	return MSK{client.MSK(ctx, providerName)}
}

// AddResourceTags tags the MSK resourse with the supplied tag keys/value, returns error if thhe
// operation fails
func (l MSK) AddResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		_, err := l.TagResource(ctx, &kafkaconnect.TagResourceInput{
			ResourceArn: aws.String(arn),
			Tags:        tags,
		})

		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"Arn": arn,
		}).Infof("%v tags added to MSK resouce", len(tags))
	}

	return nil
}

// DeleteResourceTags removes the supplied tag keys from the MSK resource, or returns an error
// if the oepration fails
func (l MSK) DeleteResourceTags(ctx context.Context, arn string, tags map[string]string) error {

	if len(tags) > 0 {

		var keys []string
		for k := range tags {
			keys = append(keys, k)
		}
		_, err := l.UntagResource(ctx, &kafkaconnect.UntagResourceInput{
			ResourceArn: aws.String(arn),
			TagKeys:     keys,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to update tags for resource %s", arn)
		}
		log.WithFields(log.Fields{
			"Arn": arn,
		}).Infof("%v tags deleted from resource", len(tags))
	}
	return nil
}
