# godom — Interact with Existing Web Browser

## The idea

Currently godom launches its own browser window and serves its own HTML page. The Go process owns the entire page.

What if a godom app could connect to an already-open browser tab instead? Rather than rendering its own UI, it could observe or manipulate pages the user is already viewing — reading DOM state, injecting behavior, responding to events on arbitrary websites.

## How it could work: Browser Extension

The most practical path is a **godom browser extension**:

1. User installs a godom companion extension in their browser
2. The extension opens a WebSocket connection back to a running godom Go process
3. The Go process sends DOM commands (read elements, click buttons, fill forms, observe changes) through the extension
4. The extension executes those commands on the active tab and sends results back
5. From the Go developer's perspective, the API feels similar to normal godom — but targeting an external page instead of a godom-owned page

This avoids Chrome DevTools Protocol (which requires launching Chrome with special flags) and works across browsers that support extensions (Chrome, Firefox, Edge).

## No concrete use case yet

There's no specific application driving this right now. But the capability could enable:

- **Browser automation** — scripting repetitive browser tasks from Go, with godom's state management keeping track of where you are in a workflow
- **Macro behavior** — record and replay sequences of browser interactions, driven from Go logic rather than fragile JS scripts
- **Page augmentation** — a Go process that watches a page and adds behavior (notifications when something changes, auto-filling forms, extracting data)
- **Testing/scraping** — programmatic interaction with web pages from Go, without headless browser overhead

## Architectural considerations

This is fundamentally different from normal godom:

- **Normal godom:** Go process owns the page, serves the HTML, controls everything
- **Extension mode:** Go process is a guest on someone else's page, must work with unknown DOM structures, handle pages that change independently

The WebSocket transport and protobuf wire format could potentially be reused, but the command set would be different — instead of "set this field on my known component," it would be "find element matching this selector and read its text."

## Status

Exploratory. No implementation planned yet. Captured here because the browser extension approach is the most viable path if this becomes useful.
