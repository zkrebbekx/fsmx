package fsmx_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/zkrebbekx/fsmx"
)

// The running example: an Order moves Draft -> Paid -> Shipped, and can be
// Cancelled from Draft or Paid.
//
//	State  = where the order is.
//	Event  = what happens to it.
//	Model  = *Order, the thing that carries the state.

type OrderState string

const (
	Draft     OrderState = "draft"
	Paid      OrderState = "paid"
	Shipped   OrderState = "shipped"
	Cancelled OrderState = "cancelled"
)

type OrderEvent string

const (
	Pay    OrderEvent = "pay"
	Ship   OrderEvent = "ship"
	Cancel OrderEvent = "cancel"
)

// Order embeds Field, so it stores its own state and satisfies Stateful — that is
// what lets us use NewFor below with no accessor functions.
type Order struct {
	fsmx.Field[OrderState]
	PaymentCaptured bool
}

// orderMachine wires the transitions once. Real code builds this at startup and
// shares the (immutable) machine.
func orderMachine() *fsmx.Machine[OrderState, OrderEvent, *Order] {
	m, err := fsmx.NewFor[OrderState, OrderEvent, *Order](Draft).
		// A transition: event E moves the model from one state to another.
		Transition(Draft, Pay, Paid).
		Transition(Paid, Ship, Shipped).
		Transition(Draft, Cancel, Cancelled).
		Transition(Paid, Cancel, Cancelled).
		// A guard: a precondition that decides whether Ship may proceed.
		Guard(Paid, Ship, func(_ context.Context, o *Order) error {
			if !o.PaymentCaptured {
				return errors.New("payment not captured")
			}
			return nil
		}).
		Build()
	if err != nil {
		panic(err) // a build error is a programming mistake
	}
	return m
}

// Concept: firing an event advances the model's state.
func Example_fire() {
	m := orderMachine()
	o := &Order{}
	m.Init(o) // a fresh order starts in the initial state, Draft

	_ = m.Fire(context.Background(), o, Pay)
	fmt.Println("after Pay:", o.GetState())
	// Output:
	// after Pay: paid
}

// Concept: an undefined transition is reported, not performed.
func Example_noTransition() {
	m := orderMachine()
	o := &Order{}
	m.Init(o)

	err := m.Fire(context.Background(), o, Ship) // can't ship a Draft
	fmt.Println("err is ErrNoTransition:", errors.Is(err, fsmx.ErrNoTransition))
	fmt.Println("state unchanged:", o.GetState())
	// Output:
	// err is ErrNoTransition: true
	// state unchanged: draft
}

// Concept: a guard blocks a transition that is structurally legal but not allowed
// right now. The state does not change when a guard rejects.
func Example_guard() {
	m := orderMachine()
	o := &Order{}
	o.SetState(Paid) // an order that is paid in the workflow...
	// ...but payment was not actually captured yet.

	err := m.Fire(context.Background(), o, Ship)
	fmt.Println("blocked:", errors.Is(err, fsmx.ErrGuard), "-", err)
	fmt.Println("state:", o.GetState())

	o.PaymentCaptured = true // now the guard passes
	_ = m.Fire(context.Background(), o, Ship)
	fmt.Println("state:", o.GetState())
	// Output:
	// blocked: true - fsmx: guard rejected on ship: payment not captured
	// state: paid
	// state: shipped
}

// Concept: Can asks "is this transition defined from here?" and Available lists
// the events you could fire from the current state.
func Example_canAndAvailable() {
	m := orderMachine()
	o := &Order{}
	m.Init(o)

	fmt.Println("can pay:", m.Can(o, Pay))
	fmt.Println("can ship:", m.Can(o, Ship))
	fmt.Println("available:", m.Available(o))
	// Output:
	// can pay: true
	// can ship: false
	// available: [pay cancel]
}

// Concept: the machine documents itself as a Mermaid state diagram.
func Example_mermaid() {
	fmt.Print(orderMachine().Mermaid())
	// Output:
	// stateDiagram-v2
	//   [*] --> draft
	//   draft --> paid: pay
	//   paid --> shipped: ship
	//   draft --> cancelled: cancel
	//   paid --> cancelled: cancel
}

// Concept: when state already lives in a database column, use New with explicit
// accessors instead of embedding Field — no methods added to your row type.
func ExampleNew_databaseColumn() {
	type Row struct {
		Status string // the persisted column
	}
	m, _ := fsmx.New[OrderState, OrderEvent, *Row](Draft,
		func(r *Row) OrderState { return OrderState(r.Status) }, // read column
		func(r *Row, s OrderState) { r.Status = string(s) },     // write column
	).
		Transition(Draft, Pay, Paid).
		Build()

	r := &Row{Status: "draft"}
	_ = m.Fire(context.Background(), r, Pay)
	fmt.Println(r.Status)
	// Output:
	// paid
}
