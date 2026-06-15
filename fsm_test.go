package fsmx

import (
	"context"
	"errors"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type orderState string

const (
	draft     orderState = "draft"
	paid      orderState = "paid"
	shipped   orderState = "shipped"
	cancelled orderState = "cancelled"
)

type orderEvent string

const (
	pay    orderEvent = "pay"
	ship   orderEvent = "ship"
	cancel orderEvent = "cancel"
)

type order struct {
	Status string
	Paid   bool
}

// orderMachine builds the example machine used across the tests.
func orderMachine() *Machine[orderState, orderEvent, *order] {
	m, err := New[orderState, orderEvent, *order](draft,
		func(o *order) orderState { return orderState(o.Status) },
		func(o *order, s orderState) { o.Status = string(s) },
	).
		Transition(draft, pay, paid).
		Transition(paid, ship, shipped).
		Transition(draft, cancel, cancelled).
		Transition(paid, cancel, cancelled).
		Guard(paid, ship, func(_ context.Context, o *order) error {
			if !o.Paid {
				return fmt.Errorf("payment not captured")
			}
			return nil
		}).
		Build()
	if err != nil {
		panic(err)
	}
	return m
}

func TestFire(t *testing.T) {
	ctx := context.Background()

	Convey("Given an order machine and a draft order", t, func() {
		m := orderMachine()
		o := &order{}
		m.Init(o)

		Convey("When a valid event fires", func() {
			err := m.Fire(ctx, o, pay)
			Convey("Then the model advances to the target state", func() {
				So(err, ShouldBeNil)
				So(o.Status, ShouldEqual, "paid")
			})
		})

		Convey("When an event with no transition fires", func() {
			err := m.Fire(ctx, o, ship) // can't ship a draft
			Convey("Then it returns ErrNoTransition and the state is unchanged", func() {
				So(errors.Is(err, ErrNoTransition), ShouldBeTrue)
				So(o.Status, ShouldEqual, "draft")
			})
		})
	})

	Convey("Given a paid but uncaptured order", t, func() {
		m := orderMachine()
		o := &order{Status: "paid", Paid: false}

		Convey("When ship fires and the guard rejects", func() {
			err := m.Fire(ctx, o, ship)
			Convey("Then it returns ErrGuard with the guard's reason, state unchanged", func() {
				So(errors.Is(err, ErrGuard), ShouldBeTrue)
				So(err.Error(), ShouldContainSubstring, "payment not captured")
				So(o.Status, ShouldEqual, "paid")
			})
		})

		Convey("When payment is captured and ship fires", func() {
			o.Paid = true
			err := m.Fire(ctx, o, ship)
			Convey("Then the guard passes and it ships", func() {
				So(err, ShouldBeNil)
				So(o.Status, ShouldEqual, "shipped")
			})
		})
	})
}

func TestCallbacksAndRollback(t *testing.T) {
	ctx := context.Background()

	Convey("Given a machine whose enter callback fails", t, func() {
		boom := errors.New("enter failed")
		m, _ := New[orderState, orderEvent, *order](draft,
			func(o *order) orderState { return orderState(o.Status) },
			func(o *order, s orderState) { o.Status = string(s) },
		).
			Transition(draft, pay, paid).
			OnEnter(paid, func(_ context.Context, _ *order) error { return boom }).
			Build()

		o := &order{}
		m.Init(o)

		Convey("When the transition fires", func() {
			err := m.Fire(ctx, o, pay)
			Convey("Then the error surfaces and the state rolls back", func() {
				So(errors.Is(err, boom), ShouldBeTrue)
				So(o.Status, ShouldEqual, "draft")
			})
		})
	})

	Convey("Given enter and exit callbacks that succeed", t, func() {
		var trace []string
		m, _ := New[orderState, orderEvent, *order](draft,
			func(o *order) orderState { return orderState(o.Status) },
			func(o *order, s orderState) { o.Status = string(s) },
		).
			Transition(draft, pay, paid).
			OnExit(draft, func(_ context.Context, _ *order) error { trace = append(trace, "exit-draft"); return nil }).
			OnEnter(paid, func(_ context.Context, _ *order) error { trace = append(trace, "enter-paid"); return nil }).
			OnTransition(draft, pay, func(_ context.Context, _ *order) error { trace = append(trace, "trans"); return nil }).
			Build()

		o := &order{}
		m.Init(o)

		Convey("When firing", func() {
			err := m.Fire(ctx, o, pay)
			Convey("Then exit runs before enter before transition hooks", func() {
				So(err, ShouldBeNil)
				So(trace, ShouldResemble, []string{"exit-draft", "enter-paid", "trans"})
			})
		})
	})
}

func TestCanAndAvailable(t *testing.T) {
	Convey("Given a draft order", t, func() {
		m := orderMachine()
		o := &order{}
		m.Init(o)

		Convey("Then Can reports structural transitions", func() {
			So(m.Can(o, pay), ShouldBeTrue)
			So(m.Can(o, ship), ShouldBeFalse)
		})

		Convey("Then Available lists events in declaration order", func() {
			So(m.Available(o), ShouldResemble, []orderEvent{pay, cancel})
		})
	})
}

func TestObserve(t *testing.T) {
	ctx := context.Background()
	get := func(o *order) orderState { return orderState(o.Status) }
	set := func(o *order, s orderState) { o.Status = string(s) }

	Convey("Given a machine with an observer", t, func() {
		var moves []string
		m, _ := New[orderState, orderEvent, *order](draft, get, set).
			Transition(draft, pay, paid).
			Observe(func(_ context.Context, _ *order, from orderState, ev orderEvent, to orderState) error {
				moves = append(moves, fmt.Sprintf("%s-%s->%s", from, ev, to))
				return nil
			}).
			Build()
		o := &order{}
		m.Init(o)

		Convey("When a transition succeeds", func() {
			So(m.Fire(ctx, o, pay), ShouldBeNil)
			Convey("Then the observer sees from/event/to", func() {
				So(moves, ShouldResemble, []string{"draft-pay->paid"})
			})
		})
	})

	Convey("Given an observer that errors", t, func() {
		boom := errors.New("observer failed")
		m, _ := New[orderState, orderEvent, *order](draft, get, set).
			Transition(draft, pay, paid).
			Observe(func(_ context.Context, _ *order, _ orderState, _ orderEvent, _ orderState) error { return boom }).
			Build()
		o := &order{}
		m.Init(o)

		Convey("When firing", func() {
			err := m.Fire(ctx, o, pay)
			Convey("Then it surfaces and the state rolls back", func() {
				So(errors.Is(err, boom), ShouldBeTrue)
				So(o.Status, ShouldEqual, "draft")
			})
		})
	})
}

func TestIntrospection(t *testing.T) {
	Convey("Given the order machine", t, func() {
		m := orderMachine()

		Convey("Then States and Events list everything in declaration order", func() {
			So(m.States(), ShouldResemble, []orderState{draft, paid, shipped, cancelled})
			So(m.Events(), ShouldResemble, []orderEvent{pay, ship, cancel})
		})

		Convey("Then IsFinal marks terminal states", func() {
			So(m.IsFinal(shipped), ShouldBeTrue)
			So(m.IsFinal(cancelled), ShouldBeTrue)
			So(m.IsFinal(draft), ShouldBeFalse)
		})

		Convey("Then every state is reachable from the initial state", func() {
			So(m.Unreachable(), ShouldBeEmpty)
		})
	})

	Convey("Given a machine with an orphaned state", t, func() {
		get := func(o *order) orderState { return orderState(o.Status) }
		set := func(o *order, s orderState) { o.Status = string(s) }
		// shipped -> cancelled is the only way into a branch unreachable from draft.
		m, _ := New[orderState, orderEvent, *order](draft, get, set).
			Transition(draft, pay, paid).
			Transition(shipped, cancel, cancelled). // shipped is never entered
			Build()

		Convey("Then Unreachable reports the orphans", func() {
			So(m.Unreachable(), ShouldContain, shipped)
			So(m.Unreachable(), ShouldContain, cancelled)
		})
	})
}

func TestBuildErrors(t *testing.T) {
	get := func(o *order) orderState { return orderState(o.Status) }
	set := func(o *order, s orderState) { o.Status = string(s) }

	Convey("Given a duplicate transition", t, func() {
		_, err := New[orderState, orderEvent, *order](draft, get, set).
			Transition(draft, pay, paid).
			Transition(draft, pay, cancelled).
			Build()
		Convey("Then Build fails", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "duplicate transition")
		})
	})

	Convey("Given a guard on an undeclared transition", t, func() {
		_, err := New[orderState, orderEvent, *order](draft, get, set).
			Transition(draft, pay, paid).
			Guard(paid, ship, func(_ context.Context, _ *order) error { return nil }).
			Build()
		Convey("Then Build fails", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "undeclared transition")
		})
	})

	Convey("Given nil accessors", t, func() {
		_, err := New[orderState, orderEvent, *order](draft, nil, nil).
			Transition(draft, pay, paid).
			Build()
		Convey("Then Build fails", func() {
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Given no transitions", t, func() {
		_, err := New[orderState, orderEvent, *order](draft, get, set).Build()
		Convey("Then Build fails", func() {
			So(err, ShouldNotBeNil)
		})
	})
}
