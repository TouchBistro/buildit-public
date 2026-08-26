package resource

import (
	"context"
	"strings"

	"github.com/TouchBistro/buildit/client"
	"github.com/TouchBistro/goutils/color"
)

var (
	BuilditInvalidChangeTag = color.Red("*** This is an invalid change ***")
)

// ValidationError represents a resource having failed validation.
// It contains the resource identifier, type, and a list of
// validation failure messages.
type ValidationError struct {
	ResourceIdentifier string
	ResourceType       string
	Messages           []string
}

func (ve *ValidationError) Error() string {
	var sb strings.Builder

	sb.WriteString(ve.ResourceType)
	sb.WriteString(": ")
	sb.WriteString(ve.ResourceIdentifier)
	sb.WriteString(": ")

	for i, msg := range ve.Messages {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(msg)
	}

	return sb.String()
}

// Resource represents a resource that can exist in AWS.
// This interface describes the fundamental behaviour required.
type Resource interface {
	// Key identifiees the resources in a buildit context (a buildit run)
	Key() Key
	// Identifier uniquely identifies the resource within a buildit scope (provider)
	Identifier() string
	// Apply creates or updates the resource on AWS.
	//
	// The provided context can be used to terminate the
	// operation if it becomes done before the operation
	// completes on its own.
	Apply(ctx context.Context) error
	// Destroy deletes the resource on AWS.
	// If the resource does not exist, Destroy should no-op
	// and not return an error.
	//
	// The provided context can be used to terminate the
	// operation if it becomes done before the operation
	// completes on its own.
	Destroy(ctx context.Context) error
	// TODO: Make resources implement String()
}

// Context is a resource context type that holds buildit's runtime context.
// for now this will only store the `provider` scope for each resource
type Context struct {
	ProviderName string
	MaxRetries   int
	MaxBackoff   int
}

// UnmarshalYAML implements the Unmarsher interface so we can
// unmarshall a Context from string in yaml
func (e *Context) UnmarshalYAML(unmarshal func(interface{}) error) error {
	rawProvider := ""
	err := unmarshal(&rawProvider)
	if err != nil {
		return err
	}

	if rawProvider == "" {
		rawProvider = client.MainProvider
	}

	e.ProviderName = rawProvider
	return nil
}

type BaseResource struct {
	Context Context `yaml:"provider"`
}
