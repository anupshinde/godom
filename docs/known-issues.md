# Known Issues

## Rapid clicks can drop events

When clicking a button very fast (10+ clicks per second), occasional click events may be lost. For example, 10 rapid clicks might register only 9.

**Cause:** The browser itself does not fire the `click` event for every physical click under rapid input. Console logging in the bridge confirms the event never reaches JavaScript — the browser swallows it, likely during a paint or layout cycle. This is not a godom or WebSocket issue.

**Impact:** Low for typical UI interactions (form submissions, toggles, navigation). Only noticeable with very rapid repeated clicks on the same element.

**Workaround:** None — this is browser behavior below the application layer.

## bridge.js g-for innerHTML parsing is context-sensitive

When `g-for` is used on table elements (`<tr>`, `<td>`, `<th>`), the browser silently strips them when parsed via `innerHTML` on a `<div>` — because these elements are only valid inside `<table>`/`<tbody>`. This caused the stock-ticker example to render empty rows.

**Current fix:** `createTmpContainer()` in bridge.js inspects the HTML string and wraps it in the appropriate table structure. This works but is whack-a-mole — it handles `<tr>`, `<td>`, `<th>` but would miss other context-sensitive elements like `<option>` inside `<select>`, `<thead>`, `<tbody>`, etc.

**Proper fix:** Instead of inspecting the HTML string, use `start.parentNode.tagName` to determine the correct wrapper element. The g-for anchor comment nodes already sit inside the parent, so the context is known. A small tag→wrapper lookup map would cover all cases in one shot. This should be done as part of the broader g-for / bridge.js review.

**Related:** The bridge has grown beyond its original ~150-line scope after adding recursive g-for support. A manual review pass of the full g-for rendering path (Go-side list diffing + bridge-side DOM manipulation) is already planned.
