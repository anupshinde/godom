# Markdown Editor

A live markdown editor with side-by-side preview, built with godom.

## Features

- **Live preview** — markdown rendered in real-time as you type, powered by [goldmark](https://github.com/yuin/goldmark) on the Go side
- **Bidirectional scroll sync** — editor and preview panes stay in sync
- **Mobile responsive** — collapsible editor pane with mobile-friendly font sizes
- **Save/load** — persist edits to `source-modified.md` with toast feedback

## When JavaScript is still the better choice

Godom's goal is to let you write GUI apps in Go without touching JavaScript. This example achieves that — all state, logic, and rendering lives in Go.

However, **scroll synchronization** is one area where a plain `<script>` tag would be simpler and more natural:

```html
<script>
    var ta = document.querySelector('textarea');
    var pane = document.querySelector('.preview').parentElement;
    ta.addEventListener('scroll', function() {
        var pct = ta.scrollTop / (ta.scrollHeight - ta.clientHeight || 1);
        pane.scrollTop = pct * (pane.scrollHeight - pane.clientHeight);
    });
</script>
```

This is 6 lines of JS that run entirely in the browser with zero latency. The godom approach (`g-scroll` event → Go handler → `g-prop:_scrollratio` → bridge applies scroll position) works, but it round-trips through WebSocket and requires a special bridge convention (`_scrollratio`) to convert ratios to pixels — because only the browser knows the element's live dimensions.

**Use JavaScript when the task is purely browser-side DOM manipulation that doesn't need Go state.** Scroll sync, CSS animations, focus management, and similar UI micro-interactions are good candidates. You can always mix a `<script>` tag into your godom template — they coexist without conflict.
