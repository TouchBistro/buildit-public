package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/TouchBistro/buildit/resource"
	"github.com/pkg/errors"
)

// validationSummary is the one-line form of a ValidationErrors, and the whole message the
// CLI ends a failed run with.
const validationSummary = "One or more resources failed validation"

// ValidationErrors is every problem found while the resources of a config were tagged,
// normalized and validated. Generate returns it as a value, so a library consumer can
// name the resources that failed (errors.As finds each *resource.ValidationError) and
// the CLI can print them once.
type ValidationErrors []error

// Error renders the summary followed by one block per failure.
func (e ValidationErrors) Error() string {
	return validationSummary + ":\n" + strings.Join(e.Lines(), "\n")
}

// Summary is the one-line form, for a caller that prints Lines itself.
func (e ValidationErrors) Summary() string {
	return validationSummary
}

// Lines returns the failures as the lines the CLI prints: a `name (type)` heading with
// its messages indented below, or a single indented line for any other error.
func (e ValidationErrors) Lines() []string {
	var lines []string
	for _, err := range e {
		// Asserted, not errors.As: a wrapped ValidationError carries context in its own
		// message that the heading form would drop.
		if verr, ok := err.(*resource.ValidationError); ok {
			lines = append(lines, fmt.Sprintf("%v (%v)", verr.ResourceIdentifier, verr.ResourceType))
			for _, msg := range verr.Messages {
				lines = append(lines, fmt.Sprintf("\t %v", msg))
			}
			continue
		}
		lines = append(lines, fmt.Sprintf("\t %v", err))
	}
	return lines
}

// Unwrap exposes the failures to errors.Is and errors.As.
func (e ValidationErrors) Unwrap() []error {
	return e
}

// graphBuilder is the one implementation of the steps every resource takes on its way
// into the graph, whichever source it came from (a config file, a ResourceSet AddX
// method, an override):
//
//  1. resolve the effective provider and name
//  2. reject reserved tag keys in the definition (or any tags, on an untaggable type)
//  3. hand the resource the global tags plus its buildit:resource-id
//  4. Normalize
//  5. check buildit's tags reached the resource
//  6. Validate
//  7. add the vertex, with its dependencies
//
// admit runs them. Failures in 2, 4, 5, 6 and 7 are collected, so one run reports
// every problem; build returns them together.
type graphBuilder struct {
	cfg   *Config
	graph *Graph
	errs  ValidationErrors
}

func (b *graphBuilder) addErr(err error) {
	if err != nil {
		b.errs = append(b.errs, err)
	}
}

// wiring points at the fields of a resource the steps read and write. Resource types
// expose these as struct fields rather than methods, so each type's loop in buildGraph
// names them once and admit does the rest.
type wiring struct {
	name      *string
	context   *resource.Context
	tags      *map[string]string
	dependsOn *[]resource.Key
	// globalTags is nil for a type AWS cannot tag.
	globalTags *map[string]string

	// legacyDelim accepts `provider/name` as well as `provider::name` in the declared
	// name, for the types that predate the `::` form.
	legacyDelim bool
	// providerFrom, when set, is a second `provider/name` value whose provider wins over
	// the one in the declared name.
	providerFrom string
	// skipValidate keeps a type whose Validate was never called at config load out of
	// validation, so existing configs keep loading.
	skipValidate bool
}

// admit takes a resource declared under name through every step. The error
// it returns is a config-shape problem (an unknown or ambiguous provider) that stops the
// build; validation problems are collected on the builder.
func admit[T resource.ValidatableResource, PT interface {
	*T
	resource.NormalizableResource
}](ctx context.Context, b *graphBuilder, name string, r PT, w wiring) error {
	delim := resource.DefaultKeyParseDelim
	if w.legacyDelim && !strings.Contains(name, delim) {
		delim = "/"
	}

	provider, id, err := b.cfg.getEffectiveProviderAndIdWithDelim(name, *w.context, delim)
	if err != nil {
		return err
	}
	if w.providerFrom != "" {
		if provider, _, err = b.cfg.getEffectiveProviderAndIdWithDelim(w.providerFrom, *w.context, "/"); err != nil {
			return err
		}
	}

	*w.name = *id
	w.context.ProviderName = *provider

	typeName := resourceTypeName(r)
	tagged := w.globalTags != nil

	if tagged {
		b.addErr(checkReservedTags(typeName, name, *w.tags))
		*w.globalTags = b.cfg.tagsFor(*id)
	} else if err := checkUnsupportedTags(typeName, name, *w.tags); err != nil {
		// Before Normalize, and skipping the resource: see checkUnsupportedTags.
		b.addErr(err)
		return nil
	}

	if err := r.Normalize(ctx); err != nil {
		b.addErr(errors.Wrapf(err, "failed normalizing %s %q", typeName, name))
		return nil
	}

	if tagged {
		b.addErr(checkBuilditTagsApplied(typeName, name, *w.tags))
	}

	if !w.skipValidate {
		if err := (*r).Validate(ctx); err != nil {
			b.addErr(err)
			return nil
		}
	}

	b.addErr(b.graph.AddVertex(*r, *w.dependsOn))
	return nil
}

// build finishes the graph: every collected failure is returned together, and only a
// graph with none gets its edges. A graph whose edges form a cycle is an error, so every
// caller of Generate gets a graph that can be ordered.
func (b *graphBuilder) build() error {
	if len(b.errs) > 0 {
		return b.errs
	}

	for to, deps := range b.graph.depdendencies {
		for _, from := range deps {
			if err := b.graph.AddEdge(from, to); err != nil {
				return err
			}
		}
	}

	if cycle := b.graph.Cycle(); cycle != nil {
		names := make([]string, len(cycle))
		for i, k := range cycle {
			names[i] = k.String()
		}
		return errors.Errorf("dependency cycle between resources: %s", strings.Join(names, " -> "))
	}
	return nil
}
