# multi-page-v2

A multi-page godom app demonstrating Phase B features end-to-end. Four tool islands (counter, clock, simulated system monitor, 3D solar system) each live in their own Go package with colocated HTML; a fifth inline-HTML tool (digiclock) shows up as live prose on the dashboard.

## Run

```bash
GODOM_NO_AUTH=1 go run ./examples/multi-page-v2/
```

With DEV mode (reads `shared/*.html` via `os.DirFS` at runtime — edit and refresh without rebuild):

```bash
GODOM_DEV=1 GODOM_NO_AUTH=1 go run ./examples/multi-page-v2/
```

Run from the repo root so `os.DirFS("examples/multi-page-v2")` resolves correctly.

## Routes

| Path | Page |
|---|---|
| `/` | Dashboard — welcome banner + 3-tile row (counter/clock/monitor) + full-width solar |
| `/counter` | Counter island with local-sibling button partials and a shared info-note |
| `/clock` | Analog clock |
| `/monitor` | Simulated CPU/memory chart via the Chart.js plugin |
| `/solar` | 3D solar system — Go computes draw commands, canvas3d plugin paints |

## What each piece demonstrates

| Feature | Where |
|---|---|
| `Island.AssetsFS` + `//go:embed` | [tools/counter](tools/counter/counter.go), [tools/clock](tools/clock/clock.go), [tools/monitor](tools/monitor/monitor.go), [tools/solar](tools/solar/solar.go) |
| `Island.TemplateHTML` (inline) | [tools/digiclock/digiclock.go](tools/digiclock/digiclock.go) |
| `Engine.SetFS` (default FS) | `Welcome` banner island declared in [main.go](main.go) — template at `shared/welcome.html` |
| `Engine.UsePartials` (bulk) | [partials/info-note.html](partials/info-note.html) — used by every tool's teaching note |
| `Engine.RegisterPartial` (raw string) | `<kbd-key>` inline partial in [main.go](main.go) — used in the welcome banner |
| Local sibling partials | [tools/counter/ui/](tools/counter/ui/) — `<decrement-button/>` and `<increment-button/>` resolve to sibling files |
| `<g-slot/>` children | Every `<info-note>` — content passed between open/close tags lands inside the partial |
| `os.DirFS` dev mode | [main.go](main.go) — `GODOM_DEV=1` switches SetFS from embed to disk |
| Shared pointer state | `*counter.State` embedded into counter, clock, monitor, and digiclock — auto-refresh across all four |
| Plugins | `chartjs.Plugin` for monitor, `solar.Plugin` for the canvas3d bridge |
| Page chrome via `html/template` | [pages/layout/base.html](pages/layout/base.html) — static HTML with `g-island` targets; not rendered by godom |

## Layout

```
examples/multi-page-v2/
├── main.go                  engine wiring, routes, DEV-mode switch
├── pages/                   Go html/template page chrome
│   ├── layout/base.html     nav + outer shell, included by every page
│   └── {dashboard,counter,clock,monitor,solar}/page.html
├── partials/                shared godom partials (UsePartials target)
│   └── info-note.html       callout with <g-slot/>
├── shared/                  engine's SetFS content
│   └── welcome.html         dashboard banner template
└── tools/                   one Go package per tool
    ├── counter/counter.go + ui/counter.html + ui/{decrement,increment}-button.html
    ├── clock/clock.go + clock.html
    ├── monitor/monitor.go + monitor.html
    ├── solar/solar.go + solar.html + canvas-bridge.js + engine.go + bodies.go + vec3.go
    └── digiclock/digiclock.go   (no HTML — TemplateHTML is inline in Go)
```

## Key design choices

- **Tools live in Go packages.** Each tool is self-contained — Go code, HTML, plugin JS (solar) all under one folder. Main never references a specific tool's template path; it just calls `tool.New(...)`.
- **Main doesn't call `SetFS` for tool templates.** Tools bring their own `AssetsFS`. `SetFS` is used here only for the `welcome` island.
- **Page chrome is Go html/template, not godom.** Static content (nav, layout) doesn't need godom; the dynamic bits are wrapped in `<div g-island="name"></div>` targets and godom hydrates them.
- **Shared state via embedded pointer.** `*counter.State` lives in main; each island embeds the pointer. When Counter mutates it, godom's shared-pointer refresh auto-refreshes every other island holding the same pointer.

## See also

- [docs/why-islands.md](../../docs/why-islands.md) — why godom calls these "islands" not "components"
- [docs/guide.md](../../docs/guide.md) — getting-started guide
- [docs/llm-reference.md](../../docs/llm-reference.md) — full API reference
