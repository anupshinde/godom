# godom — Example Ideas

Ideas for example apps that showcase godom's capabilities and push its boundaries.

---

## Cross-App Communication: Racing Game with Mobile Controller

A two-app demo that introduces a broadcast messaging primitive into godom. Two independent godom apps communicate over the network without knowing about each other — fire-and-forget, pub/sub style.

### The demo

**App1 — Racing Game (desktop/TV)**
- Renders a car racing game in the browser
- Default controls: keyboard (arrow keys for steering, space for brake)
- Subscribes to broadcast signals: `steering`, `throttle`, `brake`
- When signals arrive, they act as an additional input source alongside keyboard
- Game state lives entirely in Go — survives browser close/reopen, mobile disconnect/reconnect

**App2 — Mobile Controller (phone, landscape mode)**
- Renders a steering wheel UI in the browser, opened on a phone over LAN
- Reads the phone's gyroscope and accelerometer via the browser DeviceOrientation API (small JS plugin)
- Tilting the phone left/right publishes `steering` signals with angle values
- Tilting forward/backward publishes `throttle`/`brake` signals with intensity values
- App2 has no idea App1 exists — it just shouts signals into the void

### What this actually builds into godom

The racing game is the demo, but the real deliverable is a **cross-app broadcast messaging layer**:

- Any godom app can publish signals to a broadcast channel
- Any godom app can subscribe to signals on a broadcast channel
- No direct coupling between apps — publishers don't know who's listening, subscribers don't know who's publishing
- "Shout into the void and forget" semantics — if nobody is listening, the signal is simply lost
- Works across devices on the same LAN (fits naturally with godom's existing LAN IP resolution and QR code features)

### How it works

1. **Discovery** — Apps on the same LAN discover each other. Options: UDP multicast, mDNS/Bonjour, or a tiny shared coordinator process. LAN discovery is already partially in godom (IP resolution for QR codes), so this extends that.

2. **Signal broadcast** — When App2 reads a gyro tilt, it calls something like:
   ```go
   app.Broadcast("steering", map[string]interface{}{
       "angle": -15.3,
       "timestamp": time.Now().UnixMilli(),
   })
   ```
   This sends a UDP packet (or similar) to all listening godom apps on the network. No acknowledgment, no retry — fire and forget.

3. **Signal subscription** — App1 registers a listener:
   ```go
   app.OnSignal("steering", func(data map[string]interface{}) {
       angle := data["angle"].(float64)
       game.SteerTo(angle)
   })
   ```
   Signals arrive as they come. If the mobile disconnects, signals just stop — no error, no timeout. The game falls back to keyboard input naturally.

4. **Gyro/tilt bridge** — A small godom plugin wraps the browser's DeviceOrientation API:
   - JS side: `window.addEventListener("deviceorientation", ...)` reads alpha/beta/gamma
   - Sends readings over the existing godom WebSocket to the Go process
   - Go process converts raw orientation data into semantic signals (steering angle, throttle intensity) and broadcasts them
   - This is a thin plugin — the same pattern as the Chart.js plugin, just for a different browser API

### Why there's confidence this can work: the Solar System example

This idea is pushing boundaries, and the game experience may not be perfect on the first pass. But there's real evidence it can work at some grade of success.

The `examples/solar-system/` demo already runs a continuous animation loop — planets orbiting, rotating, all driven from Go state pushed over the binary WebSocket. It renders at good FPS. More importantly, it was tested on two different devices simultaneously (desktop + mobile over LAN) and both ran smooth. Even a kid loved playing with it, dragging planets around on the phone while watching the system animate on the desktop.

That experience is what sparked this idea. If godom can push a solar system animation to two devices at once with good FPS, it can push steering/throttle signals from a phone to a game on a desktop. The solar system demo is a single app broadcasting to multiple browsers. This racing demo is two apps broadcasting to each other — a natural next step.

### Why this is a good fit for godom

- **Already proven at speed** — the solar system example shows godom handles continuous high-frequency state updates across multiple devices on LAN
- **Binary WebSocket is already there** — the protobuf transport handles high-frequency sensor data (gyro readings at 60Hz) without breaking a sweat
- **State lives in Go** — game state survives if the mobile controller disconnects and reconnects, or if the desktop browser tab is closed and reopened
- **Single binary per app** — `go build` produces one binary for the game, one for the controller, no server infrastructure
- **LAN is already supported** — godom already resolves LAN IPs and generates QR codes for mobile access, the broadcast layer extends this
- **Plugin system handles the JS part** — DeviceOrientation is a browser API, exposed through the existing plugin pattern with minimal JS

### Design principles for the broadcast layer

- **Decoupled** — Apps don't import each other. They agree on signal names (strings) and payload shapes, nothing more.
- **Ephemeral** — Signals are not stored, queued, or replayed. If you missed it, it's gone. This keeps the system simple and predictable.
- **Transport-agnostic** — The broadcast layer defines the API. The underlying transport (UDP multicast, NATS, Redis pub/sub, WebSocket relay) is swappable. The demo uses the simplest thing that works on LAN.
- **Optional** — Apps that don't use broadcast don't pay for it. No extra goroutines, no network listeners, no dependencies.

### Beyond the racing game

Once the broadcast primitive exists, other demos become possible:

- **Presentation remote** — App1 shows slides, App2 on mobile sends next/prev/laser-pointer signals
- **Collaborative whiteboard** — Multiple godom apps on different devices, each broadcasting drawing strokes
- **IoT dashboard** — A sensor-reading app broadcasts temperature/humidity, a dashboard app subscribes and charts it
- **Multi-screen experience** — One app controls what appears on another (kiosk mode, digital signage)

The broadcast layer is the primitive. The racing game is just the most fun way to prove it works.
