package config

import (
	"context"
	"fmt"
	"reflect"

	"github.com/TouchBistro/buildit/resource"
	"github.com/TouchBistro/buildit/util"
	"github.com/TouchBistro/goutils/async"
	"github.com/TouchBistro/goutils/color"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

type Vertex struct {
	resource resource.Resource
	// Slice of resource names
	adjacentVertices []resource.Key
	inDegree         int
}

// Graph represents a directed graph of AWS resources.
// Dependencies are represented by edges.
// It uses an adjacency list implementation.
// A zero value Graph is an empty graph ready to use.
type Graph struct {
	// map of resource names to vertices
	vertices      map[resource.Key]*Vertex
	depdendencies map[resource.Key][]resource.Key // dependencies `x  --> [a,b,c,d]`; here a,b,c,d have an inward edge from x
}

// VertexKeys returns a []string of vertex keys
func (g Graph) Keys() []resource.Key {
	var keys []resource.Key
	for k := range g.vertices {
		keys = append(keys, k)
	}
	return keys
}

// Vertices returns the internal map of vertices
func (g Graph) Vertices() map[resource.Key]*Vertex {
	return g.vertices
}

// Size returns the number of vertices in this graph
func (g *Graph) Size() int {
	return len(g.vertices)
}

// Cycle and visitVertex are used for finding cycles via DFS
// Colour code:
// Black means that there are no cycles that include that vertex
// Grey means the vertex is in the current stack
// White is an unvisited vertex

// Cycle gets a cycle from the graph
func (g *Graph) Cycle() []resource.Key {
	// Return nil if there is no cycle
	if _, err := g.toposort(); err == nil {
		return nil
	}
	// Map the resources names to the DFS colouring
	colours := make(map[resource.Key]string)
	for vertex := range g.vertices {
		colours[vertex] = "white"
	}
	for vertex := range g.vertices {
		if colours[vertex] != "white" {
			continue
		}
		s := stack[resource.Key]{}
		s.Push(vertex)
		colours[vertex] = "grey"
		subproblem := g.visitVertex(vertex, colours, s)
		if len(subproblem) > 0 {
			return subproblem
		}
		colours[vertex] = "black"
	}
	return nil
}

// AddVertex adds the resource as a vertex in the graph along with its
// depedencies.
//
// A dependency in graph terms is all other verticies that have an outward
// edge into this vertex
func (g *Graph) AddVertex(res resource.Resource, deps []resource.Key) {
	// Lazy initialize map when first vertex is added
	// This way zero value graphs can be used
	if g.vertices == nil {
		g.vertices = make(map[resource.Key]*Vertex)
	}

	if g.depdendencies == nil {
		g.depdendencies = make(map[resource.Key][]resource.Key)
	}

	log.WithFields(log.Fields{
		"key":  res.Key(),
		"type": reflect.TypeOf(res).String(),
	}).Debug(color.Magenta("adding vertex"))

	g.vertices[res.Key()] = &Vertex{
		resource: res,
	}
	g.depdendencies[res.Key()] = deps
}

// GetVertex returns the reosurces for the named vertex from the graph
// a non-nil error is returned if not found
func (g *Graph) GetVertex(name resource.Key) (resource.Resource, error) {
	if r, ok := g.vertices[name]; ok {
		return r.resource, nil
	}
	return nil, errors.Errorf("resource %v not found in thie graph", name)
}

// AddEdge adds an edge to the graph between the
// vertices with the given identifiers. The vertices
// must have already been added to the graph using
// AddVertex or AddEdge will return an error.
func (g *Graph) AddEdge(from, to resource.Key) error {

	log.WithFields(log.Fields{
		"from": from,
		"to":   to,
	}).Debug(color.Magenta("adding edge"))

	return g.addEdge(from, to)
}

// internal methods  ///////

// AddEdge adds an edge to the graph between the
// vertices with the given identifiers. The vertices
// must have already been added to the graph using
// AddVertex or AddEdge will return an error.
func (g *Graph) addEdge(from, to resource.Key) error {

	message := fmt.Sprintf("%q depends on %q, but", to, from)

	v, ok := g.vertices[from]
	if !ok {
		return errors.Errorf("%v vertex %q does not exist", message, from)
	}

	w, ok := g.vertices[to]
	if !ok {
		return errors.Errorf("%v vertex %q does not exist", message, to)
	}

	v.adjacentVertices = append(v.adjacentVertices, to)
	w.inDegree++
	return nil
}

// Visit a vertex, mark the colour, and recurse to neighbours
func (g *Graph) visitVertex(element resource.Key, colours map[resource.Key]string, s stack[resource.Key]) []resource.Key {
	// Iterate over neighbours
	for _, neighbour := range g.vertices[element].adjacentVertices {
		switch colours[neighbour] {
		case "grey":
			// If the neighbour is in the stack, a cycle exists
			return s.FindCycle(neighbour)
		case "white":
			// If the neighbour has not been visited, visit and recurse
			s.Push(neighbour)
			colours[neighbour] = "grey"
			visit := g.visitVertex(neighbour, colours, s)
			if len(visit) > 0 {
				// If recursion found a cycle, return it
				return visit
			}
			// Otherwise backtrack and remove vertex from stack
			colours[neighbour] = "white"
			s.Pop()
		}
	}
	return nil
}

// toposort performs a topological sort on the graph
// and returns a list of the sorted vertices.
// If the graph is not a DAG, i.e. there is a cycle in it,
// an error will be returned.
func (g *Graph) toposort() ([]resource.Key, error) {
	sorted := make([]resource.Key, 0, len(g.vertices))
	var leaves []resource.Key
	// Keep track of indegrees that get mutated
	inDegrees := make(map[resource.Key]int)

	// Find "start vertices" which have no incoming edges
	for n, v := range g.vertices {
		if v.inDegree == 0 {
			leaves = append(leaves, n)
		} else {
			inDegrees[n] = v.inDegree
		}
	}

	// Perform toposort using Kahn's algorithm
	for len(leaves) > 0 {
		n := leaves[0]
		leaves = leaves[1:]
		sorted = append(sorted, n)

		v := g.vertices[n]
		for _, w := range v.adjacentVertices {
			inDegree := inDegrees[w]
			inDegree--
			inDegrees[w] = inDegree
			if inDegree == 0 {
				leaves = append(leaves, w)
			}
		}
	}

	if len(sorted) != len(g.vertices) {
		return nil, errors.New("dependency cycle detected in resource graph")
	}
	return sorted, nil
}

// methods for command processing ///////

// Plan calls PlanTargets with targets = nil
func (g *Graph) Plan(ctx context.Context) error {
	return g.PlanTargets(ctx, nil)
}

// PlanTargets calls Compare on all ComaprableResource targets & logs the plan
// based on the diffs found with existing resources
func (g *Graph) PlanTargets(ctx context.Context, targets []resource.Key) error {

	sorted, err := g.toposort()
	if err != nil {
		return errors.Wrap(err, "failed to toposort resource graph")
	}

	skipped := 0
	for _, n := range sorted {
		v := g.vertices[n]
		rtype := reflect.TypeOf(v.resource)
		// only plan if no targets supplied, or the name of the target matches the supplied list...
		if len(targets) > 0 && !util.ContainsComparable(targets, n) {
			skipped++ // count skipped resources
			log.WithFields(log.Fields{
				color.Yellow("Resource"): n,
				color.Yellow("Type"):     rtype.String(),
			}).Debug(color.Yellow("skipping resource not targetted"))
			continue
		}

		// only plan for comparable resource; others not suppported
		if cr, ok := v.resource.(resource.ComparableResource); ok {
			diff, err := cr.Compare(ctx)
			if err != nil {
				return errors.Wrapf(err, "failed comparing resource %s", n)
			}

			if diff != nil && diff.AWSResource() == nil {
				// there's a diff, but no existing resource exists; this means the resource will
				// be provisioned;
				log.WithFields(
					log.Fields{color.Green("Resource"): n, color.Green("Type"): rtype.String()},
				).Info(color.Green("new resource will be provisioned"))
			} else if diff != nil && len(diff.Differences()) > 0 {
				// there's a diff, and there are differences found between this & existing aws
				// resource, display the diffs
				// log.Info(color.Yellow("-+---------+-"))
				log.WithFields(
					log.Fields{color.Yellow("Resource"): n, color.Yellow("Type"): rtype.String()},
				).Info(color.Yellow("resource will be updated"))
				log.Infof("-----------------------------------")
				log.Infof("\t%v", n)
				log.Infof("-----------------------------------")
				for n, str := range diff.Differences() {
					//log.WithFields(log.Fields{color.Yellow("Resource"): n, color.Yellow("Type"): rtype.String()})

					log.Infof("\t\t|->Diff %v: %v", n, color.Yellow(str))
				}
				log.Info(color.Yellow(""))
			} else if diff == nil {
				// no diff, this & existing resource is identical
				log.WithFields(log.Fields{color.Blue("Resource"): n, color.Blue("Type"): rtype.String()}).Info(color.Blue("resource exists, no diff found"))
			}
		} else {
			log.WithFields(log.Fields{"Resource": n}).Error("Plan not supported for this resource type")
			continue
		}
	}

	// if files skipped & not logged due to logger level, inform the user
	if skipped > 0 && log.GetLevel() < log.DebugLevel {
		log.Warnf("%v resources were skipped, use log level DEBUG or higher to view list", skipped)
	}

	return nil
}

// Apply calls ApplyTargets with targets = nil
func (g *Graph) Apply(ctx context.Context) error {
	return g.ApplyTargets(ctx, nil)
}

// ApplyTargets calls Apply on all resources in the graph.
// The graph is first topologically sorted to ensure
// resources are created in the right order.
// If --targets root flag is supplied (non-empty), then only the resources supplied in
// the list are applied.
//
// The provided context can be used to terminate applying
// resources if it becomes done before all resources complete.
// Note that if the process is terminated any completed resources
// will remain applied and will not be rolled back.
func (g *Graph) ApplyTargets(ctx context.Context, targets []resource.Key) error {
	sorted, err := g.toposort()
	if err != nil {
		return errors.Wrap(err, "failed to toposort resource graph")
	}

	var waitableResources []resource.WaitableResource
	skipped := 0
	for _, n := range sorted {
		v := g.vertices[n]
		// only apply if no targets supplied, or the name of the target matches the supplied list...
		if len(targets) > 0 && !util.ContainsComparable(targets, n) {
			log.WithFields(log.Fields{"Resource": n}).Debug("skipping resource not targetted")
			skipped++
			continue
		}
		if err := v.resource.Apply(ctx); err != nil {
			return errors.Wrapf(err, "failed creating resource %s", n)
		}
		// If it's a WaitableResource we need to keep track of it so we can wait on it later
		if wr, ok := v.resource.(resource.WaitableResource); ok {
			waitableResources = append(waitableResources, wr)
		}
	}
	// if files skipped & not logged due to logger level, inform the user
	if skipped > 0 && log.GetLevel() < log.DebugLevel {
		log.Warnf("%v resources were skipped, use log level DEBUG or higher to view list", skipped)
	}

	if len(waitableResources) == 0 {
		return nil
	}

	// DISCUSS(@maintainer): Should we set a global timeout for all waits or should we allow each resource
	// to configure its own timeout? Latter is more flexible but it also means each service is responsible
	// for implementing timeouts.
	var group async.Group[struct{}]
	for _, wr := range waitableResources {
		wr := wr
		group.Queue(func(ctx context.Context) (struct{}, error) {
			return struct{}{}, wr.Wait(ctx)
		})
	}
	if _, err := group.Wait(ctx); err != nil {
		return errors.Wrap(err, "one or more errors occurred while waiting on resources")
	}
	return nil
}

// Destroy calls DestroyTargets with targets = nil
func (g *Graph) Destroy(ctx context.Context) error {
	return g.DestroyTargets(ctx, nil)
}

// DestroyTargets calls Destroy on all resources in the graph.
// If --targets root flag is supplied (non-empty), then only the resources supplied in
// the list are destroyed
// The graph is first topologically sorted in the reverse order to ensure
// resources are destroyed in the right order.
//
// The provided context can be used to terminate destroying
// resources if it becomes done before all resources complete.
// Note that if the process is terminated any completed resources
// will remain destroyed.
func (g *Graph) DestroyTargets(ctx context.Context, targets []resource.Key) error {
	sorted, err := g.toposort()
	if err != nil {
		return errors.Wrap(err, "failed to toposort resource graph")
	}

	// Iterate in reverse order to ensure that dependants are destroyed
	// before dependencies.
	skipped := 0
	for i := len(sorted) - 1; i >= 0; i-- {
		n := sorted[i]
		v := g.vertices[n]
		if len(targets) == 0 || util.ContainsComparable(targets, n) {
			err := v.resource.Destroy(ctx)
			if err != nil {
				return errors.Wrapf(err, "failed destroying resource %s", n)
			}
		} else {
			log.WithFields(log.Fields{"Resource": n}).Debug("skipping resource not targetted")
			skipped++
		}
	}

	if skipped > 0 && log.GetLevel() < log.DebugLevel {
		log.Warnf("%v resources were skipped, use log level DEBUG or higher to view list", skipped)
	}

	return nil
}
