# multi-page

A multi-page godom app using the developer-owned mux pattern — pages rendered with Go's `html/template`, islands mounted into page-level targets via `g-island`.

This is the **original pre-Phase-B** pattern: one engine-wide filesystem (`eng.SetFS(fs)`), one embed for everything, islands pointed at templates inside that FS with paths like `"components/counter/index.html"`.

## Newer alternative: `../multi-page-v2/`

For a richer demo that uses **Phase B** features — per-island `AssetsFS`, inline `TemplateHTML`, shared partials via `RegisterPartial` / `UsePartials`, `<g-slot/>` children substitution, and an `os.DirFS` hot-edit dev mode — see **[examples/multi-page-v2/](../multi-page-v2/)**.

The v2 example demonstrates tool packages that ship their own HTML alongside Go code (no engine-wide `SetFS` required), a shared `<info-note>` partial with slotted children, and a self-contained 3D solar-system tool that embeds its own plugin JS.

## When to pick which

| Pattern | Use when |
|---|---|
| This example (`multi-page/`) | You have one consolidated `ui/` folder and every island's template lives there. Simple apps. |
| [multi-page-v2/](../multi-page-v2/) | You want portable tool packages (Go + HTML + plugin JS colocated), shared reusable partials across islands, or hot-edit dev mode. |

Both patterns are fully supported. v2 doesn't replace v1 — it adds options.

## Run

```bash
GODOM_NO_AUTH=1 go run ./examples/multi-page/
```
