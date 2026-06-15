package fsmx

import "fmt"

// Builder accumulates a machine's transitions and callbacks and freezes them with
// Build. Its methods chain. A configuration error (a duplicate transition, a nil
// callback) is recorded and returned by Build.
type Builder[S, E comparable, M any] struct {
	m   *Machine[S, E, M]
	err error
}

// Transition declares that event ev moves the machine from state from to state
// to. Exactly one target may be declared per (from, ev) pair; a second is a build
// error. Guards decide whether a transition proceeds, not which one.
func (b *Builder[S, E, M]) Transition(from S, ev E, to S) *Builder[S, E, M] {
	key := edge[S, E]{from, ev}
	if _, dup := b.m.transitions[key]; dup {
		b.fail("duplicate transition from %v on %v", from, ev)
		return b
	}
	b.m.transitions[key] = to
	b.m.ordered = append(b.m.ordered, transition[S, E]{from: from, to: to, ev: ev})
	b.m.states = appendUnique(b.m.states, from, to)
	b.m.events = appendUnique(b.m.events, ev)
	return b
}

// Observe registers a callback that runs after every successful transition,
// receiving the from/event/to of the move — for audit logs, metrics, or
// persisting the model. Observers run in the order added, after the per-state and
// per-transition callbacks; an error rolls the transition back.
func (b *Builder[S, E, M]) Observe(fn Observer[S, E, M]) *Builder[S, E, M] {
	if fn == nil {
		b.fail("nil observer")
		return b
	}
	b.m.observers = append(b.m.observers, fn)
	return b
}

// appendUnique appends each value not already present, preserving order.
func appendUnique[T comparable](dst []T, vals ...T) []T {
	for _, v := range vals {
		found := false
		for _, existing := range dst {
			if existing == v {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, v)
		}
	}
	return dst
}

// Guard adds a precondition to the (from, ev) transition. Guards run, in the
// order added, before any state change; a guard returning an error blocks the
// transition. The transition must be declared first.
func (b *Builder[S, E, M]) Guard(from S, ev E, fn Callback[M]) *Builder[S, E, M] {
	key := edge[S, E]{from, ev}
	if _, ok := b.m.transitions[key]; !ok {
		b.fail("guard for undeclared transition from %v on %v", from, ev)
		return b
	}
	if fn == nil {
		b.fail("nil guard from %v on %v", from, ev)
		return b
	}
	b.m.guards[key] = append(b.m.guards[key], fn)
	return b
}

// OnEnter registers callbacks that run when the machine enters state s, after the
// state has advanced. An error rolls the transition back.
func (b *Builder[S, E, M]) OnEnter(s S, fns ...Callback[M]) *Builder[S, E, M] {
	return b.addState(b.m.onEnter, "OnEnter", s, fns)
}

// OnExit registers callbacks that run when the machine leaves state s, before the
// state advances. An error aborts the transition with the state unchanged.
func (b *Builder[S, E, M]) OnExit(s S, fns ...Callback[M]) *Builder[S, E, M] {
	return b.addState(b.m.onExit, "OnExit", s, fns)
}

// OnTransition registers callbacks that run on the specific (from, ev) transition,
// after the state has advanced. An error rolls the transition back. The
// transition must be declared first.
func (b *Builder[S, E, M]) OnTransition(from S, ev E, fns ...Callback[M]) *Builder[S, E, M] {
	key := edge[S, E]{from, ev}
	if _, ok := b.m.transitions[key]; !ok {
		b.fail("OnTransition for undeclared transition from %v on %v", from, ev)
		return b
	}
	for _, fn := range fns {
		if fn == nil {
			b.fail("nil OnTransition callback from %v on %v", from, ev)
			return b
		}
		b.m.onTrans[key] = append(b.m.onTrans[key], fn)
	}
	return b
}

// Build validates the configuration and returns the immutable machine. It errors
// if New was misconfigured (nil get/set) or any builder step failed.
func (b *Builder[S, E, M]) Build() (*Machine[S, E, M], error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.m.get == nil || b.m.set == nil {
		return nil, fmt.Errorf("fsmx: New requires non-nil get and set accessors")
	}
	if len(b.m.transitions) == 0 {
		return nil, fmt.Errorf("fsmx: machine has no transitions")
	}
	return b.m, nil
}

func (b *Builder[S, E, M]) addState(dst map[S][]Callback[M], what string, s S, fns []Callback[M]) *Builder[S, E, M] {
	for _, fn := range fns {
		if fn == nil {
			b.fail("nil %s callback for %v", what, s)
			return b
		}
		dst[s] = append(dst[s], fn)
	}
	return b
}

func (b *Builder[S, E, M]) fail(format string, args ...any) {
	if b.err == nil { // keep the first error
		b.err = fmt.Errorf("fsmx: "+format, args...)
	}
}
