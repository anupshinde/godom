// Async tasks — managed background work, safe by construction.
//
// A "fruit search" backed by a deliberately slow lookup. Compare this with the
// old pattern (see examples/sync-demo): there you keep a `Processing bool`, a
// hand-rolled `if Processing { return }` re-entry guard, a raw goroutine, and
// manual Refresh() calls. Here, Task() handles all of that:
//
//   - the closure runs OFF the event loop (so it may block), and never touches
//     island fields directly — state changes go back through t.Apply;
//   - pending/progress bind without an app field via Busy()/Progress();
//   - WithRestart() cancels an in-flight search when a new one starts.
package main

import (
	"embed"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anupshinde/godom"
)

//go:embed ui
var ui embed.FS

type App struct {
	godom.Island
	Query   string
	Results []string
}

// A tiny fixed "catalog" to search — stands in for a slow database/API call.
var catalog = []string{
	"apple", "apricot", "banana", "blackberry", "blueberry", "cherry",
	"cranberry", "date", "elderberry", "fig", "grape", "grapefruit",
	"kiwi", "lemon", "lime", "mango", "melon", "orange", "papaya",
	"peach", "pear", "pineapple", "plum", "raspberry", "strawberry", "watermelon",
}

// Search starts (or restarts) the "search" task. It runs on the event loop (it's
// a g-click handler), so it safely reads Query here and hands the task only a
// captured local — the task closure must not touch island fields directly.
func (a *App) Search() {
	query := strings.ToLower(strings.TrimSpace(a.Query))

	a.Task("search", func(t *godom.Task) {
		// Simulate a slow backend, reporting progress as we go.
		for i := 1; i <= 3; i++ {
			if t.Cancelled() {
				return
			}
			time.Sleep(350 * time.Millisecond)
			t.Progress(fmt.Sprintf("Searching… %d%%", i*100/3))
		}

		var hits []string
		for _, item := range catalog {
			if query == "" || strings.Contains(item, query) {
				hits = append(hits, item)
			}
		}

		// Back on the loop: publish the results and patch the bound nodes.
		t.Apply(func() {
			a.Results = hits
			a.MarkRefresh("Results")
		})
	}, godom.WithRestart()) // a new search cancels one already running
}

func main() {
	app := &App{}
	app.Template = "ui/index.html"

	eng := godom.NewEngine()
	eng.SetFS(ui)
	log.Fatal(eng.QuickServe(app))
}
