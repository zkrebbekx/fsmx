# CLAUDE.md

Guidance for AI assistants and contributors working in this repository.

## What this is

`fsmx` is a small, type-safe finite state machine for Go. States, events and the
owning model are user-supplied type parameters (`Machine[S, E, M]`), so the API is
checked at compile time, the transition table is validated once at `Build`, and
the machine can render itself as a Mermaid `stateDiagram-v2`. It has **zero
runtime dependencies** — keep it that way.

## Architecture

| File         | Responsibility                                                       |
| ------------ | -------------------------------------------------------------------- |
| `fsm.go`     | `Machine`, `New`, `Fire`/`Can`/`Available`/`Init`, errors, package doc.|
| `builder.go` | `Builder`: `Transition`/`Guard`/`OnEnter`/`OnExit`/`OnTransition`/`Build`.|
| `field.go`   | `Field[S]`, an embeddable state holder for models without a column.  |
| `mermaid.go` | `Mermaid` / `MermaidFor`: render the machine as a state diagram.     |

The machine is immutable after `Build` and safe to share across goroutines; the
only mutation is to the model passed to `Fire` (the caller's to synchronise).
State access goes through the `get`/`set` accessors, never reflection, so the
state can live in a DB column, a struct field, or an embedded `Field`.

## Coding standards

Follow the Go standard library style. Document every exported identifier with a
doc comment beginning with its name (`revive` enforces it). Comment the *why*,
not the obvious.

- Errors wrap the sentinels `ErrNoTransition` / `ErrGuard` with `%w`.
- Programmer mistakes (bad transition table, nil callbacks) are surfaced by
  `Build`, not by `Fire`. `Fire` only returns runtime outcomes.
- Keep the surface small: one obvious way to declare a machine and to fire it.

## Testing

- BDD `Convey` Given/When/Then ([goconvey](https://github.com/smartystreets/goconvey))
  — the required style for all tests.
- Runnable `Example` functions double as documentation.
- New behaviour needs tests; don't let coverage regress.

## Commits

[Conventional Commits](https://www.conventionalcommits.org). Subject imperative,
≤ ~72 chars; body explains *why* when non-obvious.
