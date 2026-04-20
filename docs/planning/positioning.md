# godom — Positioning & Mental Model Clarity

See also: [motivation.md](motivation.md) — why godom exists, what it's for, what it's not.

## The problem

godom is being perceived as a web application framework. It is not. This leads to expectations (SPA routing, lifecycle hooks, conditional mounting) that don't apply.

The root cause is twofold:
1. Most examples use QuickServe, which looks like "single-page app framework"
2. Developers bring a browser-native mental model where islands live on pages

## What godom actually is

godom is a rendering bridge. Go owns the state. The browser is just a display.

The key mental model shift: **islands are not on a page. Islands exist in the Go process. Their views exist as virtual DOM in Go memory, and are replicated to the browser.**

This is a desktop mindset, not a web mindset:
- A desktop app has windows and panels. They exist whether visible or not.
- Hiding a panel doesn't destroy it. Showing it doesn't create it.
- The process is the source of truth, the screen just reflects it.

In godom:
- Islands are Go structs. They exist from `Register()` until the process exits.
- The VDOM is in Go memory. The browser gets a replica.
- Closing the browser tab doesn't destroy anything. Reopening it gets the current state.
- "Navigation" between pages is just changing which island views the browser is displaying.

## Misperceptions this causes

| What developers expect | What's actually happening |
|---|---|
| "Island loads when I navigate to the page" | Island already exists. Browser requests its current view on connect. |
| "Island unloads when I leave the page" | Island still exists. Browser just stops displaying it. |
| "I need OnMount to initialize data" | Island is initialized when you create it in Go. Browser connect is just rendering. |
| "I need conditional mounting" | The island always exists. Use g-if/g-show to control what the browser displays. |
| "Page navigation loses state" | State is in Go, never lost. The browser just re-requests the view. |
| "I need SPA routing" | You own the HTTP server. Routing is yours. godom renders islands, not pages. |

## What needs to happen

### Documentation
- [ ] Explain the mental model explicitly: islands exist in Go, browser is a display
- [ ] Lead with the desktop analogy early in the guide
- [ ] Make the distinction clear: godom is not a web framework, it's a Go-to-browser rendering bridge

### Examples
- [ ] Rebalance examples: more SetMux + multi-island patterns, less QuickServe-only
- [ ] QuickServe examples should note they're the simple case, not the typical pattern
- [ ] Add an example that clearly shows: close browser, reopen, state is still there

### Upfront framing
- [ ] Add a one-liner at the top of README/docs that sets the mental model immediately. Something like: "godom is a server-side island library for rendering Go state in a browser — it is not a web application framework." Right now you have to read between the lines to understand what godom isn't.

### Patterns page
- [ ] Create a "patterns" page showing the 3-4 things developers reach for immediately:
  - Shared state between islands (embedded struct pointers)
  - Conditional display with g-if + g-island
  - The refresh model: auto for UI-triggered actions, explicit for background changes, MarkRefresh for surgical efficiency
- These aren't missing features — but if the first place a developer looks doesn't show them, they'll assume they don't exist and file it as a gap.

### Terminology
- [ ] Avoid "mounting" language (implies page lifecycle). Prefer "rendering" or "displaying"
- [ ] Avoid "navigation" for page changes. Frame it as "the browser requests a different view"
- [ ] Island "exists" from Register(). Island "renders" when a browser displays it.

---

## The deeper positioning problem

"Not a web framework" is an unsustainable position when the output is a web app. Developers won't care about the internal architecture distinction. They see HTML templates, CSS, DOM events, a browser window — that's web development to them. Telling them "no, think of it as desktop" won't stick because nothing in their daily experience with godom feels like desktop.

"Your Go program is the application, the browser is its UI layer" — this is closer but still has a problem. Electron says the same thing (sort of). Wails says the same thing. You'd need to explain how godom is different from those, and now you're back to defining yourself by what you're not.

The real differentiator is simpler than any of this: **there is no client-side application.** In React, Electron, Wails, Tauri — there's a real application running in the browser with its own state, logic, and lifecycle. In godom, there isn't. The browser has a thin patch-applier and nothing else. State, logic, diffing, decisions — all in Go.

So maybe the honest positioning is: **"godom lets you build UIs in Go. The browser renders them. There is no client-side application."**

That's the actual unique thing. Not "local" (it works over network). Not "desktop" (you write HTML). Not "not a web framework" (it produces web UIs). The unique thing is the absence of a client-side application. Everything runs in Go. The browser is a dumb terminal.

That framing also naturally answers the perceived gaps:
- "Where's the router?" — There's no client-side app to route in. Your Go HTTP server handles routes.
- "Where's OnMount?" — There's no client-side island lifecycle. Your Go code runs when you want it to.
- "Where's island communication?" — Your Go structs can share pointers. It's just Go.
