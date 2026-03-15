# Known Issues

## Rapid clicks can drop events

When clicking a button very fast (10+ clicks per second), occasional click events may be lost. For example, 10 rapid clicks might register only 9.

**Cause:** The browser itself does not fire the `click` event for every physical click under rapid input. Console logging in the bridge confirms the event never reaches JavaScript — the browser swallows it, likely during a paint or layout cycle. This is not a godom or WebSocket issue.

**Impact:** Low for typical UI interactions (form submissions, toggles, navigation). Only noticeable with very rapid repeated clicks on the same element.

**Workaround:** None — this is browser behavior below the application layer.
