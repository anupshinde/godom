# godom — What's Next

All planned features, improvements, and ideas are tracked in the GODOM project on Linear.

## Upcoming work

### High priority

| # | Issue | Type |
|---|-------|------|
| 1 | COR-76: Pull-based component init (bridge requests inits instead of push order) | Improvement |
| 2 | COR-73: Reduce internal config duplication between public and server packages | Improvement |
| 3 | COR-77: Showcase example: multi-page dashboard demonstrating all godom capabilities | Example |
| 4 | COR-79: Track page/route per WebSocket connection via Referer header | Feature |

### Medium priority

| # | Issue | Type |
|---|-------|------|
| 5 | COR-45: Inactive component pausing — skip patches when no DOM targets | Improvement |
| 6 | COR-53: Streaming / append-only updates (bypass VDOM) | Feature |
| 7 | COR-58: Developer experience: debug logging and element inspector | Feature |
| 8 | COR-67: Example: godom components embedded in a React app | Example |
| 9 | COR-68: Example: godom with external JS component library (e.g. Shoelace) | Example |

### Low priority

| # | Issue | Type |
|---|-------|------|
| 10 | COR-75: Allow CSS selectors as component targets (RegisterAt) | Feature |
| 11 | COR-78: Customizable disconnect overlay and badge | Improvement |
| 12 | COR-47: Dynamic mount from JS: `window.godom.mount()` | Feature |
| 13 | COR-48: Shadow DOM isolation (optional per-component) | Feature |
| 14 | COR-52: Virtual scrolling for large lists | Feature |
| 15 | COR-56: Tree version guard for stale patch detection | Improvement |
| 16 | COR-69: Alternative transport implementations (SSE+POST, REST API, WebTransport) | Feature |
| 17 | COR-60: Cross-app broadcast messaging (racing game demo) | Feature |

### Ideas / Future

| # | Issue | Type |
|---|-------|------|
| 18 | COR-62: TradingView Lightweight Charts plugin + trading game | Plugin/Example |
| 19 | COR-61: Plotly / ECharts plugins | Plugin |
| 20 | COR-63: Full-scale application (accounting/CRM/spreadsheet) | Example |
| 21 | Template compiler (compile HTML + directives to Go render functions) | Feature |
