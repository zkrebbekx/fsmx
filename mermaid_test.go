package fsmx

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMermaid(t *testing.T) {
	Convey("Given the order machine", t, func() {
		m := orderMachine()

		Convey("When rendered as Mermaid", func() {
			out := m.Mermaid()
			Convey("Then it is a stateDiagram-v2 with an entry arrow and labelled edges", func() {
				So(out, ShouldEqual, `stateDiagram-v2
  [*] --> draft
  draft --> paid: pay
  paid --> shipped: ship
  draft --> cancelled: cancel
  paid --> cancelled: cancel
`)
			})
		})

		Convey("When rendered for a specific order", func() {
			o := &order{Status: "paid"}
			out := m.MermaidFor(o)
			Convey("Then the current state is highlighted", func() {
				So(out, ShouldContainSubstring, "class paid current")
				So(out, ShouldContainSubstring, "classDef current")
			})
		})
	})
}
