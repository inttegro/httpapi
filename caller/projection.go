package caller

import (
	"errors"
	"fmt"
)

var (
	// ErrProjectionCallerUndefined reports that a projection cannot be resolved
	// because its granted caller is undefined.
	ErrProjectionCallerUndefined = errors.New("httpapi/caller: projection caller is undefined")

	// ErrProjectionElevation reports that a requested projection would widen
	// visibility beyond the granted caller.
	ErrProjectionElevation = errors.New("httpapi/caller: projection elevation is not allowed")
)

// ProjectionRule declares that responses visible to From may be projected as
// To. Rules are directional: the reverse projection is not implied.
type ProjectionRule struct {
	from Caller
	to   Caller
}

// Descends declares that from may use the narrower response projection to.
//
// Callers form a graph rather than a numeric privilege ladder. This lets an
// application define independent branches without accidentally allowing
// lateral projections between them.
func Descends(from, to Caller) ProjectionRule {
	from = requireDefined(from)
	to = requireDefined(to)
	if from == to {
		panic(fmt.Sprintf("httpapi/caller: projection rule %q -> %q must connect distinct callers", from, to))
	}
	return ProjectionRule{from: from, to: to}
}

// ProjectionPolicy defines the response projections allowed for callers.
//
// The zero value allows identity projections only. A configured policy also
// allows transitive descent through its declared rules. It never allows upward
// or lateral projection.
type ProjectionPolicy struct {
	descendants map[Caller]map[Caller]struct{}
}

type projectionPolicyCtor struct {
	rules  []ProjectionRule
	edges  map[Caller]map[Caller]struct{}
	policy ProjectionPolicy
	err    error
}

type projectionResolver struct {
	policy    ProjectionPolicy
	granted   Caller
	requested Caller
	resolved  Caller
	err       error
}

// DefineProjectionPolicy builds an immutable projection policy from rules.
// Invalid, duplicate, and cyclic rules panic so configuration errors fail at
// application startup.
func DefineProjectionPolicy(rules ...ProjectionRule) ProjectionPolicy {
	ctor := projectionPolicyCtor{rules: rules}
	ctor.initialize()
	ctor.indexRules()
	ctor.ensureAcyclic()
	ctor.indexDescendants()
	return ctor.result()
}

func (ctor *projectionPolicyCtor) initialize() {
	if ctor.err != nil {
		return
	}
	ctor.edges = make(map[Caller]map[Caller]struct{}, len(ctor.rules))
	ctor.policy.descendants = make(map[Caller]map[Caller]struct{}, len(ctor.rules))
}

func (ctor *projectionPolicyCtor) indexRules() {
	if ctor.err != nil {
		return
	}
	for _, rule := range ctor.rules {
		if !rule.from.Defined() || !rule.to.Defined() {
			ctor.err = errors.New("httpapi/caller: projection rule contains an undefined caller")
			return
		}
		if rule.from == rule.to {
			ctor.err = fmt.Errorf(
				"httpapi/caller: projection rule %q -> %q must connect distinct callers",
				rule.from,
				rule.to,
			)
			return
		}
		ctor.indexRule(rule)
		if ctor.err != nil {
			return
		}
	}
}

func (ctor *projectionPolicyCtor) indexRule(rule ProjectionRule) {
	if ctor.err != nil {
		return
	}
	if ctor.edges[rule.from] == nil {
		ctor.edges[rule.from] = make(map[Caller]struct{})
	}
	if _, exists := ctor.edges[rule.from][rule.to]; exists {
		ctor.err = fmt.Errorf(
			"httpapi/caller: duplicate projection rule %q -> %q",
			rule.from,
			rule.to,
		)
		return
	}
	ctor.edges[rule.from][rule.to] = struct{}{}
}

func (ctor *projectionPolicyCtor) ensureAcyclic() {
	if ctor.err != nil {
		return
	}
	cycle := findProjectionCycle(ctor.edges)
	if cycle.Defined() {
		ctor.err = fmt.Errorf("httpapi/caller: projection rules contain a cycle at %q", cycle)
	}
}

func (ctor *projectionPolicyCtor) indexDescendants() {
	if ctor.err != nil {
		return
	}
	for from := range ctor.edges {
		reachable := make(map[Caller]struct{})
		collectDescendants(from, ctor.edges, reachable)
		ctor.policy.descendants[from] = reachable
	}
}

func (ctor *projectionPolicyCtor) result() ProjectionPolicy {
	if ctor.err != nil {
		panic(ctor.err.Error())
	}
	return ctor.policy
}

// Allows reports whether granted may be projected as requested.
//
// An undefined requested caller means no projection was requested and resolves
// to granted. An undefined granted caller is always denied.
func (policy ProjectionPolicy) Allows(granted, requested Caller) bool {
	_, err := policy.Resolve(granted, requested)
	return err == nil
}

// Resolve returns the effective response caller for a requested projection.
//
// When requested is undefined, Resolve retains granted. Identity and declared
// transitive descent are allowed. Upward and lateral projections return
// ErrProjectionElevation.
func (policy ProjectionPolicy) Resolve(granted, requested Caller) (Caller, error) {
	resolver := projectionResolver{
		policy:    policy,
		granted:   granted,
		requested: requested,
	}
	resolver.ensureGrantedCaller()
	resolver.defaultRequestedCaller()
	resolver.ensureProjectionAllowed()
	return resolver.resolved, resolver.err
}

func (resolver *projectionResolver) ensureGrantedCaller() {
	if resolver.err != nil {
		return
	}
	if !resolver.granted.Defined() {
		resolver.err = ErrProjectionCallerUndefined
	}
}

func (resolver *projectionResolver) defaultRequestedCaller() {
	if resolver.err != nil {
		return
	}
	if !resolver.requested.Defined() {
		resolver.requested = resolver.granted
	}
}

func (resolver *projectionResolver) ensureProjectionAllowed() {
	if resolver.err != nil {
		return
	}
	if resolver.requested == resolver.granted {
		resolver.resolved = resolver.granted
		return
	}
	if _, ok := resolver.policy.descendants[resolver.granted][resolver.requested]; ok {
		resolver.resolved = resolver.requested
		return
	}
	resolver.err = fmt.Errorf(
		"%w: caller %q cannot project as %q",
		ErrProjectionElevation,
		resolver.granted,
		resolver.requested,
	)
}

func collectDescendants(current Caller, edges map[Caller]map[Caller]struct{}, reachable map[Caller]struct{}) {
	for next := range edges[current] {
		if _, seen := reachable[next]; seen {
			continue
		}
		reachable[next] = struct{}{}
		collectDescendants(next, edges, reachable)
	}
}

type projectionCycleFinder struct {
	edges map[Caller]map[Caller]struct{}
	state map[Caller]uint8
	cycle Caller
}

const (
	projectionUnvisited uint8 = iota
	projectionVisiting
	projectionVisited
)

func findProjectionCycle(edges map[Caller]map[Caller]struct{}) Caller {
	finder := projectionCycleFinder{
		edges: edges,
		state: make(map[Caller]uint8, len(edges)),
	}
	finder.visitRoots()
	return finder.cycle
}

func (finder *projectionCycleFinder) visitRoots() {
	for caller := range finder.edges {
		if finder.cycle.Defined() {
			return
		}
		if finder.state[caller] == projectionUnvisited {
			finder.visit(caller)
		}
	}
}

func (finder *projectionCycleFinder) visit(current Caller) {
	if finder.cycle.Defined() {
		return
	}
	finder.state[current] = projectionVisiting
	for next := range finder.edges[current] {
		switch finder.state[next] {
		case projectionVisiting:
			finder.cycle = next
			return
		case projectionUnvisited:
			finder.visit(next)
			if finder.cycle.Defined() {
				return
			}
		}
	}
	finder.state[current] = projectionVisited
}
