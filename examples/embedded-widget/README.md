# Embedded Widget

Demonstrates godom components rendered into an **external HTML page** not served by godom's own server. The page is a fictional news site ("MarketPulse") with two live godom widgets injected into it.

## What it shows

- **Register-only pattern** -- no `Mount()` or layout component. Only `Register()` + `Start()`.
- **External hosting** -- a plain Go `http.FileServer` on port 9090 serves the HTML page. godom runs on port 9091.
- **`/godom.js` route** -- the external page loads godom's JS bundle via `<script src="http://localhost:9091/godom.js">`.
- **`GODOM_WS_URL`** -- tells the bridge to connect to the godom server on a different origin.
- **`GODOM_NS`** -- renames `window.godom` to `window.marketpulse` to avoid name collisions on the host page.
- **`g-component` targets** -- the host HTML declares `<div g-component="stock">` and `<div g-component="marquee">` where godom renders.
- **`<style>` in component templates** -- component CSS is scoped by namespace prefixes (`gdstock-*`) to avoid style bleed.
- **Same data, two layouts** -- the `stock` and `marquee` components share the same Go struct but render with different HTML templates (card view vs scrolling ticker).

## Architecture

```
Port 9090 (static server)          Port 9091 (godom)
┌──────────────────────┐           ┌──────────────────────┐
│  index.html          │           │  WebSocket /ws       │
│  ├─ <script godom.js>│──fetch──► │  /godom.js bundle    │
│  ├─ g-component=     │           │                      │
│  │   "stock"         │◄──ws────► │  Stock component     │
│  ├─ g-component=     │           │                      │
│  │   "marquee"       │◄──ws────► │  Stock component     │
│  └─ static CSS/HTML  │           │  (marquee template)  │
└──────────────────────┘           └──────────────────────┘
```

## Running

```
cd examples/embedded-widget
sh run.sh
```

This runs `GODOM_PORT=9091 GODOM_NO_AUTH=1 go run .` — auth is disabled because the external page on port 9090 needs to connect to godom's WebSocket without a token.

Open http://localhost:9090/ui/ in your browser. The stock ticker card and scrolling marquee update live with simulated prices.
