// Command order is the concepts guide as a runnable program: it builds the order
// machine, prints its diagram, then walks one order through its lifecycle,
// showing transitions, a blocked guard, and the available events at each step.
//
//	go run ./examples/order
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/zkrebbekx/fsmx"
)

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

// Order owns its state via the embedded Field, so NewFor needs no accessors.
type Order struct {
	fsmx.Field[OrderState]
	PaymentCaptured bool
}

func main() {
	m, err := fsmx.NewFor[OrderState, OrderEvent, *Order](Draft).
		Transition(Draft, Pay, Paid).
		Transition(Paid, Ship, Shipped).
		Transition(Draft, Cancel, Cancelled).
		Transition(Paid, Cancel, Cancelled).
		Guard(Paid, Ship, func(_ context.Context, o *Order) error {
			if !o.PaymentCaptured {
				return errors.New("payment not captured")
			}
			return nil
		}).
		OnEnter(Shipped, func(_ context.Context, _ *Order) error {
			fmt.Println("    (side effect: shipment email sent)")
			return nil
		}).
		Build()
	if err != nil {
		panic(err)
	}

	fmt.Println("== machine ==")
	fmt.Println(m.Mermaid())

	ctx := context.Background()
	o := &Order{}
	m.Init(o)
	fmt.Printf("new order: %s, available: %v\n", o.GetState(), m.Available(o))

	fire(ctx, m, o, Pay) // Draft -> Paid

	// Ship is blocked: payment is not captured yet.
	fire(ctx, m, o, Ship)

	// Capture payment, then ship succeeds (and fires the OnEnter effect).
	o.PaymentCaptured = true
	fire(ctx, m, o, Ship) // Paid -> Shipped

	// Shipped is terminal: nothing is available, and Cancel is undefined.
	fmt.Printf("final: %s, available: %v\n", o.GetState(), m.Available(o))
	fire(ctx, m, o, Cancel)
}

// fire applies an event and prints the outcome, distinguishing a guard rejection
// from an undefined transition from success.
func fire(ctx context.Context, m *fsmx.Machine[OrderState, OrderEvent, *Order], o *Order, ev OrderEvent) {
	switch err := m.Fire(ctx, o, ev); {
	case err == nil:
		fmt.Printf("fire %-6q -> %s\n", ev, o.GetState())
	case errors.Is(err, fsmx.ErrGuard):
		fmt.Printf("fire %-6q blocked by guard: %v\n", ev, err)
	case errors.Is(err, fsmx.ErrNoTransition):
		fmt.Printf("fire %-6q not allowed from %s\n", ev, o.GetState())
	default:
		fmt.Printf("fire %-6q error: %v\n", ev, err)
	}
}
