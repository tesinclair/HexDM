# Hexcript Reference

Hexcript is the configuration language for HexDM greeters. A greeter file declares
a tree of elements, attaches appearance to daemon states, and attaches statements
to daemon events.

The greeter is unprivileged. Everything privileged lives behind the daemon API.

---

## 1. Elements

    Type::Subtype<id>(attributes){ children }

`<id>` is optional. Attributes are optional. Children are only legal on containers.

    Widget::Text<title>(font_size=5, text="HexDM")
    Container::VBox(gap=2){ ... }

### Containers

Containers control positioning. They position **their own children** — never
themselves, never their parent.

| Container | Behaviour |
|---|---|
| `Box` | Holds one child at a given alignment. Defaults to `align="center,center"` |
| `VBox` | N children, stacked vertically |
| `HBox` | N children, stacked horizontally |
| `Grid` | MxN children in a grid |
| `Stack` | N children occupying the same box, drawn in declaration order |
| `Pager` | N children, one shown at a time (`active_page`) |
| `Carousel` | N children, cycled through (`n` = visible window) |
| `Path` | N children distributed along the container's own boundary |

### Widgets

Widgets render. They have **zero** say over where they go.

| Widget | Behaviour |
|---|---|
| `Box` | A rectangle with box attributes |
| `Circle` | A circle |
| `Text` | A text run |
| `Texture` | A texture asset |
| `Animation` | An animation asset |
| `Pixel` | A single pixel |

### Rules

- Every widget must be inside a container.
- A container positions its children only.
- A default `Box` container may be omitted:
  `Container::Box{ Widget::Text }` is written `Widget::Text`.

### Sizes

All sizes (`gap`, `font_size`, `offset`, ...) are `sN`, where `s` is the system
default font size and `N` is the attribute value. Equivalent to CSS `rem`.

### Transforms

`offset_x`, `offset_y`, `scale`, `rotate` and `opacity` apply **after** layout.
They move the element within its allotted box and never affect siblings, so the
positioning invariant holds. This is how animation moves things without giving
widgets a say over layout.

### `Container::Path`

`Path` distributes children along its own boundary by **arc length**, so spacing
is even regardless of the boundary's shape.

| Attribute | Meaning |
|---|---|
| `shape` | `"rect"`, `"circle"`, `"arc"` |
| `offset` | Where the run starts, as a fraction of total boundary length (0.0–1.0) |
| `extent` | How much of the boundary the run occupies (0.0–1.0) |

Because both are arc-length fractions, the same script drives a rect ring and a
circular ring identically. Wrap `Path` and the thing being decorated in a `Stack`
to put a border effect around a widget.

---

## 2. Selectors

Both `State::` and `Script::` take the same header:

    (trigger => target, target as alias, ...)

- `trigger` — which element's signal fires this block.
- `targets` — which elements the body touches.

Shorthands:

| Form | Meaning |
|---|---|
| `(x)` | `(x => x)` |
| `(x => _, y)` | `_` includes the trigger in the targets |
| `(=> y)` | All triggers, targets `y` |
| `()` | All triggers, all elements in scope by their own id |

`Root` owns **all** states universally. A block triggered on `Root` fires whenever
any form of that state fires anywhere.

Selectors match on id and on type.

### The `:` chain

One directive, several unrelated bodies:

    State::AuthSuccess(password => pw_ring){ opacity=1.0 }
      : (password){ border="#00FF00" }

Comma-separated targets share one body. The `:` chain is for bodies that differ.

**Idiom:** one trigger, one scope, one body per clause. If two pairs don't
intersect, chain them rather than merging their headers — the scope list is
what the parser checks the body against, so keeping it minimal is the point.

### No tree descent

States do not propagate down the tree. A child that should react to its parent's
state must be named explicitly. There are no descendant, child, or all-children
combinators.

---

## 3. `Auth::`

    Auth::Fprint(fprint)
    Auth::Password(password)

Registers which widget owns a method's states. When that method fails, its
`AuthFail` fires on that widget, and `State::`/`Script::` blocks match against it.

That is all `Auth::` does. It does not control visibility, and it does not control
whether the sensor is polling.

**Fingerprint polling is always on** whenever the daemon is waiting for auth,
regardless of what is visible. Disabling it is a `Daemon::` API call available to
scripts; it is not a feature of the markup. A theme cannot silently change the
machine's authentication surface.

---

## 4. States

States are **level**: the body applies while the state holds and is dropped when
it ends.

    State::Focus(reboot_btn){ border="#AAAAAA" }
    State::CapsLock(Root => caps_warn){ opacity=1.0 }

Signals include `Focus`, `Normal`, `Auth`, `AuthFail`, `AuthSuccess`,
`FingerScanning`, `CapsLock`, `VimNormal`, `VimInput`.

---

## 5. Scripts

Scripts are **edge**: the body runs at the instant a signal fires, and its effects
persist.

    Script "::" Subtype "(" ( (list<id> | int) ("=>" list<id ("as" name)>) )? ")" "{" ... "}"

| Subtype | Fires |
|---|---|
| `Startup` | Once, after the whole tree is built. Trigger slot is empty |
| `Input` | On input |
| `Frame(N)` | Every N frames |
| `Timeout(t)` | Every t seconds |
| *state name* | On the rising edge of that state |

The state-named form exists because level semantics cannot express an effect that
must survive the state ending:

    Script::AuthFail(fprint => auth_pager as ap){ ap.active_page = 1 }

The pager must stay flipped after the failure clears. A `State::` body would
revert it.

`Frame` and `Timeout` are edge-only and have no `State::` counterpart.

### Scope

The target list is a **whitelist**. A body may only touch the elements it names,
and the parser rejects one that reaches outside. `as` renames for brevity.

### Body language

Bodies contain daemon API calls plus minimal glue:

- bounded `for i in 0..<N`
- `if`
- arithmetic on numbers

There are no user-defined functions, no variables beyond the scope list, and no
data structures. Nothing unbounded is expressible, so termination is a property
of the grammar rather than a check.

### Defined behaviour

There is no undefined behaviour. Integer overflow wraps. `n % 0 == 0`. Array
indexing wraps. Every edge case has a defined answer.

### Dynamic children

Children added inside a script block are removed at the start of that block's
next run. Those added in `Script::Frame` clear each frame; those added in
`Script::Input` clear each input. Dynamic children are therefore stateless —
they cannot hold anything across runs.

---

## 6. Namespaces

| Namespace | Contents |
|---|---|
| `Daemon::` | Privileged API calls to the daemon |
| `Math::` | Pure arithmetic — `Sin`, `Mod`, `Min`, `Max`, ... |
| `Util::` | Pure helpers — `HSVToHex`, `RGBToHex`, ... |

`Math::` and `Util::` execute in the greeter. They cost no round trip and cannot
be revoked by a scope list, so they stay small and cheap.

Anything that is a fact about the machine stays on `Daemon::`, including
`GetTime()`, `IdleTime()`, `TimeSince(State)` and `InState(State)`.

---

## 7. Linking

The config never sets daemon state. Anything the daemon acts on is
daemon-populated; the config decides only where it goes and what it looks like.

    Daemon::LinkCarousel(users, Daemon::User)
    Daemon::LinkRepeat(password_text, Daemon::PasswordLength, Widget::Circle(...))
    Daemon::LinkAccept(shutdown_btn, Daemon::Shutdown)

| Call | Effect |
|---|---|
| `LinkCarousel(c, source)` | Daemon fills and drives the carousel. The config has no say over its children |
| `LinkRepeat(c, count, template)` | Daemon maintains one child per unit of a daemon-side count |
| `LinkAccept(w, action)` | Widget's accept event invokes a named daemon action |

A linked element cannot disagree with the daemon about what is selected, because
there is only one copy of the fact. Unlinked carousels are legal for display-only
use; anything the daemon acts on must be linked.

`Daemon::Shutdown`, `Daemon::Reboot`, `Daemon::User` and `Daemon::PasswordLength`
are named capabilities, not callables. A script can bind them; it cannot invoke
them.

---

## 8. Attribute resolution

Three writers touch an attribute. They compose as layers, resolved bottom-up:

1. **Markup literal** — the base.
2. **Script writes** — modify the base, permanently.
3. **State bodies** — overlays composited on top while the state is active.

A state ending drops its overlay; it does not restore a saved value. A script
write made during a state therefore survives the state ending, and the two
families never contend for the same slot.

---

## Undecided

- Precedence between a general state and a more specific one
  (`AuthFail` vs `FingerAuthFailed`) if the specific forms are added.
- Ordering between two `State::` overlays touching the same attribute. Id-based
  ranking is a candidate, but only if it becomes necessary.
- What `TimeSince(State)` returns before that state has ever been entered.
- Asset handling: `#load` semantics, caching, and how animations name their
  frames.
