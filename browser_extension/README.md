# godom Injector — Chrome Extension

A Chrome extension that injects godom.js into websites, allowing godom apps to enhance any webpage with interactive islands.

## How it works

1. You configure **rules** — each rule maps URL patterns to a godom app
2. When you visit a matching page, the extension fetches `godom.js` from the app server and injects it into the page
3. A floating godom badge appears in the bottom-right corner
4. Click the badge to open a resizable sidebar panel where a godom island renders
5. The godom app can also render named islands anywhere on the page via `g-island` attributes

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
| **Panel Island** | `g-island` name for the sidebar panel | `extension` |
| **Isolate panel CSS** | Use shadow DOM to prevent host page CSS from affecting the panel | On |
| **Enabled** | Whether the rule is active | On |
| **Show badge** | Show the floating icon and sidebar panel | On |
| **Allow root mode** | Let the app render into `document.body` (replaces the page) | Off |
| **Include Pages** | URL patterns where injection should happen (one per line, `*` = wildcard) | — |
| **Exclude Pages** | URL patterns to skip even if included (one per line) | — |

Rules can be enabled/disabled and badge visibility toggled directly from the rule list without opening the editor.

### URL pattern examples

```
https://example.com/*          — all pages on example.com
https://github.com/myorg/*     — all pages under myorg
https://app.example.com/dash*  — pages starting with /dash
```

Excludes override includes.

## Sidebar panel

The sidebar panel slides in from the right when you click the floating badge.

- **Resizable** — drag the left edge to resize (200px to 80% of viewport)
- **Splits the page** — the host page shrinks to make room, no content is hidden
- **Persists across navigations** — sidebar open/closed state and width are remembered per site via sessionStorage
- **Kebab menu** (⋮) with:
  - **Close Panel** — closes the sidebar
  - **Maximize** — expands the panel to cover the full page. Close button restores to sidebar.
  - **Hide Badge** — hides the badge and panel for this rule (restore from extension settings)

## Export / Import

Use the **Export** and **Import** buttons to share rules between machines. Export copies the JSON to clipboard, Import accepts pasted JSON.

## Cross-machine setup

The extension works with godom apps on the same machine (`localhost`) out of the box. For apps running on another machine on your network, HTTPS is required because browsers block insecure WebSocket connections (`ws://`) from HTTPS pages.

The simplest setup is a reverse proxy with TLS on the godom machine:

```bash
# Using Caddy
caddy reverse-proxy --from https://192.168.1.xx:9443 --to localhost:9091
```

Then use `https://192.168.1.xx:9443` as the App URL.

Other options: Cloudflare Tunnel (`cloudflared tunnel --url http://localhost:9091`) or Tailscale with HTTPS certificates.

The extension shows a warning when a non-HTTPS URL is used with a remote host.

## Named islands vs root mode

godom apps can register islands in two ways:

- **Named islands** (e.g. `counter`, `clock`) — render into `g-island` elements on the page. These work with the extension sidebar panel and can be injected anywhere.
- **Root mode** (`document.body`) — replaces the entire page. The extension blocks this by default (`Allow root mode` is off) to prevent wiping out the host page. This is controlled by the `GODOM_INJECT_ALLOW_ROOT` bridge flag.

The sidebar panel renders the island specified in **Panel Island** (default: `extension`). Your godom app should register an island with that name for it to appear in the sidebar.

## File structure

```
browser_extension/
  manifest.json               — Chrome Manifest V3
  background/background.js    — service worker: injection, icon click handler
  content/content.js          — content script: URL matching, triggers injection
  options/                    — settings UI (opens in full tab)
  icons/                      — godom favicon as PNG (16, 48, 128px)
```
