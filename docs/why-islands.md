# Why "island", not "component"?

godom calls its stateful runtime units **islands**, not components. This page explains why.

## The two tiers

godom has two composition tiers that look similar but cost very different things:

| Tier | How you use it | Runtime cost | Example |
|---|---|---|---|
| **Partial** (stateless) | `<my-button>` in a template, resolved to `my-button.html` from the filesystem | Zero — pure text substitution at parse time | shared buttons, icons, layout fragments |
| **Island** (stateful) | `g-island="name"` + a Go struct embedding `godom.Island`, registered with `eng.Register` | Real — a goroutine, an event queue, a VDOM subtree, isolated state | anything with behavior, data, or a lifecycle |

A partial is free. An island is a long-lived Go runtime unit.

## Why "component" was the wrong word

If you're coming from React, Vue, Angular, or Web Components, "component" conjures:

- a **lightweight** rendering primitive
- **cheap** to create, destroy, compose
- the natural building block, reached for by default

godom's stateful units are none of those. They carry:

- a goroutine (one per island, not per render)
- a buffered event channel with a dedicated processor
- persistent state that survives browser close/reopen
- independent lifecycle, initialization, and teardown

Calling them "components" trained users — and assistants — to reach for them the way they'd reach for `<Button />` in React. That's wrong for godom: a button is a partial, not a unit of state.

## Why "island"

The pattern godom implements is called **islands architecture**. It's the model behind Astro, Qwik, Marko, and Deno Fresh: a page is a static HTML sea with isolated, interactive units (islands) dropped into it. Each island owns its state, hydration, and lifecycle.

That's exactly what a godom `g-island` is. Using the canonical name:

- Signals the weight honestly — users reach for it when they actually want an isolated runtime unit.
- Transfers vocabulary for free — anyone who's read about the pattern gets the cost model and the boundaries.
- Scales cleanly — "a page has many islands" reads naturally; "a page has many apps" does not.

The browser extension injecting stateful units into arbitrary host pages is literally the canonical islands-architecture example.

## What changed in the rename

| Before | After |
|---|---|
| `g-component="name"` | `g-island="name"` |
| `godom.Component` (embed) | `godom.Island` |
| `Register(comp)` | `Register(isl)` (same shape; param rename only) |
| `Components() []*Info` | `Islands() []*Info` |
| `internal/component/` | `internal/island/` |
| `BuildComponentInfo` | `BuildIslandInfo` |
| `ExpandComponents` | `ExpandPartials` (the HTML-include mechanism is about *partials*, not islands) |
| `examples/multi-component/` | `examples/multi-island/` |
| `examples/same-component-repeated/` | `examples/same-island-repeated/` |

`TargetName` on the embed is unchanged — it still names the mount target (`<div g-island="header">`).

The protobuf field `BrowserMessage.component` was renamed to `island`. Wire format is unchanged (protobuf uses field numbers, not names — field 40 still carries the init-request target name).

## What Phase B added (on top of the rename)

Phase B landed alongside the rename and is **purely additive** — nothing was removed or altered. Every pre-Phase-B app (one `SetFS(fs)` plus `Template: "ui/counter.html"` on each island) continues to work unchanged.

New surface:

| Addition | Purpose |
|---|---|
| `Island.AssetsFS fs.FS` | Per-island filesystem. Lets a tool package ship its own `//go:embed` instead of relying on the engine-wide `SetFS`. |
| `Island.TemplateHTML string` | Inline HTML — skips the filesystem entirely. For tiny islands with one template and no sibling partials. |
| `Engine.RegisterPartial(name, html)` | Register a shared partial by name from a raw Go string. Looked up when a template uses `<name>`. |
| `Engine.UsePartials(fs, baseDir)` | Bulk-register every `*.html` in a directory as a named partial. |
| `<g-slot/>` marker in partials | Partials can carry a `<g-slot/>`; any content between a consumer's custom-element tags replaces it. Partials without a slot discard children (the prior behavior). |

`Engine.SetFS(fs)` is now optional — an island with `AssetsFS` or `TemplateHTML` doesn't need the engine-wide default.

See [docs/guide.md#partials](guide.md#partials) and [examples/multi-page-v2](../examples/multi-page-v2) for walkthroughs.

## Where "component" still appears in docs

Intentionally, when referring to external frameworks and standards:

- **React/Vue/Angular components** — unchanged; those are their terms.
- **Web Components API** — unchanged; that's the W3C term for `<custom-element>` with Shadow DOM and lifecycle callbacks.
- **custom elements** (HTML standard) — godom's partials build on custom-element syntax (hyphenated tag names), but godom partials don't hydrate or carry a class definition — they're just file includes.

If a doc says "component" without qualifying it by framework, it's probably stale — please file an issue or PR.
