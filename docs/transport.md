# Transport: WebSocket vs SSE + POST

godom uses WebSocket for bidirectional communication between Go and the browser. We evaluated SSE + POST as a zero-dependency alternative and decided against it for now.

## How SSE + POST would work

**Server-Sent Events (SSE)** is a one-way streaming HTTP response. The browser connects to an endpoint and the server holds the connection open, pushing messages as `data:` lines:

```
data: {"type":"init","commands":[...]}

data: {"type":"update","commands":[...]}
```

Browser side uses the built-in `EventSource` API:

```js
var es = new EventSource("/events");
es.onmessage = function(evt) {
    var msg = JSON.parse(evt.data);
    execCommands(msg.commands);
};
```

For browser → Go (user events like clicks, keystrokes), the browser sends regular HTTP POST requests:

```js
fetch("/event", {
    method: "POST",
    body: JSON.stringify({type: "call", method: "AddTodo"})
});
```

Go side is a normal HTTP handler. No special libraries needed.

## Why we chose WebSocket

### 1. No debouncing — by design

godom's `g-bind` fires on every keystroke via the `input` event with **no debouncing**. This is a deliberate design choice, not an oversight. Debouncing would mean the Go state lags behind what the user sees in the input field — the two-way binding would feel sluggish, and any directive that depends on the bound field (e.g. a filtered list driven by a search box) would update in visible jumps rather than smoothly.

Because we don't debounce, a user typing "hello world" generates 11 messages in quick succession. Each message triggers a full round trip: browser sends the keystroke, Go updates state, Go diffs and sends back DOM commands.

With WebSocket, all 11 messages go over a single already-open connection. Zero connection overhead — just frames on a persistent TCP stream.

With POST, each keystroke is a separate HTTP request. On localhost with keep-alive this still works, but each request carries HTTP headers and request/response framing. More importantly, POST requests can be queued or serialized by the browser differently than WebSocket frames, which can introduce subtle ordering or timing issues under load. The no-debounce design amplifies this: what would be tolerable overhead for occasional clicks becomes constant overhead for every character typed.

This is also why SSE + POST would be a poor fit if we ever want the bridge to stay dumb. Adding client-side debounce to make POST viable would mean the bridge is making timing decisions — it would need to know how long to wait, whether to batch, and how to handle rapid sequences. That pushes logic into JS, which contradicts the dumb-bridge principle.

### 2. Future high-frequency events

Drag-and-drop, resize handles, canvas drawing, and similar interactions generate events at 60+ fps. While the right design is to handle the visual part in the bridge and only send the final result to Go, there are legitimate cases where Go needs per-frame updates (e.g., a Go-side physics simulation driving a canvas).

WebSocket handles this naturally. POST at 60fps would create significant overhead even on localhost.

### 3. Broadcast semantics

godom supports multiple browser tabs viewing the same app state. When one tab triggers a state change, all tabs get the update.

With WebSocket, we maintain a connection pool and write to all connections.

With SSE, we'd maintain an equivalent pool of open SSE response writers. This works fine — SSE supports the same pattern. No real difference here.

### 4. Reconnection

SSE has built-in auto-reconnect via `EventSource`. WebSocket requires manual reconnect (our bridge does `setTimeout(connect, 1000)`). SSE is slightly better here.

## When SSE + POST would be the right choice

- If godom needed to work behind corporate proxies that block WebSocket upgrades
- If we wanted zero external dependencies (SSE + POST is pure stdlib)
- If the message pattern was heavily asymmetric — lots of server push, rare client events

For godom (localhost-only, fast input binding, potential high-frequency events), WebSocket is the better fit.

For ideas on alternative transports and media streaming, see [planning/future-protocol-extensions.md](planning/future-protocol-extensions.md).
