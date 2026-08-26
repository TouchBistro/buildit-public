package lambda

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/TouchBistro/buildit/awsw"
	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

const (
	HighestPublishedVersionKey string = "$highest"
)

type LogConfig struct {
	LogGroup            *string `yaml:"logGroup"`
	ApplicationLogLevel string  `yaml:"logLevel,omitempty"`       // trace debug info (default) warn error fatal
	SystemLogLevel      string  `yaml:"systemLogLevel,omitempty"` // debug info (default) warn
	Format              *string `yaml:"format"`                   // text json
}

// equals compares this logConfig with the supplied other & resturns a EqualsResult
func (c *LogConfig) equals(other *LogConfig) resource.EqualsResult {
	if c == nil && other != nil {
		return resource.LeftZero
	} else if other == nil && c != nil {
		return resource.RightZero
	} else {
		if c.ApplicationLogLevel != other.ApplicationLogLevel || c.SystemLogLevel != other.SystemLogLevel ||
			util.Coalesce(c.Format, string(types.LogFormatText)) != util.Coalesce(other.Format, string(types.LogFormatText)) ||
			util.Coalesce(c.LogGroup, "") != util.Coalesce(other.LogGroup, "") {
			return resource.NotEqual
		}
	}
	return resource.Equal
}

type VpcConfig struct {
	Name           *string  `yaml:"name"`
	SecurityGroups []string `yaml:"securityGroups"`
	Subnets        []string `yaml:"subnets"`
}

// LambdaImageOverrides define the image override attribtes
// for the lambda image
type LambdaImageOverrides struct {
	Command          []string `yaml:"command"`
	Entrypoint       []string `yaml:"entrypoint"`
	WorkingDirectory *string  `yaml:"workingDir"`
}

// Mount an EFS file system
type FileSystem struct {
	Name           string `yaml:"name"` // name of the access point
	LocalMountPath string `yaml:"localMountPath"`
	Arn            *string
}

// Function represents a Function resource
type Function struct {
	resource.BaseResource `yaml:",inline"`
	Name               string                `yaml:"-"`
	Description        string                `yaml:"description"`
	Code               Code                  `yaml:"code"`
	Role               string                `yaml:"role"`
	Policy             *Policy               `yaml:"policy,omitempty"`
	Architectures      []string              `yaml:"architectures"` // used by zip/image both
	Environment        map[string]string     `yaml:"environment"`
	Secrets            map[string]string     `yaml:"secrets"`
	EphemeralStorage   *int32                `yaml:"ephemeralStorage,omitempty"` // default:512 MB
	Memory             *int32                `yaml:"memory,omitempty"`           // default:256 MB
	Handler            *string               `yaml:"handler,omitempty"`          // zip only
	Layers             []LayerRef            `yaml:"layers"`                     // zip only
	PackageType        *string               `yaml:"type"`                       // zip | image (default)
	ImageOverrides     *LambdaImageOverrides `yaml:"imageOverride,omitempty"`    // image only
	Publish            *bool                 `yaml:"publish,omitempty"`          // default: false,
	ForcePublish       *bool                 `yaml:"forcePublish,omitempty"`
	Runtime            string                `yaml:"runtime"`        // zip only
	Timeout            *int32                `yaml:"timeoutSeconds"` // default 3s
	VpcConfig          *VpcConfig            `yaml:"vpc,omitempty"`
	Aliases            []LambdaAlias         `yaml:"aliases"`
	FunctionUrl        *FunctionUrlConfig    `yaml:"functionUrl,omitempty"`
	LogConfig          *LogConfig            `yaml:"logging,omitempty"`
	FileSystem         *FileSystem           `yaml:"fileSystem"`
	TerminateRecursion *bool                 `yaml:"terminateRecursion"` // enable recursion protection
	DependsOn          []resource.Key        `yaml:"dependsOn"`
	Tags               map[string]string     `yaml:"tags"`
	GlobalTags         map[string]string     `yaml:"-"`
	_existingVersions  []string              `yaml:"-"` // read from AWS only
	_arn               *string               `yaml:"-"` // read from AWS only
}

// Key returns the unique key for the resource for this buildit context
func (r Function) Key() resource.Key {
	return resource.NewKey(r.Context.ProviderName, r.Identifier())
}

// Identifier returns the unique id for the resource
func (r Function) Identifier() string {
	return r.Name
}

// Normalize will set any default values or sanitize/clean up any necessary fields
func (r *Function) Normalize(ctx context.Context) {

	// Merge globalTags to security group tags, if key is not already present
	// later we'll use sg.Tags to add/update tags

	if r.Tags == nil {
		r.Tags = make(map[string]string)
	}
	resource.ResourceTags(r.Tags).Merge(r.GlobalTags)

	// set publish & forcePublish to false as default
	if r.Publish == nil {
		r.Publish = aws.Bool(false)
	}

	if r.ForcePublish == nil {
		r.ForcePublish = aws.Bool(false)
	}

	// set package type = image default
	if r.PackageType == nil {
		r.PackageType = aws.String(string(types.PackageTypeImage))
	}

	// package type
	if strings.EqualFold(*r.PackageType, string(types.PackageTypeImage)) {
		r.PackageType = aws.String(string(types.PackageTypeImage))
	} else if strings.EqualFold(*r.PackageType, string(types.PackageTypeZip)) {
		r.PackageType = aws.String(string(types.PackageTypeZip))
	}

	// check package code
	if *r.PackageType == string(types.PackageTypeZip) {
		r.Code._SHA256, r.Code._CodeSize, _ =
			awsw.NewS3(ctx, r.Context.ProviderName).GetObjectChecksumAndSize(ctx, *r.Code.S3Bucket, *r.Code.Key, r.Code.ObjVersion)
	}

	// ephemeral storage
	if r.EphemeralStorage == nil {
		r.EphemeralStorage = aws.Int32(512) // 512 MB
	}

	// memory size
	if r.Memory == nil {
		r.Memory = aws.Int32(256) // 256 MB
	}

	// timeout
	if r.Timeout == nil {
		r.Timeout = aws.Int32(3) // 3s
	}

	// architectures
	if len(r.Architectures) == 0 {
		r.Architectures = append(r.Architectures, "x86_64")
	}

	// function url
	if r.FunctionUrl != nil {
		r.FunctionUrl.normalize()
	}

	// fetch secrets & add to environment
	if len(r.Secrets) > 0 {

		for k, v := range r.Secrets {
			val, err := awsw.NewSecretsManager(ctx, r.Context.ProviderName).GetValueBySecretId(ctx, v)
			if err != nil {
				log.Warnf("error fetching secret value key %v from %v", k, v)
			}

			if r.Environment == nil {
				r.Environment = make(map[string]string)
			}

			r.Environment[k] = *val
		}
	}

	// alias
	for _, al := range r.Aliases {
		if al.FunctionUrl != nil {
			name := al.Name
			al.FunctionUrl.alias = &name
			al.FunctionUrl.normalize()
		}
	}

	// logging
	if r.LogConfig != nil {

		// if log format not provided, use Text; else fix the case/value to: Text/JSON
		if r.LogConfig.Format == nil {
			r.LogConfig.Format = aws.String(string(types.LogFormatText)) // Text
		} else {
			if strings.EqualFold(*r.LogConfig.Format, string(types.LogFormatText)) {
				r.LogConfig.Format = aws.String(string(types.LogFormatText))
			} else if strings.EqualFold(*r.LogConfig.Format, string(types.LogFormatJson)) {
				r.LogConfig.Format = aws.String(string(types.LogFormatJson))
			}
		}

		// set log-level defaults to Info if not supplied when format = JSON
		if *r.LogConfig.Format == string(types.LogFormatJson) {
			if r.LogConfig.ApplicationLogLevel == "" {
				r.LogConfig.ApplicationLogLevel = string(types.ApplicationLogLevelInfo)

			}
			if r.LogConfig.SystemLogLevel == "" {
				r.LogConfig.SystemLogLevel = string(types.SystemLogLevelInfo)
			}
		}
	}

	// enable recursion protection by default
	if r.TerminateRecursion == nil {
		r.TerminateRecursion = aws.Bool(true)
	}

	// layer ref
	for n, ly := range r.Layers {
		ly.Normalize(ctx, r.Context)
		r.Layers[n] = ly
	}
}

// Validate the lambda function input
func (r Function) Validate(ctx context.Context) error {

	var errorMsgs []string

	if r.Identifier() == "" {
		errorMsgs = append(errorMsgs, "Lambda function name is required")
	}

	// package type
	switch *r.PackageType {
	case string(types.PackageTypeImage), string(types.PackageTypeZip):
		if *r.PackageType == string(types.PackageTypeImage) && r.Code.Image == nil {
			errorMsgs = append(errorMsgs, "image url not supplied for packageType == Image")
		} else if *r.PackageType == string(types.PackageTypeZip) && (r.Code.S3Bucket == nil || r.Code.Key == nil) {
			errorMsgs = append(errorMsgs, "s3 bucket & key not supplied for packageType == Zip")
		}
	default:
		errorMsgs = append(errorMsgs, fmt.Sprintf("invalid package type supplied %q for lambda function", *r.PackageType))
	}

	if *r.PackageType == string(types.PackageTypeZip) {
		// check package code
		if r.Code._SHA256 == nil || r.Code._CodeSize == nil {
			errorMsgs = append(errorMsgs, fmt.Sprintf("code package is not readable, or buildit was not able to confirm checksum & size from bucket= %v, key=  %v, object version=%v", *r.Code.S3Bucket, *r.Code.Key, util.Coalesce(r.Code.ObjVersion, "")))
		}

		if r.Runtime == "" {
			errorMsgs = append(errorMsgs, "runtime required when package type is Zip")
		}
		if r.Handler == nil {
			errorMsgs = append(errorMsgs, "handler required when package type is Zip ")
		}
	}

	if r.FileSystem != nil && r.FileSystem.LocalMountPath == "" {
		errorMsgs = append(errorMsgs, "localMountPath required when specifying file system config")
	}

	// function url
	if r.FunctionUrl != nil {
		msgs := r.FunctionUrl.validate()
		if msgs != nil {
			errorMsgs = append(errorMsgs, msgs...)
		}
	}

	for _, al := range r.Aliases {
		if al.FunctionUrl != nil {
			msgs := al.FunctionUrl.validate()
			if msgs != nil {
				errorMsgs = append(errorMsgs, msgs...)
			}
		}
	}

	// logging
	if r.LogConfig != nil {

		switch *r.LogConfig.Format {
		case string(types.LogFormatText), string(types.LogFormatJson):
			// no-op
		default:
			errorMsgs = append(errorMsgs, fmt.Sprintf("invalid log format supplied %v", *r.LogConfig.Format))
		}

		if r.LogConfig.LogGroup == nil {
			errorMsgs = append(errorMsgs, "log group name must be supplied")
		}

		// validate log levels when JSON
		if *r.LogConfig.Format == string(types.LogFormatJson) {
			switch r.LogConfig.ApplicationLogLevel {
			case string(types.ApplicationLogLevelTrace),
				string(types.ApplicationLogLevelDebug), string(types.ApplicationLogLevelInfo),
				string(types.ApplicationLogLevelWarn), string(types.ApplicationLogLevelError),
				string(types.ApplicationLogLevelFatal):
			// no-op
			default:
				errorMsgs = append(errorMsgs, fmt.Sprintf("invalid application log level supplied %v", r.LogConfig.ApplicationLogLevel))
			}

			switch r.LogConfig.SystemLogLevel {
			case string(types.SystemLogLevelDebug),
				string(types.SystemLogLevelInfo), string(types.SystemLogLevelWarn):
				// no-op
			default:
				errorMsgs = append(errorMsgs, fmt.Sprintf("invalid system log level supplied %v", r.LogConfig.SystemLogLevel))
			}
		}

	}

	if errorMsgs == nil {
		return nil
	}

	return &resource.ValidationError{
		ResourceIdentifier: r.Identifier(),
		ResourceType:       "lambda function",
		Messages:           errorMsgs,
	}
}

// Apply builds the lambda function
func (r Function) Apply(ctx context.Context) error {

	log.Debugf("creating lambda function %v", r.Identifier())

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
		log.WithField("Name", r.Identifier()).Info("lambda function already exists, updating")
		for _, d := range diffs.Differences() {
			log.Debug(d)
		}
		err = r.applyDiffs(ctx, diffs)
		if err != nil {
			return errors.Wrapf(err, "failed to update lambda function %v", r.Identifier())
		}
		return nil
	}

	return r.apply(ctx)
}

// Destroy the lambda function
func (r Function) Destroy(ctx context.Context) error {

	log.Debugf("destroying lambda function %v", r.Identifier())

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return errors.Wrapf(err, "error finding lambda function %v", r.Identifier())
	}

	if existing == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("lambda function does not exist, nothing to destroy, skippping ")
		return nil
	}

	client := client.Lambda(ctx, r.Context.ProviderName)

	// list aliases for this function
	out, err := client.ListAliases(ctx, &lambda.ListAliasesInput{
		FunctionName: aws.String(r.Identifier()),
	})
	if err != nil {
		return errors.Wrapf(err, "error listing aliases for fucntion %v", r.Identifier())
	}

	// delete all aliases
	for _, alias := range out.Aliases {
		_, err = client.DeleteAlias(ctx, &lambda.DeleteAliasInput{
			FunctionName: aws.String(r.Identifier()),
			Name:         alias.Name,
		})
		if err != nil {
			return errors.Wrapf(err, "error destroying alias %v for lambda function %v", alias.Name, r.Identifier())
		}
	}

	// delete all function urls
	if existing.FunctionUrl != nil {
		err := existing.FunctionUrl.destroy(ctx, r.Context, r.Identifier())
		if err != nil {
			return errors.Wrapf(err, "error destroying function url config for %v", r.Identifier())
		}
	}

	// delete the function
	_, err = client.DeleteFunction(ctx, &lambda.DeleteFunctionInput{
		FunctionName: aws.String(r.Identifier()),
	})
	if err != nil {
		return errors.Wrapf(err, "error destroying function %v", r.Identifier())
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Red("lambda function destroyed"))
	return nil
}

// FunctionDiff respresnts diffs between lambda function definition & AWS representation
type FunctionDiff struct {
	resource.BaseResourceDiff

	descriptionDiff    bool
	roleDiff           bool
	policyEquals       resource.EqualsResult
	publishRequired    bool // this means a publishable change is made & publish is set to true
	packageTypeDiff    bool
	imageDiff          bool
	imageOverridesDiff bool
	zipDiff            bool
	archsDiff          bool
	envDiff            bool
	storageDiff        bool
	memoryDiff         bool
	timeoutDiff        bool
	handlerDiff        bool
	runtimeDiff        bool
	vpcConfigDiff      bool
	layersDiff         bool
	loggingDiff        bool
	fileSystemDiff     bool
	functionUrlEquals  resource.EqualsResult
	aliasesDiff        bool
	aliasesToAdd       []LambdaAlias
	aliasesToUpdate    map[LambdaAlias]LambdaAliasDiff
	aliasesToDelete    []LambdaAlias
	recursionDiff      bool
	tagsDiff           bool
	tagDiff            util.TagDiffResult
}

type LambdaAliasDiff struct {
	resource.BaseResourceDiff

	aliasDiff  bool
	funcEquals resource.EqualsResult
	polEquals  resource.EqualsResult
}

// Compare fetches the existing lambda function and if it exists, checks if this
// resource is equal to the corresponding AWS lambda function
// resolveFileSystemArn resolves the configured fileSystem access point reference (ARN, fsap-* ID,
// or name) to its ARN. A no-op when no file system is configured or the ARN is already resolved,
// so it is safe to call from every lifecycle entry point. FileSystem is a pointer, so the resolved
// ARN persists for the rest of the resource lifecycle (e.g. from Compare into applyDiffs).
func (r Function) resolveFileSystemArn(ctx context.Context) error {
	if r.FileSystem == nil || r.FileSystem.Arn != nil {
		return nil
	}
	arn, err := awsw.NewEFS(ctx, r.Context.ProviderName).AccessPointArnForIdentifier(ctx, r.FileSystem.Name)
	if err != nil {
		return errors.Wrapf(err, "error resolving fileSystem access point %v for lambda function %v", r.FileSystem.Name, r.Identifier())
	}
	r.FileSystem.Arn = arn
	return nil
}

func (r Function) Compare(ctx context.Context) (resource.ResourceDiff, error) {

	// resolve the efs access point reference so the comparison below is against what AWS stores
	if err := r.resolveFileSystemArn(ctx); err != nil {
		return nil, err
	}

	existing, err := r.fetchExisting(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aws resource for %v", r.Identifier())
	}

	diffs := &FunctionDiff{}

	diff := false

	if existing == nil {
		diffs.Messages = append(diffs.Messages, "lambda function does not exist")
		return diffs, nil
	}

	diffs.Resource = existing

	// comparing description
	if r.Description != existing.Description {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("description will be updated %v -> %v", existing.Description, r.Description))
		diffs.descriptionDiff = true
	}

	// role
	if r.Role != existing.Role {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("execution role will be updated %v -> %v", existing.Role, r.Role))
		diffs.roleDiff = true
	}

	// package type & code here ..
	if util.Coalesce(r.PackageType, "Image") != util.Coalesce(existing.PackageType, "Image") {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("package is different: %v -> %v: %v", *existing.PackageType, *r.PackageType, resource.BuilditInvalidChangeTag))
		diffs.packageTypeDiff = true
	}

	// package type Image
	if util.Coalesce(r.Code.Image, "") != util.Coalesce(existing.Code.Image, "") {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("image for lambda function will be updated %v -> %v", util.Coalesce(existing.Code.Image, ""), util.Coalesce(r.Code.Image, "")))
		diffs.imageDiff = true
	}

	// package type Zip
	if *r.PackageType == string(types.PackageTypeZip) && *existing.PackageType == string(types.PackageTypeZip) {
		if util.Coalesce(r.Code._SHA256, "") != util.Coalesce(existing.Code._SHA256, "") ||
			util.CoalesceComparable(r.Code._CodeSize, 0) != util.CoalesceComparable(existing.Code._CodeSize, 0) {
			diff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("code package zip checksum is different %v -> %v", util.Coalesce(existing.Code._SHA256, ""), util.Coalesce(r.Code._SHA256, "")))
			diffs.zipDiff = true
		}
	}

	// image overrides
	if r.ImageOverrides != nil && existing.ImageOverrides == nil ||
		r.ImageOverrides == nil && existing.ImageOverrides != nil {
		diff = true
		diffs.Messages = append(diffs.Messages, "image overrides settings to be updated")
		diffs.imageOverridesDiff = true
	} else if r.ImageOverrides != nil && existing.ImageOverrides != nil {
		if !util.SliceElementsEqual[string](r.ImageOverrides.Command, existing.ImageOverrides.Command) {
			diff = true
			diffs.Messages = append(diffs.Messages, "image override commands to be updated")
			diffs.imageOverridesDiff = true
		}
		if !util.SliceElementsEqual[string](r.ImageOverrides.Entrypoint, existing.ImageOverrides.Entrypoint) {
			diff = true
			diffs.Messages = append(diffs.Messages, "image override entrypoints to be updated")
			diffs.imageOverridesDiff = true
		}
		if util.Coalesce(r.ImageOverrides.WorkingDirectory, "") != util.Coalesce(existing.ImageOverrides.WorkingDirectory, "") {
			diff = true
			diffs.Messages = append(diffs.Messages,
				fmt.Sprintf("image override working directories to be updated %v -> %v", util.Coalesce(existing.ImageOverrides.WorkingDirectory, ""), util.Coalesce(r.ImageOverrides.WorkingDirectory, "")))
			diffs.imageOverridesDiff = true
		}
	}

	// file system
	if r.FileSystem != nil && existing.FileSystem == nil ||
		r.FileSystem == nil && existing.FileSystem != nil {
		diff = true
		diffs.Messages = append(diffs.Messages, "file system settings to be updated")
		diffs.fileSystemDiff = true
	} else if r.FileSystem != nil && existing.FileSystem != nil {
		if r.FileSystem.LocalMountPath != existing.FileSystem.LocalMountPath {
			diff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("file system local mount path to be updated %v -> %v", r.FileSystem.LocalMountPath, existing.FileSystem.LocalMountPath))
			diffs.fileSystemDiff = true
		}
		if util.Coalesce(r.FileSystem.Arn, "") != util.Coalesce(existing.FileSystem.Arn, "") {
			diff = true
			diffs.Messages = append(diffs.Messages,
				fmt.Sprintf("file system arn override to be updated %v -> %v", util.Coalesce(r.FileSystem.Arn, ""), util.Coalesce(existing.FileSystem.Arn, "")))
			diffs.fileSystemDiff = true
		}
	}

	// runtime
	if r.Runtime != existing.Runtime {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("runtime to be updated %v -> %v", existing.Runtime, r.Runtime))
		diffs.runtimeDiff = true
	}

	// handlers
	if util.Coalesce(r.Handler, "") != util.Coalesce(existing.Handler, "") {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("handler to be updated %v -> %v", util.Coalesce(existing.Handler, ""), util.Coalesce(r.Handler, "")))
		diffs.handlerDiff = true
	}

	// layers
	if !layerRefSliceEquals(r.Layers, existing.Layers) {
		diff = true
		diffs.Messages = append(diffs.Messages, "layers to be updated")
		diffs.layersDiff = true
	}

	// logging
	if r.LogConfig.equals(existing.LogConfig) != resource.Equal {
		diff = true
		diffs.Messages = append(diffs.Messages, "logging configuration to be updated")
		diffs.loggingDiff = true
	}

	//architectures
	if util.DiffStringSlices(r.Architectures, existing.Architectures) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("architectures to be updated %v -> %v", existing.Architectures, r.Architectures))
		diffs.archsDiff = true
	}

	// environment
	if !util.StringMap(r.Environment).Equals(existing.Environment) {
		diff = true
		diffs.Messages = append(diffs.Messages, "environment variables to be updated")
		diffs.envDiff = true
	}

	// ephemeral storage
	if util.CoalesceInt32(r.EphemeralStorage, 512) != util.CoalesceInt32(existing.EphemeralStorage, 512) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("ephemeral storage to be updated %v -> %v", util.CoalesceInt32(existing.EphemeralStorage, 512), util.CoalesceInt32(r.EphemeralStorage, 512)))
		diffs.storageDiff = true
	}

	// memory
	if util.CoalesceInt32(r.Memory, 256) != util.CoalesceInt32(existing.Memory, 256) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("memory size to be updated %v -> %v", util.CoalesceInt32(existing.Memory, 256), util.CoalesceInt32(r.Memory, 256)))
		diffs.memoryDiff = true
	}

	// timeout
	if util.CoalesceInt32(r.Timeout, 3) != util.CoalesceInt32(existing.Timeout, 3) {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("timeout seconds to be updated %v -> %v", util.CoalesceInt32(existing.Timeout, 3), util.CoalesceInt32(r.Timeout, 3)))
		diffs.timeoutDiff = true
	}

	// vpc config
	if r.VpcConfig != nil && existing.VpcConfig == nil ||
		r.VpcConfig == nil && existing.VpcConfig != nil {
		diff = true
		diffs.Messages = append(diffs.Messages, "vpc configuration to be updated")
		diffs.vpcConfigDiff = true
	} else if r.VpcConfig != nil && existing.VpcConfig != nil {
		if !util.SliceElementsEqual[string](r.VpcConfig.SecurityGroups, existing.VpcConfig.SecurityGroups) {
			diff = true
			diffs.Messages = append(diffs.Messages, "vpc security groups to be updated")
			diffs.vpcConfigDiff = true
		}
		if !util.SliceElementsEqual[string](r.VpcConfig.Subnets, existing.VpcConfig.Subnets) {
			diff = true
			diffs.Messages = append(diffs.Messages, "vpc subnets to be updated")
			diffs.vpcConfigDiff = true
		}
		if util.Coalesce(r.VpcConfig.Name, "") != util.Coalesce(existing.VpcConfig.Name, "") {
			diff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("vpc name to be updated %v -> %v ", util.Coalesce(existing.VpcConfig.Name, ""), util.Coalesce(r.VpcConfig.Name, "")))
			diffs.vpcConfigDiff = true
		}
	}

	if util.CoalesceComparable[bool](r.ForcePublish, false) {
		diff = true
		diffs.publishRequired = true
		diffs.Messages = append(diffs.Messages, "force publish is set to true; a new lambda function version would be created at apply")
	} else if diff && util.CoalesceComparable[bool](r.Publish, false) {
		// at this point, if we have any changes & also publish is set to true, then
		// the changes need to be applied
		diff = true
		diffs.publishRequired = true
		diffs.Messages = append(diffs.Messages, "configuration or code changes were made & publish is set to true; a new lambda function version would be created at apply")
	}

	// policy
	var policyDiffs []string
	policyDiffs, diffs.policyEquals = r.Policy.equals(existing.Policy)
	if len(policyDiffs) > 0 {
		diffs.Messages = append(diffs.Messages, policyDiffs...)
	}
	switch diffs.policyEquals {
	case resource.LeftZero:
		diff = true
		diffs.Messages = append(diffs.Messages, "lambda function policy will be removed")
	case resource.RightZero:
		diff = true
		diffs.Messages = append(diffs.Messages, "lambda function policy will be added")
	case resource.NotEqual:
		diff = true
		diffs.Messages = append(diffs.Messages, "Lambda function policy will be updated")
	}

	// aliases
	aliasesToAddMap := make(map[string]LambdaAlias)
	for _, al := range r.Aliases {
		aliasesToAddMap[al.Name] = al
	}

	aliasesToUpdate := make(map[LambdaAlias]LambdaAliasDiff)
	var aliasesToAdd []LambdaAlias
	var aliasesToDelete []LambdaAlias

	for _, exa := range existing.Aliases {
		if def, ok := aliasesToAddMap[exa.Name]; !ok {
			//existing alias does not exist in toAdd list, then it needs to be deleted...
			diff = true
			diffs.Messages = append(diffs.Messages, fmt.Sprintf("alias to be remvoed %q", exa.Name))
			diffs.aliasesDiff = true
			aliasesToDelete = append(aliasesToDelete, exa)
			delete(aliasesToAddMap, exa.Name)
		} else {
			if equal, equalFn, equalPol := def.equals(exa); !equal {
				// existing alias is diff from the one in toAdd list, needs to be upated...
				diff = true
				diffs.Messages = append(diffs.Messages, fmt.Sprintf("alias to be updated %q", exa.Name))
				diffs.aliasesDiff = true
				aliasesToUpdate[def] = LambdaAliasDiff{
					BaseResourceDiff: resource.BaseResourceDiff{
						Resource: exa,
					},
					aliasDiff:  !equal,
					funcEquals: equalFn,
					polEquals:  equalPol,
				}
			}
			delete(aliasesToAddMap, exa.Name)
		}
	}

	if len(aliasesToAddMap) > 0 {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("%v aliases to be added", len(aliasesToAddMap)))
		diffs.aliasesDiff = true
		for _, v := range aliasesToAddMap {
			aliasesToAdd = append(aliasesToAdd, v)
		}
	}

	if diffs.aliasesDiff {
		diffs.aliasesToAdd = aliasesToAdd
		diffs.aliasesToDelete = aliasesToDelete
		diffs.aliasesToUpdate = aliasesToUpdate
	}

	// recursion
	if *r.TerminateRecursion != *existing.TerminateRecursion {
		diff = true
		diffs.Messages = append(diffs.Messages, fmt.Sprintf("recursion protection config to be updated %v -> %v", *r.TerminateRecursion, *existing.TerminateRecursion))
		diffs.recursionDiff = true
	}

	// function url diff
	diffs.functionUrlEquals = r.FunctionUrl.equals(existing.FunctionUrl)
	if diffs.functionUrlEquals != resource.Equal {
		diff = true
	}
	switch diffs.functionUrlEquals {
	case resource.LeftZero:
		diffs.Messages = append(diffs.Messages, "function url to be deleted")
	case resource.RightZero:
		diffs.Messages = append(diffs.Messages, "function url to be added")
	case resource.NotEqual:
		diffs.Messages = append(diffs.Messages, "function url to be updated")
	}

	// tags
	if tagDiff := resource.TagDiffForContext(ctx, existing.Tags, r.Tags); tagDiff.HasChanges() {
		diff = true
		diffs.tagsDiff = true
		diffs.tagDiff = tagDiff
		diffs.Messages = append(diffs.Messages, resource.TagDiffSummary(existing.Tags, diffs.tagDiff)...)
	}

	if !diff {
		return nil, nil
	}

	// return
	return diffs, nil
}

// fetchExisting fetches lambda function if exists
func (r Function) fetchExisting(ctx context.Context) (*Function, error) {

	client := client.Lambda(ctx, r.Context.ProviderName)

	// fetch the lambda func
	out, err := client.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(r.Identifier()),
	})

	if err != nil {
		// if lambda not found, then return nil obj
		var rnfe *types.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil, nil
		}
		// for other err's return err
		return nil, err
	}

	oCfg := out.Configuration
	if oCfg == nil {
		return nil, errors.Errorf("empty lambda configuration read for %v", r.Identifier())
	}

	// fetch all versions
	versions, err := r.listVersions(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching versions for lambda function %v", r.Identifier())
	}

	// architectures
	var archs []string
	for _, ar := range oCfg.Architectures {
		archs = append(archs, string(ar))

	}

	// epehemral storage
	var ephemeralStorage *int32
	if oCfg.EphemeralStorage != nil {
		ephemeralStorage = oCfg.EphemeralStorage.Size
	}

	// layer
	var layers []LayerRef
	for _, ly := range oCfg.Layers {
		layer, err := findLayerByArn(ctx, r.Context, ly.Arn)

		if err != nil {
			return nil, errors.Wrap(err, "error fetching lambda layer")
		}
		layers = append(layers, layer.LayerRef)
	}

	// image overrides
	var overrides *LambdaImageOverrides
	if oCfg.ImageConfigResponse != nil && oCfg.ImageConfigResponse.ImageConfig != nil {
		iCfg := oCfg.ImageConfigResponse.ImageConfig
		overrides = &LambdaImageOverrides{
			Command:          iCfg.Command,
			Entrypoint:       iCfg.EntryPoint,
			WorkingDirectory: iCfg.WorkingDirectory,
		}
	}

	// file system
	var fileSystem *FileSystem
	if len(oCfg.FileSystemConfigs) > 0 {
		fCfg := oCfg.FileSystemConfigs[0]
		fileSystem = &FileSystem{
			Arn:            fCfg.Arn,
			LocalMountPath: *fCfg.LocalMountPath,
			Name:           "",
		}
	}

	// vpc config
	var vpcConfig *VpcConfig
	if oCfg.VpcConfig != nil {
		vpcId := *oCfg.VpcConfig.VpcId
		vpcName, err := awsw.NewEC2(ctx, r.Context.ProviderName).VpcNameById(ctx, vpcId)
		if err != nil {
			return nil, errors.Wrapf(err, "error fetching vpc name for id %v", vpcId)
		}

		sgNames, err := awsw.NewEC2(ctx, r.Context.ProviderName).SecurityGroupNamesByIds(ctx, vpcName, oCfg.VpcConfig.SecurityGroupIds)
		if err != nil {
			return nil, errors.Wrapf(err, "error fetching security group names")
		}

		subnetNames, err := awsw.NewEC2(ctx, r.Context.ProviderName).SubnetNamesByIds(ctx, oCfg.VpcConfig.SubnetIds)
		if err != nil {
			return nil, errors.Wrapf(err, "erro fetching subnet names")
		}

		vpcConfig = &VpcConfig{
			Name:           vpcName,
			SecurityGroups: sgNames,
			Subnets:        subnetNames,
		}
	}

	// role
	// beware: this call to RoleNameForArn is slow like anything.
	// for some odd reason AWS doesn't have a civilized way of fetching
	// a role by its arn. you have to loop through all roles
	roleName, err := awsw.NewIAM(ctx, r.Context.ProviderName).RoleNameForArn(ctx, *oCfg.Role)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching role name for role arn")
	}

	// function urls
	furlMap := make(map[string]*FunctionUrlConfig)
	var functionUrlConfig *FunctionUrlConfig

	outUrls, err := client.ListFunctionUrlConfigs(ctx, &lambda.ListFunctionUrlConfigsInput{
		FunctionName: aws.String(r.Identifier()),
	})
	if err != nil {
		var rnfe *types.ResourceNotFoundException
		if !errors.As(err, &rnfe) {
			return nil, errors.Wrapf(err, "error fetching function url configuration for %v", r.Identifier())
		}
	}

	for _, u := range outUrls.FunctionUrlConfigs {
		// if the function url arn ends with the function name, then it is the
		// function url for $LATEST
		if strings.HasSuffix(*u.FunctionArn, r.Identifier()) {
			functionUrlConfig = &FunctionUrlConfig{
				InvokeMode: aws.String((string)(u.InvokeMode)),
				alias:      nil, //$LATEST
			}
		} else {
			// save to the map of arn->url config so we can
			// check with alias
			arn := *u.FunctionArn
			arnSegments := strings.Split(arn, ":")
			aliasStr := arnSegments[len(arnSegments)-1]
			furlMap[*u.FunctionArn] = &FunctionUrlConfig{
				InvokeMode: aws.String((string)(u.InvokeMode)),
				alias:      &aliasStr,
			}
		}
	}

	// logging
	var logging *LogConfig
	if oCfg.LoggingConfig != nil {
		logging = &LogConfig{
			ApplicationLogLevel: (string)(oCfg.LoggingConfig.ApplicationLogLevel),
			SystemLogLevel:      (string)(oCfg.LoggingConfig.SystemLogLevel),
			LogGroup:            oCfg.LoggingConfig.LogGroup,
			Format:              (*string)(&oCfg.LoggingConfig.LogFormat),
		}
	}

	// policy
	policy, err := r.fetchLambdaPolicy(ctx, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching policy for the lambda function %v", r.Identifier())
	}

	// alias
	var aliases []LambdaAlias
	out_Alias, err := client.ListAliases(ctx, &lambda.ListAliasesInput{
		FunctionName: aws.String(r.Name),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching aliases for lambda func %v", r.Identifier())
	}

	// var aliasName string
	for _, a := range out_Alias.Aliases {

		// check if a function url config was read
		// for this alias arn earlier; if yes, then
		// save it here...
		var furlConfig *FunctionUrlConfig
		if c, ok := furlMap[*a.AliasArn]; ok {
			furlConfig = c
		}

		policy, err := r.fetchLambdaPolicy(ctx, a.Name)
		if err != nil {
			return nil, errors.Wrapf(err, "error fetching policy for lambda function alias")
		}

		aliases = append(aliases, LambdaAlias{
			Name:        *a.Name,
			Description: *a.Description,
			Version:     *a.FunctionVersion,
			FunctionUrl: furlConfig,
			Policy:      policy,
		})
	}

	// environment
	var environment map[string]string
	if oCfg.Environment != nil {
		environment = oCfg.Environment.Variables
	}

	// code
	var sha256 *string
	var codeSize *int64
	if oCfg.PackageType == types.PackageTypeZip {
		sha256 = out.Configuration.CodeSha256
		codeSize = &out.Configuration.CodeSize
	}

	// recursion config
	rConfig, err := client.GetFunctionRecursionConfig(ctx, &lambda.GetFunctionRecursionConfigInput{
		FunctionName: aws.String(r.Identifier()),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error fetching recursion config for lambda function")
	}
	TerminateRecursion := rConfig.RecursiveLoop == types.RecursiveLoopTerminate

	existing := &Function{
		Name: util.Coalesce(oCfg.FunctionName, ""),
		Code: Code{
			Image:     out.Code.ImageUri,
			_SHA256:   sha256,
			_CodeSize: codeSize,
		},
		Role:               *roleName,
		Policy:             policy,
		Architectures:      archs,
		Description:        util.Coalesce(oCfg.Description, ""),
		Environment:        environment,
		EphemeralStorage:   ephemeralStorage,
		Memory:             oCfg.MemorySize,
		Handler:            oCfg.Handler,
		Layers:             layers,
		PackageType:        aws.String(string(oCfg.PackageType)),
		ImageOverrides:     overrides,
		FileSystem:         fileSystem,
		Runtime:            string(oCfg.Runtime),
		Timeout:            oCfg.Timeout,
		VpcConfig:          vpcConfig,
		LogConfig:          logging,
		Aliases:            aliases,
		TerminateRecursion: aws.Bool(TerminateRecursion),
		FunctionUrl:        functionUrlConfig,
		_existingVersions:  versions,
		_arn:               out.Configuration.FunctionArn,
		Tags:               out.Tags,
	}

	// if logger set to trace
	if log.GetLevel() == log.TraceLevel {
		bytes, _ := yaml.Marshal(existing)
		log.Trace(string(bytes))
	}

	return existing, nil
}

// apply provisions a new lambda function
func (r Function) apply(ctx context.Context) error {

	// a no-op when Compare already resolved the access point reference
	if err := r.resolveFileSystemArn(ctx); err != nil {
		return err
	}

	code := &types.FunctionCode{}
	if *r.PackageType == string(types.PackageTypeImage) {
		code.ImageUri = r.Code.Image
	} else {
		code.S3Bucket = r.Code.S3Bucket
		code.S3Key = r.Code.Key
		code.S3ObjectVersion = r.Code.ObjVersion
	}

	roleArn, err := awsw.NewIAM(ctx, r.Context.ProviderName).RoleArnForName(ctx, r.Role)
	if err != nil {
		return err
	}

	var archs []types.Architecture
	for _, a := range r.Architectures {
		archs = append(archs, types.Architecture(a))
	}

	var imageConfig *types.ImageConfig
	if r.ImageOverrides != nil {
		imageConfig = &types.ImageConfig{
			Command:          r.ImageOverrides.Command,
			EntryPoint:       r.ImageOverrides.Entrypoint,
			WorkingDirectory: r.ImageOverrides.WorkingDirectory,
		}
	}

	var fileConfig []types.FileSystemConfig
	if r.FileSystem != nil {
		fileConfig = append(fileConfig, types.FileSystemConfig{
			Arn:            r.FileSystem.Arn,
			LocalMountPath: aws.String(r.FileSystem.LocalMountPath),
		})
	}

	var vpcConfig *types.VpcConfig
	if r.VpcConfig != nil {

		groups, err := awsw.NewEC2(ctx, r.Context.ProviderName).SecurityGroupIdsByNames(ctx, nil, r.VpcConfig.SecurityGroups)
		if err != nil {
			return err
		}

		subnets, err := awsw.NewEC2(ctx, r.Context.ProviderName).SubnetIdsByNames(ctx, r.VpcConfig.Subnets)
		if err != nil {
			return err
		}

		vpcConfig = &types.VpcConfig{
			SecurityGroupIds: groups,
			SubnetIds:        subnets,
		}
	}

	// layers
	var layers []string
	if r.Layers != nil {
		layers, err = r.toLayerArnSlice(ctx)
		if err != nil {
			return err
		}
	}

	// logging
	var logging *types.LoggingConfig
	if r.LogConfig != nil {
		logging = &types.LoggingConfig{
			ApplicationLogLevel: types.ApplicationLogLevel(r.LogConfig.ApplicationLogLevel),
			SystemLogLevel:      types.SystemLogLevel(r.LogConfig.SystemLogLevel),
			LogFormat:           types.LogFormat(*r.LogConfig.Format),
			LogGroup:            r.LogConfig.LogGroup,
		}
	}

	input := &lambda.CreateFunctionInput{
		FunctionName:      aws.String(r.Identifier()),
		Description:       aws.String(r.Description),
		Code:              code,
		Role:              roleArn,
		Architectures:     archs,
		Environment:       &types.Environment{Variables: r.Environment},
		EphemeralStorage:  &types.EphemeralStorage{Size: r.EphemeralStorage},
		MemorySize:        r.Memory,
		Handler:           r.Handler,
		Layers:            layers,
		PackageType:       types.PackageType(*r.PackageType),
		ImageConfig:       imageConfig,
		FileSystemConfigs: fileConfig,
		Publish:           *r.Publish,
		Runtime:           types.Runtime(r.Runtime),
		Timeout:           r.Timeout,
		LoggingConfig:     logging,
		VpcConfig:         vpcConfig,
		Tags:              r.Tags,
	}

	client := client.Lambda(ctx, r.Context.ProviderName)
	out, err := client.CreateFunction(ctx, input)
	if err != nil {
		return errors.Wrapf(err, "error creating lambda function %v", r.Identifier())
	}

	arn := *out.FunctionArn
	versionCreated := *out.Version

	log.WithFields(log.Fields{
		"Name":    r.Identifier(),
		"Arn":     arn,
		"Version": versionCreated,
	}).Info(color.Green("lambda function created"))

	// wait for the creation
	err = r.waitForUpdate(ctx)
	if err != nil {
		return err
	}

	// create function url
	if r.FunctionUrl != nil {
		err = r.FunctionUrl.apply(ctx, r.Context, r.Identifier())
		if err != nil {
			return err
		}
	}

	// TODO: create policy
	if r.Policy != nil {
		err := r.Policy.apply(ctx, r.Context, r.Identifier(), nil)
		if err != nil {
			return err
		}
	}

	// create alias
	err = r.createAliases(ctx, r.Aliases, versionCreated)
	if err != nil {
		return err
	}

	// if recursion protection is off, then apply
	if !*r.TerminateRecursion {
		_, err = client.PutFunctionRecursionConfig(ctx, &lambda.PutFunctionRecursionConfigInput{
			FunctionName:  aws.String(r.Identifier()),
			RecursiveLoop: types.RecursiveLoopAllow,
		})
		if err != nil {
			return err
		}
		log.WithFields(log.Fields{
			"Function":  r.Identifier(),
			"Recursion": string(types.RecursiveLoopAllow),
		}).Info(color.Green("lambda function recursion config set to allow"))
	}

	return nil
}

// applyDiffs applies diffs to an existing lambda function
func (r Function) applyDiffs(ctx context.Context, diffs resource.ResourceDiff) error {

	if diffs == nil {
		log.WithFields(log.Fields{
			"Name": r.Identifier(),
		}).Info("no updates required for lambda function")
		return nil
	}

	lamDiffs, ok := diffs.(*FunctionDiff)
	if !ok {
		return errors.New("invalid diff type supplied")
	}

	// a no-op when Compare already resolved the access point reference
	if err := r.resolveFileSystemArn(ctx); err != nil {
		return err
	}

	// fetch existing sg
	existing, ok := lamDiffs.Resource.(*Function)
	if !ok {
		return errors.Errorf("cannot retrieve existing lambda function")
	}

	// check invalid diff
	if lamDiffs.packageTypeDiff {
		return errors.Errorf("package type cannot be modified for a lambda function")
	}

	var err error
	client := client.Lambda(ctx, r.Context.ProviderName)

	// fetch the highestPublishedVersion currently
	// this may change if the resource is updated
	highestPublishedVersion := existing._existingVersions[len(existing._existingVersions)-1] //ordered

	// update other config
	var configDiffsFound bool
	var codeDiffsFound bool

	// config

	var description *string
	if lamDiffs.descriptionDiff {
		configDiffsFound = true
		description = aws.String(r.Description)
	}

	// ephemeral storage
	var storage *types.EphemeralStorage
	if lamDiffs.storageDiff {
		configDiffsFound = true
		storage = &types.EphemeralStorage{
			Size: r.EphemeralStorage,
		}
	}

	// memory size
	var memory *int32
	if lamDiffs.memoryDiff {
		configDiffsFound = true
		memory = r.Memory
	}

	// role
	var roleArn *string
	if lamDiffs.roleDiff {
		configDiffsFound = true
		roleArn, err = awsw.NewIAM(ctx, r.Context.ProviderName).RoleArnForName(ctx, r.Role)
		if err != nil {
			return err
		}
	}

	// timeout
	var timeout *int32
	if lamDiffs.timeoutDiff {
		configDiffsFound = true
		timeout = r.Timeout
	}

	var environment *types.Environment
	if lamDiffs.envDiff {
		configDiffsFound = true
		environment = &types.Environment{
			Variables: r.Environment,
		}
	}

	var handler *string
	if lamDiffs.handlerDiff {
		configDiffsFound = true
		handler = aws.String(*r.Handler)
	}

	var imageConfig *types.ImageConfig
	if lamDiffs.imageOverridesDiff {
		configDiffsFound = true
		if r.ImageOverrides != nil {
			imageConfig = &types.ImageConfig{
				Command:          r.ImageOverrides.Command,
				EntryPoint:       r.ImageOverrides.Entrypoint,
				WorkingDirectory: r.ImageOverrides.WorkingDirectory,
			}
		}
	}

	var fileConfig []types.FileSystemConfig
	if lamDiffs.fileSystemDiff {
		configDiffsFound = true
		if r.FileSystem != nil {
			fileConfig = append(fileConfig, types.FileSystemConfig{
				Arn:            r.FileSystem.Arn,
				LocalMountPath: aws.String(r.FileSystem.LocalMountPath),
			})
		}
	}

	// runtime
	var runtime types.Runtime
	if lamDiffs.runtimeDiff {
		configDiffsFound = true
		runtime = types.Runtime(r.Runtime)
	}

	// layers
	var layers []string
	if lamDiffs.layersDiff {
		configDiffsFound = true
		layers, err = r.toLayerArnSlice(ctx)
		if err != nil {
			return err
		}
	}

	var logging *types.LoggingConfig
	if lamDiffs.loggingDiff {
		if r.LogConfig != nil {
			configDiffsFound = true
			logging = &types.LoggingConfig{
				ApplicationLogLevel: types.ApplicationLogLevel(r.LogConfig.ApplicationLogLevel),
				SystemLogLevel:      types.SystemLogLevel(r.LogConfig.SystemLogLevel),
				LogFormat:           types.LogFormat(*r.LogConfig.Format),
				LogGroup:            r.LogConfig.LogGroup,
			}
		}
	}

	// vpc config
	var vpcConfig *types.VpcConfig
	if lamDiffs.vpcConfigDiff {
		configDiffsFound = true
		if r.VpcConfig != nil {

			groups, err := awsw.NewEC2(ctx, r.Context.ProviderName).SecurityGroupIdsByNames(ctx, nil, r.VpcConfig.SecurityGroups)
			if err != nil {
				return err
			}

			subnets, err := awsw.NewEC2(ctx, r.Context.ProviderName).SubnetIdsByNames(ctx, r.VpcConfig.Subnets)
			if err != nil {
				return err
			}

			vpcConfig = &types.VpcConfig{
				SecurityGroupIds: groups,
				SubnetIds:        subnets,
			}
		}
	}

	if configDiffsFound {
		out, err := client.UpdateFunctionConfiguration(ctx, &lambda.UpdateFunctionConfigurationInput{
			FunctionName:      aws.String(r.Identifier()),
			Description:       description,
			Role:              roleArn,
			EphemeralStorage:  storage,
			MemorySize:        memory,
			Timeout:           timeout,
			Environment:       environment,
			Handler:           handler,
			ImageConfig:       imageConfig,
			FileSystemConfigs: fileConfig,
			Runtime:           runtime,
			Layers:            layers,
			LoggingConfig:     logging,
			VpcConfig:         vpcConfig,
		})
		if err != nil {
			return errors.Wrapf(err, "error updating lambda function configuration %v", r.Identifier())
		}

		log.WithFields(log.Fields{
			"Name":    r.Identifier(),
			"Version": *out.Version,
		}).Info(color.Yellow("lambda function configuration updated"))

		// need to wait till the update is complete...
		err = r.waitForUpdate(ctx)
		if err != nil {
			return err
		}
	}

	// code changes
	var archs []types.Architecture
	if lamDiffs.archsDiff {
		codeDiffsFound = true
		for _, a := range r.Architectures {
			archs = append(archs, types.Architecture(a))
		}
	}

	var image *string
	if lamDiffs.imageDiff {
		codeDiffsFound = true
		if r.Code.Image != nil {
			image = r.Code.Image
		}
	}

	var bucket, key, version *string
	if lamDiffs.zipDiff {
		codeDiffsFound = true
		bucket = r.Code.S3Bucket
		key = r.Code.Key
		version = r.Code.ObjVersion
	}

	if codeDiffsFound {
		out, err := client.UpdateFunctionCode(ctx, &lambda.UpdateFunctionCodeInput{
			FunctionName:    aws.String(r.Identifier()),
			ImageUri:        image,
			S3Bucket:        bucket,
			S3Key:           key,
			S3ObjectVersion: version,
			Architectures:   archs,
		})

		if err != nil {
			return errors.Wrapf(err, "error updating lambda function code for %v", r.Identifier())
		}

		log.WithFields(log.Fields{
			"Name":    r.Identifier(),
			"Version": *out.Version,
		}).Info(color.Yellow("lambda function configuration updated"))

		// need to wait till the update is complete...
		err = r.waitForUpdate(ctx)
		if err != nil {
			return err
		}
	}

	if lamDiffs.tagsDiff {
		upserts := lamDiffs.tagDiff.Upserts()
		if len(upserts) > 0 {
			err = awsw.NewLambda(ctx, r.Context.ProviderName).AddResourceTags(ctx, *existing._arn, upserts)
			if err != nil {
				return err
			}
		}

		if len(lamDiffs.tagDiff.Deleted) > 0 {
			err = awsw.NewLambda(ctx, r.Context.ProviderName).DeleteResourceTags(ctx, *existing._arn, lamDiffs.tagDiff.Deleted)
			if err != nil {
				return err
			}
		}

	}

	// publish the $LATEST
	if lamDiffs.publishRequired {
		log.Info("updates were made to lambda configuration or code; Publish is set to true;  buildit will publish changes")
		out, err := client.PublishVersion(ctx, &lambda.PublishVersionInput{
			FunctionName: aws.String(r.Identifier()),
		})
		if err != nil {
			return errors.Wrapf(err, "error publishing lambda function %v", r.Identifier())
		}

		log.WithFields(log.Fields{
			"Name":    r.Identifier(),
			"Version": *out.Version,
		}).Info(color.Yellow("lambda function published"))

		highestPublishedVersion = *out.Version // update the hightest published version, which is currently the new update
	}

	// alias
	if lamDiffs.aliasesDiff {

		// check if this lambda is published
		err = r.createAliases(ctx, lamDiffs.aliasesToAdd, highestPublishedVersion)
		if err != nil {
			return err
		}

		err = r.deleteAliases(ctx, lamDiffs.aliasesToDelete)
		if err != nil {
			return err
		}

		err = r.updateAliases(ctx, lamDiffs.aliasesToUpdate, highestPublishedVersion)
		if err != nil {
			return err
		}
	}

	// recursion
	if lamDiffs.recursionDiff {
		recursiveLoop := types.RecursiveLoopTerminate
		if !*r.TerminateRecursion {
			recursiveLoop = types.RecursiveLoopAllow
		}
		_, err = client.PutFunctionRecursionConfig(ctx, &lambda.PutFunctionRecursionConfigInput{
			FunctionName:  aws.String(r.Identifier()),
			RecursiveLoop: recursiveLoop,
		})
		if err != nil {
			return err
		}
		log.WithFields(log.Fields{
			"Function":  r.Identifier(),
			"Recursion": string(recursiveLoop),
		}).Info(color.Yellow("lambda function recursion config updated"))
	}

	// if function url to be created
	switch lamDiffs.functionUrlEquals {
	case resource.LeftZero:
		// delete existing
		err = existing.FunctionUrl.destroy(ctx, r.Context, r.Identifier())
		if err != nil {
			return err
		}
	case resource.RightZero:
		// create new
		err = r.FunctionUrl.apply(ctx, r.Context, r.Identifier())
		if err != nil {
			return err
		}
	case resource.NotEqual:
		// update
		err = r.FunctionUrl.update(ctx, r.Context, r.Identifier())
		if err != nil {
			return err
		}
	}

	switch lamDiffs.policyEquals {
	case resource.LeftZero:
		// delete existing
		existing := existing.Policy
		err = existing.destroy(ctx, r.Context, r.Identifier(), nil)
		if err != nil {
			return err
		}
	case resource.RightZero:
		// create
		err = r.Policy.apply(ctx, r.Context, r.Identifier(), nil)
		if err != nil {
			return err
		}
	case resource.NotEqual:
		// unfortunately there is not update,
		// you have to delete existing statementsIds & create the new ones
		// destroy
		existing := existing.Policy
		err = existing.destroy(ctx, r.Context, r.Identifier(), nil)
		if err != nil {
			return err
		}
		// create
		err = r.Policy.apply(ctx, r.Context, r.Identifier(), nil)
		if err != nil {
			return err
		}
	}

	log.WithFields(log.Fields{
		"Name": r.Identifier(),
	}).Info(color.Yellow("lambda function updated"))

	return nil
}

// private

// waitForUpdate waits 15s for 15 retries for lambda function to reach ACTIVE state
func (r Function) waitForUpdate(ctx context.Context) error {
	log.Info("waiting for lambda function updates before continuing...")
	client := client.Lambda(ctx, r.Context.ProviderName)

	waiter := lambda.NewFunctionUpdatedV2Waiter(client, func(f *lambda.FunctionUpdatedV2WaiterOptions) {
		f.MinDelay = time.Second * time.Duration(3)  // 3s
		f.MaxDelay = time.Second * time.Duration(15) // 15s
	})

	_, err := waiter.WaitForOutput(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(r.Identifier())},
		1*time.Minute)

	return err
}

// toLayerArnSlice converts the []Layer to []string with each index containing the arn of the Layer
// the output is a []string of qualified arns for the layers.
// if: VersionArn is set, simply use that as-is,
// else: check for Arn & Version to build a version qualified arn
// else: fetch the arn for the Name & build a version qualified arn
func (r Function) toLayerArnSlice(ctx context.Context) ([]string, error) {
	var layers []string
	for _, l := range r.Layers {
		var arn string
		if l.VersionArn != nil {
			arn = *l.VersionArn
		} else if l.Arn != nil {
			arn = fmt.Sprintf("%v:%v", *l.Arn, l.Version)
		} else {
			awsl, err := findLayerByNameAndVersion(ctx, r.Context, l.Name, l.Version)
			if err != nil {
				return nil, errors.Wrapf(err, "layer %v version %v not found", l.Name, l.Version)
			}
			arn = *awsl.VersionArn // take the version
		}
		layers = append(layers, arn)
	}
	return layers, nil
}

// alias related

// createAlias creates the specified aliaes
func (r Function) createAliases(ctx context.Context, aliases []LambdaAlias, highestPublishedVersion string) error {

	client := client.Lambda(ctx, r.Context.ProviderName)
	for _, alias := range aliases {

		ver := alias.Version
		if strings.ToLower(ver) == HighestPublishedVersionKey {
			ver = highestPublishedVersion
		}

		out2, err := client.CreateAlias(ctx, &lambda.CreateAliasInput{
			FunctionName:    aws.String(r.Identifier()),
			FunctionVersion: aws.String(ver),
			Name:            aws.String(alias.Name),
			Description:     aws.String(alias.Description),
		})
		if err != nil {
			return errors.Wrapf(err, "error creating alias %v for lambda function %v", alias.Name, r.Identifier())
		}

		log.WithFields(log.Fields{
			"Function Name": r.Identifier(),
			"Alias Name":    *out2.Name,
			"Version":       *out2.FunctionVersion,
		}).Info(color.Green("lambda function alias created"))

		// create alias
		if alias.FunctionUrl != nil {
			alias.FunctionUrl.alias = &alias.Name // set this name
			err = alias.FunctionUrl.apply(ctx, r.Context, r.Identifier())
			if err != nil {
				return errors.Wrapf(err, "error creating function url config for alias %v", alias.Name)
			}
		}

		// create policy
		if alias.Policy != nil {
			err := alias.Policy.apply(ctx, r.Context, r.Identifier(), &alias.Name)
			if err != nil {
				return errors.Wrapf(err, "error creating policy for alias %v", alias.Name)
			}
		}
	}

	return nil
}

// updateAliases updates the alias from the supplied map[LambdaAlias]LambdaAliasDiff for this lambda function
func (r Function) updateAliases(ctx context.Context, aliases map[LambdaAlias]LambdaAliasDiff, highestPublishedVersion string) error {

	client := client.Lambda(ctx, r.Context.ProviderName)
	for alias, diff := range aliases {

		// update alias
		if diff.aliasDiff {
			ver := alias.Version
			if strings.ToLower(ver) == HighestPublishedVersionKey {
				ver = highestPublishedVersion
			}

			out, err := client.UpdateAlias(ctx, &lambda.UpdateAliasInput{
				FunctionName:    aws.String(r.Identifier()),
				FunctionVersion: aws.String(ver),
				Name:            aws.String(alias.Name),
				Description:     aws.String(alias.Description),
			})
			if err != nil {
				return errors.Wrapf(err, "error updating alias %v for lambda function %v", alias.Name, r.Identifier())
			}
			log.WithFields(log.Fields{
				"Function Name": r.Identifier(),
				"Alias Name":    *out.Name,
				"Version":       *out.FunctionVersion,
			}).Info(color.Yellow("lambda function alias updated"))
		}

		// update alias function url config
		var err error
		switch diff.funcEquals {
		case resource.LeftZero:
			// delete existing
			existing := diff.Resource.(LambdaAlias).FunctionUrl
			existing.alias = &alias.Name // make sure we apply the alias name
			err = existing.destroy(ctx, r.Context, r.Identifier())
			if err != nil {
				return err
			}
		case resource.RightZero:
			// create
			alias.FunctionUrl.alias = &alias.Name // make sure we apply alias
			err = alias.FunctionUrl.apply(ctx, r.Context, r.Identifier())
			if err != nil {
				return err
			}
		case resource.NotEqual:
			// update
			alias.FunctionUrl.alias = &alias.Name // make sure we apply alias
			err = alias.FunctionUrl.update(ctx, r.Context, r.Identifier())
			if err != nil {
				return err
			}
		}

		// update alias function url config
		switch diff.polEquals {
		case resource.LeftZero:
			// delete existing
			existing := diff.Resource.(LambdaAlias).Policy
			err = existing.destroy(ctx, r.Context, r.Identifier(), &alias.Name)
			if err != nil {
				return err
			}
		case resource.RightZero:
			// create
			err = alias.Policy.apply(ctx, r.Context, r.Identifier(), &alias.Name)
			if err != nil {
				return err
			}
		case resource.NotEqual:
			// unfortunately there is not update,
			// you have to delete existing statementsIds & create the new ones
			// destroy
			existing := diff.Resource.(LambdaAlias).Policy
			err = existing.destroy(ctx, r.Context, r.Identifier(), &alias.Name)
			if err != nil {
				return err
			}
			// create
			err = alias.Policy.apply(ctx, r.Context, r.Identifier(), &alias.Name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteAliases delete the supplied alias names for this lambda function
func (r Function) deleteAliases(ctx context.Context, aliases []LambdaAlias) error {

	client := client.Lambda(ctx, r.Context.ProviderName)

	for _, alias := range aliases {

		// delete any function url configs
		if alias.FunctionUrl != nil {
			err := alias.FunctionUrl.destroy(ctx, r.Context, r.Identifier())
			if err != nil {
				return errors.Wrapf(err, "error destroying lambda function url config for %v, alias %v", r.Identifier(), alias.Name)
			}
		}

		_, err := client.DeleteAlias(ctx, &lambda.DeleteAliasInput{
			FunctionName: aws.String(r.Identifier()),
			Name:         aws.String(alias.Name),
		})
		if err != nil {
			return errors.Wrapf(err, "error deleting alias %v for lambda function %v", alias.Name, r.Identifier())
		}

		log.WithFields(log.Fields{
			"Function Name": r.Identifier(),
			"Alias Name":    alias.Name,
		}).Info(color.Red("lambda function alias deleted"))
	}
	return nil
}

// fetch lambda policy from Aws, for a function, alias or version
func (r Function) fetchLambdaPolicy(ctx context.Context, qualifier *string) (*Policy, error) {

	client := client.Lambda(ctx, r.Context.ProviderName)
	outp, err := client.GetPolicy(ctx, &lambda.GetPolicyInput{
		FunctionName: aws.String(r.Identifier()),
		Qualifier:    qualifier,
	})
	if err != nil {
		var rnfe *types.ResourceNotFoundException
		if errors.As(err, &rnfe) {
			return nil, nil
		}
		return nil, errors.Wrapf(err, "error fetching permissions policies for the function %v", r.Identifier())
	}

	policy, err := unmarshalPolicy(outp.Policy)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing permissions policies for the function %v", r.Identifier())
	}

	return policy, nil
}

// helpers

// listVersions returns a list of version for this lambda
func (r Function) listVersions(ctx context.Context) ([]string, error) {

	var marker *string
	var done bool
	var versions []string

	client := client.Lambda(ctx, r.Context.ProviderName)

	for !done {
		out, err := client.ListVersionsByFunction(ctx, &lambda.ListVersionsByFunctionInput{
			FunctionName: aws.String(r.Identifier()),
			Marker:       marker,
		})

		if err != nil {
			return nil, err
		}

		for _, ver := range out.Versions {
			versions = append(versions, *ver.Version)
		}

		marker = out.NextMarker
		done = out.NextMarker == nil // no next marker means it's done

	}

	return versions, nil
}
