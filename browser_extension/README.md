# godom Injector — Chrome Extension

A Chrome extension that injects godom.js into websites, allowing godom apps to enhance any webpage with interactive components.

## How it works

1. You configure **rules** — each rule maps URL patterns to a godom app
2. When you visit a matching page, the extension fetches `godom.js` from the app server and injects it into the page
3. A floating godom badge appears in the bottom-right corner
4. Click the badge to open a resizable sidebar panel where a godom component renders
5. The godom app can also render named components anywhere on the page via `g-component` attributes

## Installation

1. Open `chrome://extensions`
2. Enable **Developer mode** (top-right toggle)
3. Click **Load unpacked** and select the `browser_extension/` folder

## Configuration

Click the extension icon to open the settings page. Each rule has:

| Field | Description | Default |
|-------|-------------|---------|
| **Rule Name** | Label for the rule | — |
| **godom App URL** | Where the godom app is running | — |
| **Script Path** | Path to the godom.js bundle | `/godom.js` |
| **Panel Component** | `g-component` name for the sidebar panel | `extension` |
| **Isolate panel CSS** | Use shadow DOM to prevent host page CSS from affecting the panel | On |
| **Allow root mode** | Let the app render into `document.body` (replaces the page) | Off |
| **Include Pages** | URL patterns where injection should happen (one per line, `*` = wildcard) | — |
| **Exclude Pages** | URL patterns to skip even if included (one per line) | — |

### URL pattern examples

```
https://example.com/*          — all pages on example.com
https://github.com/myorg/*     — all pages under myorg
https://app.example.com/dash*  — pages starting with /dash
```

Excludes override includes.

## Export / Import

Use the **Export** and **Import** buttons to share rules between machines. Export copies the JSON to clipboard, Import accepts pasted JSON.

## Cross-machine setup

The extension works with godom apps on the same machine (`localhost`) out of the box. For apps running on another machine on your network, HTTPS is required because browsers block insecure WebSocket connections (`ws://`) from HTTPS pages.

The simplest setup is a reverse proxy with TLS on the godom machine:

```bash
# Using Caddy
caddy reverse-proxy --from https://192.168.1.50:9443 --to localhost:9091
```

Then use `https://192.168.1.50:9443` as the App URL.

Other options: Cloudflare Tunnel (`cloudflared tunnel --url http://localhost:9091`) or Tailscale with HTTPS certificates.

## Named components vs root mode

godom apps can register components in two ways:

- **Named components** (e.g. `counter`, `clock`) — render into `g-component` elements on the page. These work with the extension sidebar panel and can be injected anywhere.
- **Root mode** (`document.body`) — replaces the entire page. The extension blocks this by default (`Allow root mode` is off) to prevent wiping out the host page.

The sidebar panel renders the component specified in **Panel Component** (default: `extension`). Your godom app should register a component with that name for it to appear in the sidebar.

## File structure

```
browser_extension/
  manifest.json               — Chrome Manifest V3
  background/background.js    — service worker: injection, icon click handler
  content/content.js          — content script: URL matching, triggers injection
  options/                    — settings UI (opens in full tab)
  icons/                      — godom favicon as PNG (16, 48, 128px)
```
