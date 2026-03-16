# godom — TradingView Lightweight Charts Demo

## The idea

Integrate TradingView's Lightweight Charts library as a godom plugin and build a small interactive demo around it — something with a gameplay element, like a "predict the next candle" game or a trading simulator where you buy/sell against historical data.

## Why Lightweight Charts

[TradingView Lightweight Charts](https://github.com/nicbarker/TradingView-lightweight-charts) is an open-source, small (~40KB), fast, canvas-based charting library purpose-built for financial data:

- Candlestick, line, area, bar, histogram chart types
- Time-based X axis with proper financial time handling (gaps for weekends/holidays)
- Crosshair, price/time tooltips, price lines, markers
- Smooth real-time updates — designed for live streaming data
- No dependencies, tiny footprint compared to full TradingView or Plotly
- Free and open source (Apache 2.0)

It's the go-to library when you want financial charts without the weight of a full charting framework. Perfect for a focused godom plugin.

## The demo: trading game

A small game that makes the chart interactive and fun:

### "Predict the Market"

1. Go loads historical price data (real or generated) for a stock/crypto
2. The chart shows candles up to a certain point in time — the "now" line
3. The player sees the history and has to decide: **Buy**, **Sell**, or **Hold**
4. They place their bet, then Go reveals the next N candles with an animation
5. Score is tracked based on whether their prediction was correct
6. Repeat — the chart advances, new decision point

### Why this works as a demo

- **Real-time data push** — Go controls the pace, reveals candles one at a time, pushes updates over WebSocket. Shows godom's strength at streaming data to the browser.
- **User interaction** — buy/sell/hold buttons, maybe a slider for position size. All handled as normal godom events.
- **State in Go** — portfolio value, trade history, score, current position. Survives browser refresh. The chart is just a view of Go state.
- **Gameplay loop** — makes it engaging to interact with. More memorable than a static dashboard.

### Alternative demo: live data simulator

If the game angle is too much scope, a simpler version:

- Go generates synthetic price data (random walk with drift, or replay historical data at speed)
- Pushes new candles to the chart in real time
- Moving averages, volume bars, and price markers all update live
- A sidebar shows positions, P&L, and trade log — all godom components alongside the chart plugin

## Plugin design

Same pattern as Chart.js:

```go
import "godom/plugins/lwcharts"

func main() {
    app := godom.New()
    lwcharts.Register(app)
    app.Mount(&TradingGame{}, ui)
    log.Fatal(app.Start())
}
```

```go
type TradingGame struct {
    Chart      lwcharts.Chart
    Score      int
    Portfolio  float64
    Position   string // "long", "short", "flat"
    History    []Trade
}

type Trade struct {
    Action string
    Price  float64
    Result string
}
```

The chart type:

```go
type Chart struct {
    Series []Candlestick
    Markers []Marker         // buy/sell markers on the chart
    PriceLines []PriceLine   // horizontal lines (entry price, stop loss)
    Options map[string]interface{}
}

type Candlestick struct {
    Time  int64   // unix timestamp
    Open  float64
    High  float64
    Low   float64
    Close float64
}
```

```html
<div g-plugin:lwcharts="Chart"></div>

<div class="controls">
    <button g-click="Buy">Buy</button>
    <button g-click="Sell">Sell</button>
    <button g-click="Hold">Hold</button>
</div>

<div class="score">
    <span g-text="Score"></span>
    <span g-text="Portfolio"></span>
</div>
```

Plugin JS adapter:

- `init`: creates chart via `createChart(el, options)`, adds candlestick series, sets initial data
- `update`: calls `series.update(candle)` for new candles (Lightweight Charts has an efficient single-candle update), updates markers and price lines

The library is designed for real-time streaming updates — `series.update()` adds or updates the latest candle without re-rendering the full chart. This matches godom's incremental update model well.

## What this demonstrates

| Aspect | What it shows |
|--------|---------------|
| **Streaming updates** | Go pushes candles one at a time, chart animates them in. Shows real-time data flow. |
| **Plugin + components together** | The chart is a plugin, the controls/score are normal godom components. They coexist on the same page, share state through the Go struct. |
| **Small focused plugin** | ~40KB library, thin adapter. Shows that godom plugins don't have to be heavyweight. |
| **Interactivity** | User actions (buy/sell) create markers on the chart and update game state. Events flow both ways. |
| **Practical appeal** | Financial charts are something people actually want to build. This proves godom can do it. |

## Library embedding

At ~40KB minified, Lightweight Charts is trivial to embed in the Go binary. Even smaller than Chart.js. Single binary, no CDN, fully offline — consistent with godom's approach.

## Status

Small scope, high appeal. The plugin itself is straightforward (thin adapter over a well-designed library). The game demo adds engagement without major complexity — it's mostly Go logic (load data, track score, reveal candles) with a simple UI. Good candidate for a quick, impressive example.
