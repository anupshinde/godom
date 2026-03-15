# Why godom

## The problem

You want to build a local app. You want a UI. Your options today:

1. **Electron/Tauri/Wails** — package a browser (or webview) into the binary, get a native window. But now you're shipping a browser runtime, dealing with build toolchains, and for Electron specifically, each app is 100MB+ of Chromium.

2. **Native GUI toolkits** (Qt, GTK, Fyne, Gio) — cross-platform is painful, bindings are fragile, and the UI never looks as good as HTML/CSS.

3. **Web app** — build a Go API server, write a JS frontend. Now you're maintaining two codebases, two build systems, and the developer experience is API-centric: you think in HTTP endpoints and JSON payloads, not in "user clicked a button."

godom takes a different path: **your Go process is the app, and the browser is just the screen.**

## The key insight

Most machines already have a browser. It's the best UI rendering engine available — responsive, accessible, styled with CSS, debuggable with devtools. Why package another copy of it into every app?

godom doesn't embed a browser or create a native window. It starts an HTTP server on localhost and opens a tab in whatever browser you already have. There is no "application window" — the browser tab *is* the window.

This is not a web framework. There are no API endpoints, no REST, no JSON contracts between frontend and backend. You write a Go struct, put `g-click="Save"` in your HTML, and the framework calls your Go method. The developer experience is closer to desktop GUI programming than web development.

## Local apps and local services

godom is for local use. This is not a limitation — it's the core assumption that makes everything simpler:

- **No auth** — you don't authenticate with your own desktop apps. The user running the Go process is the user viewing the UI. Same trust boundary.
- **No HTTPS** — localhost doesn't need TLS. The traffic never leaves the machine.
- **No CORS, no CSP, no rate limiting** — none of the security machinery that protects multi-user web apps applies here.
- **No deployment ceremony** — `go build` gives you one binary. Run it, the UI appears. Stop it, the UI is gone.

godom also works as a **local network service**. Run the binary on a headless machine, bind it to the local IP, and access the UI from any browser on the network. This is useful for home servers, lab machines, Raspberry Pis — anything where you want a UI without a monitor.

Because state lives in the Go process — not in the browser — you get two things for free:

- **State survives the browser.** Close the tab, reopen it, and the app is exactly where you left off. The Go process didn't lose anything — it just sends the current state to the new connection.
- **Multiple tabs stay in sync.** Open the app in two browser windows and type in one — the other updates instantly. This isn't a feature we built; it's a natural consequence of Go owning the state and pushing DOM commands to every connected tab.

These aren't things you'd normally get from a web app without explicit sync infrastructure. Here they fall out of the architecture for free.

**Security note:** godom has no authentication or encryption. It assumes a trusted network. If your local network is compromised, you have bigger problems than godom. We may add optional auth in the future, but it is not a priority — the project is designed around trusted environments.

## Why not Wails, Tauri, or Electron

| | Electron | Tauri | Wails | godom |
|---|---|---|---|---|
| Ships a browser | Yes (Chromium) | No (system webview) | No (system webview) | No (system browser) |
| Native window | Yes | Yes | Yes | No — browser tab |
| Frontend language | JS/TS | JS/TS | JS/TS | None (HTML + Go) |
| Build toolchain | Node + Electron Forge | Rust + Node + Cargo | Go + Node | `go build` |
| Binary size | 100MB+ | ~5MB | ~8MB | ~5MB (Go binary only) |
| Target | Desktop apps | Desktop apps | Desktop apps | Local apps and services |
| Run as a service | No | No | No | Yes — headless machine, local network |

The fundamental difference: Wails, Tauri, and Electron all create a **desktop application with an embedded webview**. You still write JavaScript for the frontend. The Go/Rust backend communicates with the JS frontend through bindings.

godom eliminates the JS layer entirely. The Go process owns the DOM. The browser executes rendering commands — it doesn't run application logic. There is no frontend/backend split because there is no frontend.

The tradeoff: you don't get a native window, a dock icon, or OS-level window management. Your app lives in a browser tab. For many local tools — dashboards, admin panels, dev tools, config editors, data viewers — that's perfectly fine.

## When godom is the wrong choice

- You need a native window, system tray, or OS integration → use Wails or Tauri
- You're building a multi-user web application → use a web framework
- You need offline-first with service workers → godom requires the Go process running
- You want a mobile app → godom is desktop browsers only
