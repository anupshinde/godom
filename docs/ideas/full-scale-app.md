# godom — Full-Scale Application

## The idea

Build a real, useful application in godom — not a demo, not a toy, but something you'd actually use day to day. The goal is to push godom into territory where a framework either proves itself or breaks. Find out what's missing when you try to build something serious.

If the app grows beyond example scope, it becomes its own project. The godom example is the seed.

## Database: SQLite

All candidates need persistence. The choice is simple: **SQLite** via Go (`modernc.org/sqlite` for pure Go, or `mattn/go-sqlite3` with CGo). Single file, no server, ships with the binary. Consistent with godom's single-binary philosophy.

SQLite handles surprisingly large workloads — it's good enough for most single-user and small-team apps. The app would demonstrate godom + SQLite as a full local application stack.

## Candidates

### Spreadsheet / Excel-like tool

A grid-based data tool with formulas, sorting, filtering.

**What it pushes in godom:**
- Massive DOM — hundreds of visible cells, thousands in the dataset, virtual scrolling needed
- Cell editing — inline input fields, focus management, tab/enter navigation between cells
- Formula engine — Go parses and evaluates formulas (`=SUM(A1:A10)`), recalculates dependents
- Copy/paste — clipboard integration with the browser
- Performance — updating one cell triggers recalculation and re-render of dependent cells

**Risk:** Virtual scrolling (rendering only visible rows) is the hardest part. godom's `g-for` renders the full list today. This would require either a windowing approach in Go (only send visible rows) or a new directive. High effort, but would solve a general problem for any godom app with large lists.

**SQLite use:** Each sheet is a table. Formulas could query across sheets.

---

### Accounting / Bookkeeping tool

Double-entry bookkeeping — chart of accounts, journal entries, ledger views, reports.

**What it pushes in godom:**
- Forms with validation — date pickers, account selectors, amount fields with debit/credit balancing
- Table views with filtering and sorting — transaction lists, ledger views
- Reports — balance sheet, income statement, trial balance, generated from Go queries against SQLite
- Multi-view navigation — accounts, transactions, reports as different views in one app

**Risk:** Moderate complexity, well-understood domain. The UI patterns (forms, tables, navigation) are exactly what godom needs to prove it can handle. No single killer challenge, but lots of accumulated polish needed.

**SQLite use:** Natural fit. Accounts table, transactions table, journal entries. SQL aggregation for reports.

---

### CRM app

Contact management, deal tracking, activity log, pipeline view.

**What it pushes in godom:**
- Kanban board — drag-and-drop cards across pipeline stages (proven mouse interaction from solar system)
- Search and filtering — quick search across contacts/deals, filter by status/tag/date
- Detail views — click a contact to see full profile, activity timeline, related deals
- Relationships — contacts linked to deals, deals linked to activities. Navigating between them.

**Risk:** Scope creep. CRMs are infinitely expandable. Need to define a tight MVP (contacts + deals + pipeline board) and stop there.

**SQLite use:** Contacts, deals, activities, pipeline stages. Foreign keys for relationships. Full-text search via SQLite FTS5.

---

### Complex calculator / engineering tool

A scientific or financial calculator with history, variables, unit conversion, graphing.

**What it pushes in godom:**
- Expression parsing in Go — mathematical expressions, variables, functions
- Real-time feedback — result updates as you type
- History and variables — reference previous results, define named variables
- Graphing — plot functions using an existing chart plugin (Chart.js, Plotly, or ECharts)

**Risk:** Lower complexity than the other options. Good as a demo but less likely to push godom to its limits. More of a "nice example" than a stress test.

**SQLite use:** Calculation history, saved variables, saved expressions.

---

### AI wrapper tool

A local UI for interacting with AI APIs — prompt management, conversation history, model comparison.

**What it pushes in godom:**
- Streaming responses — AI APIs return tokens incrementally, need to render them as they arrive
- Markdown rendering — AI responses contain markdown, code blocks, lists. Need a rendering plugin or Go-side HTML conversion.
- Conversation state — long chat histories with branching (edit a previous message, fork the conversation)
- Multiple panels — prompt editor, conversation view, settings, model selector side by side

**Risk:** Streaming text display is the interesting challenge. godom pushes full state snapshots — streaming token-by-token may need a different approach (append-only updates, or a dedicated text stream channel). This could expose a real gap in godom's model.

**SQLite use:** Conversations, messages, prompt templates, API key storage (encrypted).

---

### Service management / configuration agent

A tool for managing services, configurations, deployments — like a local control plane UI.

**What it pushes in godom:**
- System interaction — Go reads/writes config files, starts/stops services, monitors processes
- Real-time status — service health, resource usage, log tailing
- Forms for configuration — edit YAML/JSON configs through structured forms
- Dangerous actions — restart service, apply config. Confirmation dialogs, audit logging.

**Risk:** Largest scope by far. Highly system-specific. Better as a separate project from the start rather than a godom example.

**SQLite use:** Audit log, configuration history, service definitions.

## Recommendation

For a godom example that could grow into its own project, **accounting/bookkeeping** or **CRM** hit the sweet spot:

- **Accounting** is the more constrained domain — double-entry bookkeeping has clear rules, a well-defined data model, and a finite set of views. Less risk of scope creep. The UI patterns (forms, tables, reports, navigation) are exactly what a "can godom build real apps?" test needs to cover. It's also genuinely useful — plenty of people want simple local bookkeeping without cloud SaaS.

- **CRM** is more visually impressive (kanban board, drag-and-drop) and exercises godom's proven mouse interaction strengths. But the domain is less constrained and scope creep is a real risk.

Either way, the pattern is the same: start as a godom example, keep it focused, and spin it out into its own repo if it grows legs.

## Status

Idea stage. Choosing the app is the first decision. The implementation would likely happen after godom has routing/navigation support (needed for multi-view apps) and possibly after the form builder example proves out the form interaction patterns.
