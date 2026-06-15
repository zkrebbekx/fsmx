package fsmx

import (
	"fmt"
	"strings"
)

// Mermaid renders the machine as a Mermaid stateDiagram-v2: an entry arrow to the
// initial state, then one arrow per transition labelled with its event, in
// declaration order. Paste the result into any Mermaid renderer, a Markdown file,
// or feed it to github.com/zkrebbekx/go-mermaid for richer rendering.
//
// State and event values are printed with %v, so use simple identifier-like
// constants (string or int) for a clean diagram.
func (m *Machine[S, E, M]) Mermaid() string {
	return m.mermaid(nil)
}

// MermaidFor is Mermaid with the model's current state highlighted, for a live
// "where is this entity now" diagram.
func (m *Machine[S, E, M]) MermaidFor(model M) string {
	cur := m.get(model)
	return m.mermaid(&cur)
}

func (m *Machine[S, E, M]) mermaid(highlight *S) string {
	var b strings.Builder
	b.WriteString("stateDiagram-v2\n")
	fmt.Fprintf(&b, "  [*] --> %v\n", m.initial)
	for _, t := range m.ordered {
		fmt.Fprintf(&b, "  %v --> %v: %v\n", t.from, t.to, t.ev)
	}
	// Terminal states get an exit arrow, so the diagram shows where a workflow ends.
	for _, s := range m.states {
		if m.IsFinal(s) {
			fmt.Fprintf(&b, "  %v --> [*]\n", s)
		}
	}
	if highlight != nil {
		fmt.Fprintf(&b, "  class %v current\n", *highlight)
		b.WriteString("  classDef current fill:#ffd54f,stroke:#f57f17,font-weight:bold\n")
	}
	return b.String()
}
