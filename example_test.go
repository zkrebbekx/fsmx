package fsmx_test

import (
	"context"
	"fmt"

	"github.com/zkrebbekx/fsmx"
)

type DoorState string

const (
	Closed DoorState = "closed"
	Open   DoorState = "open"
	Locked DoorState = "locked"
)

type DoorEvent string

const (
	OpenIt   DoorEvent = "open"
	CloseIt  DoorEvent = "close"
	LockIt   DoorEvent = "lock"
	UnlockIt DoorEvent = "unlock"
)

// Door stores its state in an embedded Field, so no accessors are needed.
type Door struct {
	fsmx.Field[DoorState]
}

func ExampleMachine_Fire() {
	m, _ := fsmx.New[DoorState, DoorEvent, *Door](Closed,
		(*Door).GetState, (*Door).SetState).
		Transition(Closed, OpenIt, Open).
		Transition(Open, CloseIt, Closed).
		Transition(Closed, LockIt, Locked).
		Transition(Locked, UnlockIt, Closed).
		Build()

	d := &Door{}
	m.Init(d)

	_ = m.Fire(context.Background(), d, OpenIt)
	fmt.Println(d.GetState())

	fresh := &Door{}
	m.Init(fresh) // a new door starts closed
	fmt.Println(m.Available(fresh))
	// Output:
	// open
	// [open lock]
}

func ExampleMachine_Mermaid() {
	m, _ := fsmx.New[DoorState, DoorEvent, *Door](Closed,
		(*Door).GetState, (*Door).SetState).
		Transition(Closed, OpenIt, Open).
		Transition(Closed, LockIt, Locked).
		Build()

	fmt.Print(m.Mermaid())
	// Output:
	// stateDiagram-v2
	//   [*] --> closed
	//   closed --> open: open
	//   closed --> locked: lock
}
