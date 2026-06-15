// Package fsmx is a small, type-safe finite state machine for Go. States and
// events are your own comparable types (typically string or int constants), so a
// typo cannot compile and a transition table is checked once at build time. The
// entity that owns the state — your domain model — is threaded through every
// guard and callback, and the machine can render itself as a Mermaid state
// diagram.
//
// A machine is immutable after Build and safe for concurrent use across
// goroutines; the only mutation is to the model you pass to Fire, which is the
// caller's to synchronise.
package fsmx

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoTransition is returned by Fire when no transition is defined from the
// model's current state for the given event.
var ErrNoTransition = errors.New("fsmx: no transition")

// ErrGuard wraps a guard's rejection: Fire returns it (with the guard's own
// error) when a guard blocks an otherwise-valid transition.
var ErrGuard = errors.New("fsmx: guard rejected")

// Callback runs against the model during a transition. Returning an error aborts
// the transition with the state unchanged (see Fire for exactly when).
type Callback[M any] func(ctx context.Context, model M) error

// Machine is a built, immutable state machine over state type S, event type E and
// model type M. Construct one with New, configure it on the returned Builder, and
// freeze it with Build.
type Machine[S, E comparable, M any] struct {
	initial S
	get     func(M) S
	set     func(M, S)

	transitions map[edge[S, E]]S
	guards      map[edge[S, E]][]Callback[M]
	onEnter     map[S][]Callback[M]
	onExit      map[S][]Callback[M]
	onTrans     map[edge[S, E]][]Callback[M]

	ordered []transition[S, E] // build order, for a deterministic diagram
}

type edge[S, E comparable] struct {
	from S
	ev   E
}

type transition[S, E comparable] struct {
	from, to S
	ev       E
}

// New begins building a machine. initial is the start state (the target of the
// diagram's entry arrow). get and set read and write the state on a model — point
// them at a database column, a struct field, or an embedded Field.
//
//	m, err := fsmx.New[OrderState, OrderEvent, *Order](Draft,
//		func(o *Order) OrderState    { return OrderState(o.Status) },
//		func(o *Order, s OrderState) { o.Status = string(s) },
//	).Transition(Draft, Pay, Paid).Build()
func New[S, E comparable, M any](initial S, get func(M) S, set func(M, S)) *Builder[S, E, M] {
	return &Builder[S, E, M]{m: &Machine[S, E, M]{
		initial:     initial,
		get:         get,
		set:         set,
		transitions: map[edge[S, E]]S{},
		guards:      map[edge[S, E]][]Callback[M]{},
		onEnter:     map[S][]Callback[M]{},
		onExit:      map[S][]Callback[M]{},
		onTrans:     map[edge[S, E]][]Callback[M]{},
	}}
}

// Init sets a model to the machine's initial state. Use it when creating a new
// entity so its first Fire has a state to transition from.
func (m *Machine[S, E, M]) Init(model M) { m.set(model, m.initial) }

// Fire applies event ev to model. It looks up the transition from the model's
// current state; if none exists it returns ErrNoTransition. Each guard for the
// transition runs first — a guard error aborts with the state unchanged, wrapped
// in ErrGuard. Then the exit callbacks of the current state run (an error here
// also leaves the state unchanged), the state is advanced, and the enter and
// transition callbacks of the target run. If one of those fails the state is
// rolled back to where it started and the error returned, so a failed Fire never
// leaves a half-applied transition.
func (m *Machine[S, E, M]) Fire(ctx context.Context, model M, ev E) error {
	from := m.get(model)
	key := edge[S, E]{from, ev}
	to, ok := m.transitions[key]
	if !ok {
		return fmt.Errorf("%w from %v on %v", ErrNoTransition, from, ev)
	}
	for _, g := range m.guards[key] {
		if err := g(ctx, model); err != nil {
			return fmt.Errorf("%w on %v: %w", ErrGuard, ev, err)
		}
	}
	for _, fn := range m.onExit[from] {
		if err := fn(ctx, model); err != nil {
			return err // before the state changes: nothing to roll back
		}
	}
	m.set(model, to)
	for _, fn := range m.onEnter[to] {
		if err := fn(ctx, model); err != nil {
			m.set(model, from)
			return err
		}
	}
	for _, fn := range m.onTrans[key] {
		if err := fn(ctx, model); err != nil {
			m.set(model, from)
			return err
		}
	}
	return nil
}

// Can reports whether a transition exists from the model's current state for ev.
// It is structural and does not run guards (which need a context and may have
// side effects); use Fire and inspect the error to know whether a guarded
// transition would actually proceed.
func (m *Machine[S, E, M]) Can(model M, ev E) bool {
	_, ok := m.transitions[edge[S, E]{m.get(model), ev}]
	return ok
}

// Available returns the events that have a transition from the model's current
// state, in the order they were declared.
func (m *Machine[S, E, M]) Available(model M) []E {
	from := m.get(model)
	var out []E
	for _, t := range m.ordered {
		if t.from == from {
			out = append(out, t.ev)
		}
	}
	return out
}
