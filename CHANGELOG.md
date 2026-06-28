# Changelog

All notable changes to godom are documented here. Versions follow [semantic
versioning](https://semver.org/); while on the 0.x line, minor versions may
include breaking changes (called out explicitly).

## v0.2.0

First tagged release since `v0.1.1`. A large, consolidating release: the
reactivity and per-connection APIs have landed and are proven in a real
application, the template/asset model was reworked (Phase B), and a browser
extension, Shadow DOM, charting plugins, and two new examples joined the tree.
The public API is now considered stable.

### Breaking change

- **`g-component` → `g-island`.** The mount directive was renamed to match the
  "islands" terminology used throughout the docs. **Upgrade:** rename every
  `g-component="…"` to `g-island="…"` in your templates. No behavioral change
  beyond the name.

### Reactivity & connection APIs (new)

- **Computed fields** — `Compute(name, fn, deps...)` declares a derived field the
  engine keeps in sync. Recomputed in dependency order and surgically patched
  whenever a dependency changes; no manual recompute in handlers. See
  `examples/computed-fields`.
- **Async tasks** — `Task(name, fn, opts...)` runs background work off the event
  loop with safety by construction: the closure never touches island state
  directly (results marshal back via `t.Apply`), panics become a failed task, and
  progress is reported with `t.Progress`. Template bindings `Busy(name)` /
  `Progress(name)` / `Err(name)` / `Crashed(name)` drive UI with no app field.
  `WithRestart()` / `WithQueue()` control re-entry. See `examples/async-tasks`.
- **Per-connection client bridge** — `*godom.Client` is an addressable handle for
  a single browser connection. `Clients()` / `ClientsWith(...)` enumerate them;
  `Eval` / `Call` / `CallAsync` target one connection (VDOM patches are
  page-scoped, but raw JS calls otherwise broadcast — this lets you address one
  tab).
- **Connection environment** — islands can read per-connection `Env`/`Viewport`,
  and implement `EnvAware` to get an `OnConnect` hook when a browser attaches.
- **Client JS modules & capabilities** — `RegisterClientModule` ships JS in the
  bundle (loaded after the bridge); capability discovery lets the server learn
  what a given connection supports.

### Template & asset model (Phase B)

- **Three template sources per island:** per-island `AssetsFS` + `Template`,
  inline `TemplateHTML`, or the engine-wide `SetFS` + `Template`. Validated at
  registration.
- **Partials** — stateless HTML fragments via custom-element tags, resolved from
  a sibling file or an engine registry (`RegisterPartial` / `UsePartials`).
- **`<g-slot/>`** — partials can project consumer children into one or more slots.
- **Reference example:** `examples/multi-page-v2` covers the Phase B surface
  end-to-end.

### Rendering & platform

- **Shadow DOM** via `g-shadow` for style isolation.
- **Charting plugins** — Plotly / ECharts bridges.
- **Browser extension** — Chrome MV3 extension that injects godom into arbitrary
  sites via URL rules.
- **Pull-based init** — non-root islands initialize on demand; bridge/init
  pipeline reworked.
- **`ExecJS` + `godom.call`**, `g-html`, richer `g-expr` comparisons & ternary,
  `g-attr:` boolean attributes, mux routing / custom server control, and
  per-connection env config.

### Fixes & internals

- Cross-island refresh propagation for mixed-content interpolation (#39).
- Bind to all interfaces for LAN access (#34).
- Event-queue concurrency hardening, node-lookup map correctness (COR-70),
  `g-attr` boolean handling (COR-71).
- Internal config de-duplication between public and server layers (COR-73).

### Quality

- `make` gates all green: build, `go vet`, `go test -race`, example
  build/validate, 92%+ coverage.
- Test conventions are documented in `docs/testing.md`.

## v0.1.1 — 2026-03-17

Early tagged release (VDOM engine, islands, wire protocol). See git history for
detail.
