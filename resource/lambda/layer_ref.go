package lambda

import (
	"context"
	"fmt"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	awsarn "github.com/aws/aws-sdk-go-v2/aws/arn"
	log "github.com/sirupsen/logrus"
)

type LayerRef struct {
	Arn        *string `yaml:"arn"`
	Name       string  `yaml:"name"`
	Version    int64   `yaml:"version"`
	VersionArn *string `yaml:"versionArn"`
}

// Identifier returns the id for this layer, arn or name
func (r LayerRef) Identifier() string {
	if r.VersionArn != nil {
		return *r.VersionArn
	} else if r.Arn != nil {
		return fmt.Sprintf("%v:%v", *r.Arn, r.Version)
	} else {
		return fmt.Sprintf("%v:%v", r.Name, r.Version)
	}
}

func (r LayerRef) key() string {
	if r.VersionArn != nil {
		return *r.VersionArn
	} else if r.Arn != nil {
		return fmt.Sprintf("%v:%v", *r.Arn, r.Version)
	} else {
		return fmt.Sprintf("%v:%v", r.Name, r.Version)
	}
}

// private

func (r *LayerRef) Normalize(ctx context.Context, rctx resource.Context) {

	if r.Arn == nil {
		var layerVer *int64
		if r.Version != 0 {
			layerVer = &r.Version
		}
		arn, varn, err := awsw.NewLambda(ctx, rctx.ProviderName).LayerArnForNameAndVersion(ctx, r.Name, layerVer)
		if arn != nil && err == nil {
			r.Arn = arn
			r.VersionArn = varn
		}
	}
}

// equals compares this arn with the other
func (r LayerRef) equals(other LayerRef) bool {

	// if both layer refs have a valid arn supplied, use that to compare.
	// also validates if the arn is a valid AWS arn.
	if r.Arn != nil && other.Arn != nil {
		return awsarn.IsARN(*r.Arn) && awsarn.IsARN(*other.Arn) &&
			util.Coalesce(r.Arn, "") == util.Coalesce(other.Arn, "") && r.Version == other.Version
	}
	// if both arns are NOT supplied, use name/version for comparison
	return r.Name == other.Name && r.Version == other.Version
}

// helpers

// layerRefSliceEqual compares the two []LayerRef slices for elements
// order of elements is not checked
func layerRefSliceEquals(this, that []LayerRef) bool {

	if len(this) != len(that) {
		return false
	}

	mmap := make(map[string]LayerRef)
	for _, l1 := range this {
		mmap[l1.key()] = l1
	}

	kmap := make(map[string]LayerRef)
	for _, l2 := range that {
		kmap[l2.key()] = l2
	}

	for _, l2 := range that {
		if l1, ok := mmap[l2.key()]; !ok {
			log.Debugf("lambda ref key %v does not exist in the other", l2.key())
			return false
		} else {
			if !l1.equals(l2) {
				log.Debugf("lambda ref key %v & %v are not the same on both sides", l1.key(), l2.key())
				return false
			}
		}
	}
	return true
}
