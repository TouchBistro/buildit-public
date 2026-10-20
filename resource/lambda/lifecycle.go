package lambda

import "github.com/TouchBistro/buildit/resource"

// Top-level lambda resources satisfy the lifecycle interfaces; see
// resource/resource_lifecycle.go for the receiver rule these assertions enforce.
var (
	_ resource.NormalizableResource = (*Function)(nil)
	_ resource.ValidatableResource  = (*Function)(nil)
	_ resource.NormalizableResource = (*Layer)(nil)
	_ resource.ValidatableResource  = (*Layer)(nil)
)
