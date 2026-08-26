package lambda

// Code defines the image or pakcage source
type Code struct {
	Image      *string `yaml:"image,omitempty" `  // image only
	S3Bucket   *string `yaml:"bucket,omitempty"`  // zip only
	Key        *string `yaml:"key,omitempty"`     // zip only
	ObjVersion *string `yaml:"version,omitempty"` // zip only
	_SHA256    *string `yaml:"-,omitempty"`       // zip only, only read for comparison
	_CodeSize  *int64  `yaml:"-,omitempty"`       // zip only, only read for comparison
}
