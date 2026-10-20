package firehose

import "github.com/TouchBistro/buildit/resource"

// FirehoseDeliveryStream satisfies the lifecycle interfaces; see
// resource/resource_lifecycle.go for the receiver rule this assertion enforces.
var (
	_ resource.NormalizableResource = (*FirehoseDeliveryStream)(nil)
	_ resource.ValidatableResource  = (*FirehoseDeliveryStream)(nil)
)
