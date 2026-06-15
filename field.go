package fsmx

// Field is an embeddable holder for a model's state, for when the state does not
// already live in a database column. Embed it (by value) and the model satisfies
// Stateful[S] — GetState and SetState come for free — so it works directly with
// NewFor, no accessor functions needed:
//
//	type Door struct {
//		fsmx.Field[DoorState]
//		// ... other fields
//	}
//
//	m, _ := fsmx.NewFor[DoorState, DoorEvent, *Door](Closed).
//		Transition(Closed, Open, Opened).
//		Build()
//
// Use Machine.Init to set a fresh model to the initial state.
type Field[S comparable] struct {
	state S
}

// GetState returns the stored state.
func (f *Field[S]) GetState() S { return f.state }

// SetState stores s.
func (f *Field[S]) SetState(s S) { f.state = s }
