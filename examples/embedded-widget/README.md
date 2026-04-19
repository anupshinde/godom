# Embedded Widget

Demonstrates godom islands rendered into an **external HTML page** not served by godom's own server. The page is a fictional news site ("MarketPulse") with three live godom widgets injected into it.

## What it shows

- **Register-only pattern** -- no `Mount()` or layout island. Only `Register()` + `Start()`.
- **External hosting** -- a plain Go `http.FileServer` on port 9090 serves the HTML page. godom runs on port 9091.
- **`/godom.js` route** -- the external page loads godom's JS bundle via `<script src="http://localhost:9091/godom.js">`.
- **`GODOM_WS_URL`** -- tells the bridge to connect to the godom server on a different origin.
- **`GODOM_NS`** -- renames `window.godom` to `window.marketpulse` to avoid name collisions on the host page.
- **`g-island` targets** -- the host HTML declares `<div g-island="stock">` and `<div g-island="marquee">` where godom renders.
- **`<style>` in island templates** -- island CSS is scoped by namespace prefixes (`gdstock-*`) to avoid style bleed.
- **`g-shadow` for CSS isolation** -- the heatmap island uses Shadow DOM (`g-shadow` attribute) for full CSS isolation. Host page styles cannot reach inside, and island styles cannot leak out. The host page includes a `.ghm span` rule that would break the heatmap without shadow protection.
- **Same data, two layouts** -- the `stock` and `marquee` islands share the same Go struct but render with different HTML templates (card view vs scrolling ticker).

## Architecture

```
Port 9090 (static server)          Port 9091 (godom)
┌──────────────────────┐           ┌──────────────────────┐
│  index.html          │           │  WebSocket /ws       │
│  ├─ <script godom.js>│──fetch──► │  /godom.js bundle    │
│  ├─ g-island=     │           │                      │
│  │   "stock"         │◄──ws────► │  Stock island     │
│  ├─ g-island=     │           │                      │
│  │   "marquee"       │◄──ws────► │  Stock island     │
│  ├─ g-island=     │           │  (marquee template)  │
│  │   "heatmap"       │◄──ws────► │  Heatmap island   │
│  │   (g-shadow)      │           │  (Shadow DOM)        │
│  └─ static CSS/HTML  │           │                      │
└──────────────────────┘           └──────────────────────┘
```

## Running

```
cd examples/embedded-widget
sh run.sh
```

This runs `GODOM_PORT=9091 GODOM_NO_AUTH=1 go run .` — auth is disabled because the external page on port 9090 needs to connect to godom's WebSocket without a token.

Open http://localhost:9090/ui/ in your browser. The stock ticker card and scrolling marquee update live with simulated prices.
