// Package fsmx is a small, type-safe finite state machine for Go.
//
// # The mental model
//
// A machine answers one question: "given a thing in some state, which event
// moves it to which next state, and is that allowed right now?" fsmx makes the
// three nouns your own types:
//
//   - State (S): where a thing can be — e.g. Draft, Paid, Shipped.
//   - Event (E): what happens to it — e.g. Pay, Ship, Cancel.
//   - Model (M): the thing itself — your *Order, *Door, *Subscription. It holds
//     the current state; the machine reads and writes it.
//
// You declare the legal (state, event) -> state transitions once. Then Fire takes
// a model and an event, checks the transition is allowed (optionally via guards),
// runs your callbacks, and advances the model's state. Because S and E are your
// own comparable types — usually string or int constants — a wrong event is a
// compile error, not a runtime surprise, and the whole table is validated once at
// Build.
//
// # The smallest complete example
//
//	type Light string
//	const (Green, Yellow, Red Light = "green", "yellow", "red")
//
//	type Tick string
//	const (Next Tick = "next")
//
//	// A model can hold its state via the embeddable Field, so it satisfies
//	// Stateful and needs no accessors.
//	type Signal struct{ fsmx.Field[Light] }
//
//	m, _ := fsmx.NewFor[Light, Tick, *Signal](Green).
//		Transition(Green, Next, Yellow).
//		Transition(Yellow, Next, Red).
//		Transition(Red, Next, Green).
//		Build()
//
//	s := &Signal{}
//	m.Init(s)                  // start in Green
//	m.Fire(context.TODO(), s, Next) // -> Yellow
//
// # Two ways to connect state to your model
//
// fsmx never reflects over your model; it reads and writes state through two
// functions. You can supply them in either of two ways:
//
//   - NewFor, when the model implements Stateful[S] (GetState/SetState). Embed
//     Field[S] to get those for free, or write the two methods yourself.
//   - New, when you pass the get and set funcs explicitly. Use this to map state
//     onto an existing database column or struct field without adding methods —
//     e.g. a `Status string` column converted to and from your State type.
//
// # Guards and callbacks
//
// A Guard is a precondition: it decides whether a declared transition may proceed
// (return an error to block it). OnEnter, OnExit and OnTransition are side
// effects that run around a successful transition. All of them receive the model,
// so your rules and effects live next to your data. See Fire for the exact order
// and the atomicity guarantee.
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
	observers   []Observer[S, E, M]

	ordered []transition[S, E] // build order, for a deterministic diagram
	states  []S                // every state, declaration order, deduplicated
	events  []E                // every event, declaration order, deduplicated
}

// Observer runs after every successful transition, receiving the model and the
// from/event/to of the move. It is the place for cross-cutting concerns —
// audit logging, metrics, or persisting the model — that apply to all
// transitions rather than one. An error rolls the transition back, exactly like a
// callback.
type Observer[S, E comparable, M any] func(ctx context.Context, model M, from S, ev E, to S) error

type edge[S, E comparable] struct {
	from S
	ev   E
}

type transition[S, E comparable] struct {
	from, to S
	ev       E
}

// Stateful is implemented by a model that stores its own state. Embedding
// Field[S] satisfies it; NewFor uses it so you can build a machine without
// writing accessor functions.
type Stateful[S comparable] interface {
	GetState() S
	SetState(S)
}

// NewFor begins building a machine for a model that implements Stateful[S] — the
// common case. It is New with the accessors wired to the model's GetState and
// SetState, so there are no closures to write:
//
//	type Order struct{ fsmx.Field[OrderState] }
//	m, err := fsmx.NewFor[OrderState, OrderEvent, *Order](Draft).
//		Transition(Draft, Pay, Paid).Build()
//
// When state lives in an existing column or field instead (no methods to add),
// use New with explicit get/set functions.
func NewFor[S, E comparable, M Stateful[S]](initial S) *Builder[S, E, M] {
	return New[S, E, M](initial,
		func(m M) S { return m.GetState() },
		func(m M, s S) { m.SetState(s) },
	)
}

// New begins building a machine. initial is the start state (the target of the
// diagram's entry arrow). get and set read and write the state on a model — point
// them at a database column, a struct field, or an embedded Field. For a model
// that implements Stateful[S], prefer NewFor, which supplies the accessors for
// you.
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
	for _, ob := range m.observers {
		if err := ob(ctx, model, from, ev, to); err != nil {
			m.set(model, from)
			return err
		}
	}
	return nil
}

// States returns every state mentioned by a transition, in declaration order.
func (m *Machine[S, E, M]) States() []S { return append([]S(nil), m.states...) }

// Events returns every event mentioned by a transition, in declaration order.
func (m *Machine[S, E, M]) Events() []E { return append([]E(nil), m.events...) }

// IsFinal reports whether s has no outgoing transitions — a terminal state from
// which the machine cannot move.
func (m *Machine[S, E, M]) IsFinal(s S) bool {
	for _, t := range m.ordered {
		if t.from == s {
			return false
		}
	}
	return true
}

// Unreachable returns the states that cannot be reached from the initial state by
// any sequence of transitions (the initial state itself is always reachable). An
// empty result means every declared state is reachable — useful as a build-time
// sanity check in a test.
func (m *Machine[S, E, M]) Unreachable() []S {
	seen := map[S]bool{m.initial: true}
	for grew := true; grew; {
		grew = false
		for _, t := range m.ordered {
			if seen[t.from] && !seen[t.to] {
				seen[t.to] = true
				grew = true
			}
		}
	}
	var out []S
	for _, s := range m.states {
		if !seen[s] {
			out = append(out, s)
		}
	}
	return out
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
