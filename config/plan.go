package config

import (
	"context"
	"reflect"

	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/color"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// PlanAction is the outcome a plan determined for one resource.
type PlanAction int

const (
	// PlanCreate: Compare found no existing AWS resource; it will be provisioned.
	PlanCreate PlanAction = iota
	// PlanUpdate: an AWS resource exists and differs from the definition.
	PlanUpdate
	// PlanNoChange: the AWS resource matches the definition.
	PlanNoChange
	// PlanUnsupported: the resource type does not implement ComparableResource.
	PlanUnsupported
	// PlanSkipped: a non-empty targets list excluded the resource; Compare was not called.
	PlanSkipped
)

var planActionNames = [...]string{"create", "update", "no-change", "unsupported", "skipped"}

func (a PlanAction) String() string {
	if int(a) < 0 || int(a) >= len(planActionNames) {
		return "unknown"
	}
	return planActionNames[a]
}

// PlanEntry is the machine-readable plan result for a single resource.
type PlanEntry struct {
	Key resource.Key
	// Type is the Go type name of the resource, as reported by reflect.
	Type   string
	Action PlanAction
	// Differences is ResourceDiff.Differences(); nil unless Action is PlanUpdate.
	Differences []string
	// AWSResource is ResourceDiff.AWSResource(); nil unless Action is PlanUpdate.
	AWSResource any
	// Changes is DetailedResourceDiff.Changes() when the diff implements that
	// interface and Action is PlanUpdate; nil for every other diff. Resources
	// adopt it one at a time, so a nil here means "no structured detail", not
	// "no differences": Differences is always populated for an update.
	Changes []resource.Change
}

// PlanResult walks the graph in dependency order, calls Compare on every targeted
// ComparableResource, and returns one PlanEntry per vertex. It performs no
// logging; see PlanTargets for the human rendering. A Compare error aborts the
// plan and returns the entries collected so far alongside the error.
func (g *Graph) PlanResult(ctx context.Context, targets []resource.Key) ([]PlanEntry, error) {
	entries := make([]PlanEntry, 0, g.Size())
	err := g.planEach(ctx, targets, func(e PlanEntry) { entries = append(entries, e) })
	return entries, err
}

// planEach is the traversal behind PlanResult and PlanTargets. It hands each
// entry to visit as soon as that resource's Compare returns, so a renderer can
// print progressively instead of after the last AWS call. A Compare error stops
// the walk; entries already visited stay visited.
func (g *Graph) planEach(ctx context.Context, targets []resource.Key, visit func(PlanEntry)) error {
	sorted, err := g.toposort()
	if err != nil {
		return errors.Wrap(err, "failed to toposort resource graph")
	}

	for _, n := range sorted {
		v := g.vertices[n]
		entry := PlanEntry{Key: n, Type: reflect.TypeOf(v.resource).String()}

		if len(targets) > 0 && !util.ContainsComparable(targets, n) {
			entry.Action = PlanSkipped
			visit(entry)
			continue
		}

		cr, ok := v.resource.(resource.ComparableResource)
		if !ok {
			entry.Action = PlanUnsupported
			visit(entry)
			continue
		}

		diff, err := cr.Compare(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed comparing resource %s", n)
		}
		switch {
		case diff != nil && diff.AWSResource() == nil:
			entry.Action = PlanCreate
		case diff != nil && len(diff.Differences()) > 0:
			entry.Action = PlanUpdate
			entry.Differences = diff.Differences()
			entry.AWSResource = diff.AWSResource()
			if detailed, ok := diff.(resource.DetailedResourceDiff); ok {
				entry.Changes = detailed.Changes()
			}
		case diff == nil:
			entry.Action = PlanNoChange
		default:
			// Non-nil diff with an AWS resource but no messages: a resource that
			// flagged a diff without appending a message. Nothing user-visible
			// differs, so it is reported as no change. The previous PlanTargets
			// matched none of its branches here and printed nothing for the
			// resource; reporting it is the one deliberate output difference.
			entry.Action = PlanNoChange
		}
		visit(entry)
	}
	return nil
}

// planRenderer writes entries to the logger in the format buildit has always
// used: green for create, yellow for update, blue for no change. Skipped
// resources are listed only at DEBUG; below that finish reports their count.
type planRenderer struct {
	skipped int
}

func (r *planRenderer) entry(e PlanEntry) {
	switch e.Action {
	case PlanSkipped:
		r.skipped++
		log.WithFields(log.Fields{
			color.Yellow("Resource"): e.Key,
			color.Yellow("Type"):     e.Type,
		}).Debug(color.Yellow("skipping resource not targetted"))
	case PlanUnsupported:
		log.WithFields(log.Fields{"Resource": e.Key}).Error("Plan not supported for this resource type")
	case PlanCreate:
		log.WithFields(
			log.Fields{color.Green("Resource"): e.Key, color.Green("Type"): e.Type},
		).Info(color.Green("new resource will be provisioned"))
	case PlanUpdate:
		log.WithFields(
			log.Fields{color.Yellow("Resource"): e.Key, color.Yellow("Type"): e.Type},
		).Info(color.Yellow("resource will be updated"))
		log.Infof("-----------------------------------")
		log.Infof("\t%v", e.Key)
		log.Infof("-----------------------------------")
		for i, str := range e.Differences {
			log.Infof("\t\t|->Diff %v: %v", i, color.Yellow(str))
		}
		log.Info(color.Yellow(""))
	case PlanNoChange:
		log.WithFields(log.Fields{color.Blue("Resource"): e.Key, color.Blue("Type"): e.Type}).Info(color.Blue("resource exists, no diff found"))
	}
}

// finish emits the end-of-plan summary.
func (r *planRenderer) finish() {
	// if files skipped & not logged due to logger level, inform the user
	if r.skipped > 0 && log.GetLevel() < log.DebugLevel {
		log.Warnf("%v resources were skipped, use log level DEBUG or higher to view list", r.skipped)
	}
}
