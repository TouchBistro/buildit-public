package lambda

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsarn "github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/pkg/errors"
)

// Layer represents a lambda layer, version
//
// TODO: the Tags field below is dead, and so no tag reaches a layer — buildit:resource-id
// included. PublishLayerVersion takes no tags, Normalize never merges GlobalTags in, and
// nothing sends them to AWS. Its yaml key is "_" rather than "tags", so a config cannot set
// it either. Making it real needs a TagResource call plus a compare/diff path; until then
// leave the key as-is rather than exposing a field that silently does nothing.
type Layer struct {
	resource.BaseResource `yaml:",inline"`
	LayerRef
	Description   *string           `yaml:"description,omitempty"`             // description of this layer
	Runtimes      []string          `yaml:"compatibleRuntimes,omitempty"`      // compatible runtimes to use this layer
	Architectures []string          `yaml:"compatibleArchitectures,omitempty"` // supported os/architectures
	License       *string           `yaml:"license,omitempty"`                 // the license agreement
	Code          Code              `yaml:"code"`                              // only supports S3 location to upload code
	Publish       bool              `yaml:"publish"`                           // only publish a new version of the layer when this is set to `true`;
	Tags          map[string]string `yaml:"_"`
	GlobalTags    map[string]string `yaml:"-"`
	DependsOn     []resource.Key    `yaml:"-"`
}

func (r Layer) String() string {
	return r.Identifier()
}

// Key returns the unique key for the resource for this buildit context
func (r Layer) Key() resource.Key {
	return resource.NewKey(r.Context.ProviderName, r.Identifier())
}

// Identifier returns the id for this layer, arn or name
func (r Layer) Identifier() string {
	return r.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields
func (r *Layer) Normalize(ctx context.Context) {

	// fetch code checksum/size
	r.Code._SHA256, r.Code._CodeSize, _ =
		awsw.NewS3(ctx, r.Context.ProviderName).GetObjectChecksumAndSize(ctx, *r.Code.S3Bucket, *r.Code.Key, r.Code.ObjVersion)
}

// Validate the lambda layer input
func (r Layer) Validate(ctx context.Context) error {

	var errorMsgs []string

	if r.Identifier() == "" {
		errorMsgs = append(errorMsgs, "lambda layer name cannot be empty")
	}

	if len(r.Runtimes) == 0 {
		errorMsgs = append(errorMsgs, "lambda layer compatible runtimes are required")
	}

	// code bucket
	if r.Code.S3Bucket == nil {
		errorMsgs = append(errorMsgs, "s3 bucket for lambda layer code must be supplied")
	}

	// code key
	if r.Code.Key == nil {
		errorMsgs = append(errorMsgs, "s3 key for lambda layer code must be supplied")
	}

	// check package code
	if r.Code._SHA256 == nil || r.Code._CodeSize == nil {
		errorMsgs = append(errorMsgs, fmt.Sprintf("code package is not readable, or buildit was not able to confirm checksum & size from bucket= %v, key=  %v, object version=%v", *r.Code.S3Bucket, *r.Code.Key, util.Coalesce(r.Code.ObjVersion, "")))
	}

	if errorMsgs == nil {
		return nil
	}

	return &resource.ValidationError{
		ResourceIdentifier: r.Identifier(),
		ResourceType:       "lambda layer",
		Messages:           errorMsgs,
	}
}

// Apply builds the lambda layer
func (r Layer) Apply(ctx context.Context) error {

	log.Debugf("creating lambda layer %v", r.Identifier())

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

	//if diff found & existing resource exists...
	if diffs.AWSResource() != nil {
		log.WithField("Name", r.Identifier()).Info("lambda layer already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = r.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update lambda layer %v", r.Identifier())
		}
		return nil
	}

	return r.apply(ctx)
}

// Destroy the lambda layer
func (r Layer) Destroy(ctx context.Context) error {

	log.Debugf("destroying lambda layer %v", r.Identifier())

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding lambda layer %v", r.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("lambda layer does not exist, nothing to destroy, skippping ")
		return nil
	}

	client := client.Lambda(ctx, r.Context.ProviderName)

	done := false
	var marker *string
	var versions []int64

	for !done {
		// list versions
		out, err := client.ListLayerVersions(ctx, &lambda.ListLayerVersionsInput{
			LayerName: aws.String(r.Name),
		})

		if err != nil {
			return errors.Wrapf(err, "error fetching lambda layer version for %v", r.Name)
		}

		marker = out.NextMarker
		done = marker == nil

		for _, v := range out.LayerVersions {
			versions = append(versions, v.Version)
		}
	}

	// delete all versions
	for _, v := range versions {
		_, err := client.DeleteLayerVersion(ctx, &lambda.DeleteLayerVersionInput{
			LayerName:     aws.String(r.Name),
			VersionNumber: aws.Int64(v),
		})

		if err != nil {
			return errors.Wrapf(err, "error deleting version %v or layer %v", v, r.Name)
		}
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Red("all versions of lambda layer destroyed"))

	return nil
}

// LayerDiff respresnts diffs between lambda layer definition & AWS representation
type LayerDiff struct {
	resource.BaseResourceDiff

	descriptionDiff   bool
	licenseDiff       bool
	architecturesDiff bool
	runtimesDiff      bool
	publishRequested  bool
}

// Compare fetches the existing lambda layer and if it exists, checks if this
// resource is equal to the corresponding AWS lambda layer
func (r Layer) Compare(ctx context.Context) (resource.ResourceDiff, error) {

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", r.Identifier())
	}

	diffs := &LayerDiff{}

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "lambda layer does not exist")
		return diffs, nil
	}

	diffs.Resource = existing
	diff := false

	// description
	if strings.TrimSpace(util.Coalesce(r.Description, "")) != strings.TrimSpace(util.Coalesce(existing.Description, "")) {
		diff = true
		diffs.descriptionDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("lambda layer description has changed %v -> %v",
			util.Coalesce(existing.Description, ""), util.Coalesce(r.Description, "")))

	}

	// architectures
	if util.DiffStringSlices(r.Architectures, existing.Architectures) {
		diff = true
		diffs.architecturesDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("lambda layer compatible architectures have changed %v -> %v",
			existing.Architectures, r.Architectures))
	}

	// runtimes
	if util.DiffStringSlices(r.Runtimes, existing.Runtimes) {
		diff = true
		diffs.runtimesDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("lambda layer compatible runtimes have changed %v -> %v",
			existing.Runtimes, r.Runtimes))

	}

	// license
	if util.Coalesce(r.License, "") != util.Coalesce(existing.License, "") {
		diff = true
		diffs.licenseDiff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("lambda layer license has changed %v -> %v",
			util.Coalesce(existing.License, ""), util.Coalesce(r.License, "")))
	}

	// publish flag
	if r.Publish {
		diff = true
		diffs.publishRequested = true
		diffs.Messages = append(diffs.Messages, "lambda layer publish flag set to true")
	}

	if diff {
		return diffs, nil
	}

	// return
	return nil, nil
}

// fetchExisting fetches lambda layer if exists
func (r Layer) fetchExisting(ctx context.Context) (*Layer, error) {

	client := client.Lambda(ctx, r.Context.ProviderName)
	out, err := client.ListLayerVersions(ctx, &lambda.ListLayerVersionsInput{
		LayerName: aws.String(r.Name),
	})

	if err != nil {
		var rnfe *types.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil, errors.Wrapf(err, "error fetching layer versions for %v", r.Identifier())
		}
	}

	if len(out.LayerVersions) > 0 {
		vMax := out.LayerVersions[0]
		var runtimes []string
		var architectres []string

		for _, a := range vMax.CompatibleArchitectures {
			architectres = append(architectres, string(a))
		}
		for _, r := range vMax.CompatibleRuntimes {
			runtimes = append(runtimes, string(r))
		}

		layer := &Layer{
			LayerRef: LayerRef{
				Name:       r.Name,
				VersionArn: vMax.LayerVersionArn,
				Version:    vMax.Version,
			},
			Description:   vMax.Description,
			Runtimes:      runtimes, //vMax.CompatibleRuntimes,
			Architectures: architectres,
			License:       vMax.LicenseInfo,
			Code: Code{
				_SHA256:   nil,
				_CodeSize: nil,
			},
		}
		return layer, nil
	}

	//client.PublishLayerVersion(ctx, &lambda.PublishLayerVersionInput{})
	return nil, nil
}

// apply provisions a new lambda layer
func (r Layer) apply(ctx context.Context) error {

	client := client.Lambda(ctx, r.Context.ProviderName)

	var architectres []types.Architecture
	for _, a := range r.Architectures {
		architectres = append(architectres, types.Architecture(a))
	}

	var runtimes []types.Runtime
	for _, r := range r.Runtimes {
		runtimes = append(runtimes, types.Runtime(r))
	}

	out, err := client.PublishLayerVersion(ctx, &lambda.PublishLayerVersionInput{
		LayerName: aws.String(r.Name),
		Content: &types.LayerVersionContentInput{
			S3Bucket:        r.Code.S3Bucket,
			S3Key:           r.Code.Key,
			S3ObjectVersion: r.Code.ObjVersion,
		},
		Description:             r.Description,
		CompatibleArchitectures: architectres,
		CompatibleRuntimes:      runtimes,
		LicenseInfo:             r.License,
	})

	if err != nil {
		return errors.Wrapf(err, "error publishing a new version for lambda layer %v", r.Name)
	}

	log.WithFields(log.Fields{
		"Name":    r.Identifier(),
		"Version": out.Version,
	}).Info(color.Green("lambda layer published"))

	return nil
}

// applyDiffs applies diffs to an existing lambda layer
func (r Layer) applyDiffs(ctx context.Context, diffs resource.ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("no updates required for lambda layer")
		return nil
	}

	resDiffs, ok := diffs.(*LayerDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// fetch existing sg
	_, ok = resDiffs.Resource.(*Layer)
	if !ok {
		return errors.Errorf("cannot retrieve existing lambda layer")
	}

	return r.apply(ctx)
}

// private
/**
// equals compares this arn with the other
func (r Layer) equals(other Layer) bool {
	return r.LayerRef.equals(other.LayerRef)
}

// helper

// layerSliceEqual compares the two []Layer slices for elements
// order of elements is not checked
func layerSliceEquals(this, that []Layer) bool {

	if len(this) != len(that) {
		return false
	}

	mmap := make(map[string]Layer)
	for _, l1 := range this {
		mmap[l1.Identifier()] = l1
	}

	for _, l2 := range that {
		if l1, ok := mmap[l2.Identifier()]; !ok {
			return false
		} else {
			if !l1.equals(l2) {
				return false
			}
		}
	}
	return true
}
**/

// findLayerByArn return a Layer object by arn, else a non-nil error
func findLayerByArn(ctx context.Context, rctx resource.Context, arn *string) (*Layer, error) {

	if arn == nil {
		return nil, errors.New("nil arn supplied")
	}

	client := client.Lambda(ctx, rctx.ProviderName)
	out, err := client.GetLayerVersionByArn(ctx, &lambda.GetLayerVersionByArnInput{
		Arn: arn,
	})

	if err != nil {
		return nil, errors.Wrapf(err, "error fetching lambda layer for arn %v", *arn)
	}
	var archs []string
	for _, a := range out.CompatibleArchitectures {
		archs = append(archs, string(a))
	}

	var rts []string
	for _, rt := range out.CompatibleRuntimes {
		rts = append(rts, string(rt))
	}

	a, _ := awsarn.Parse(*out.LayerArn)
	splits := strings.Split(a.Resource, ":") // resource part of arn is `layer:<name>'`
	res := splits[len(splits)-1]             // pick the last index
	l := &Layer{
		LayerRef: LayerRef{
			Name:       res,
			Arn:        out.LayerArn,
			VersionArn: out.LayerVersionArn,
			Version:    out.Version,
		},
		Description:   out.Description,
		Architectures: archs,
		Runtimes:      rts,
		License:       out.LicenseInfo,
	}

	return l, nil
}

// findLayerByNameAndVersion returns a Layer object by name & vesion number, else a non-nil error
func findLayerByNameAndVersion(ctx context.Context, rctx resource.Context, name string, version int64) (*Layer, error) {

	client := client.Lambda(ctx, rctx.ProviderName)
	out, err := client.GetLayerVersion(ctx, &lambda.GetLayerVersionInput{
		LayerName:     aws.String(name),
		VersionNumber: aws.Int64(version),
	})

	if err != nil {
		return nil, errors.Wrapf(err, "error fetching lambda layer for naem %q & version numbrer %q", name, version)
	}
	var archs []string
	for _, a := range out.CompatibleArchitectures {
		archs = append(archs, string(a))
	}

	var rts []string
	for _, rt := range out.CompatibleRuntimes {
		rts = append(rts, string(rt))
	}

	a, _ := awsarn.Parse(*out.LayerArn)
	l := &Layer{
		LayerRef: LayerRef{
			Name:       a.Resource,
			Version:    out.Version,
			Arn:        out.LayerArn,
			VersionArn: out.LayerVersionArn,
		},
		Description:   out.Description,
		Architectures: archs,
		Runtimes:      rts,
		License:       out.LicenseInfo,
	}

	return l, nil
}
