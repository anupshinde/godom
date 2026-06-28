package server

import (
	"reflect"
	"testing"

	"github.com/anupshinde/godom/internal/island"
	"github.com/anupshinde/godom/internal/vdom"
)

type computedApp struct {
	Island  struct{}
	Count   int
	Doubled int
}

const computedHTML = `<!DOCTYPE html><html><head></head><body><span g-text="Doubled"></span></body></html>`

func makeComputedCI(t *testing.T, app *computedApp) *island.Info {
	t.Helper()
	v := reflect.ValueOf(app)
	templates, err := vdom.ParseTemplate(computedHTML)
	if err != nil {
		t.Fatal(err)
	}
	ci := &island.Info{Value: v, Typ: v.Elem().Type(), VDOMTemplates: templates}
	if err := ci.RegisterComputed([]island.ComputedDef{
		{Name: "Doubled", Fn: func() any { return app.Count * 2 }, Deps: []string{"Count"}},
	}); err != nil {
		t.Fatal(err)
	}
	return ci
}

// executeRefresh must recompute computeds on BOTH paths: the surgical path (when
// the dependency is marked) and the full path (a bare refresh with no marks).
func TestExecuteRefresh_RecomputesComputeds(t *testing.T) {
	app := &computedApp{}
	ci := makeComputedCI(t, app)
	ci.IDCounter = &vdom.IDCounter{}
	BuildInit(ci)

	ctx := &serverCtx{
		pool:   &connPool{},
		sm:     &sharedPtrMaps{ptrToCompIdx: map[uintptr][]int{}, compIdxToPtr: map[int][]uintptr{}},
		lookup: newNodeLookup(),
		comps:  []*island.Info{ci},
	}

	// Surgical path: mark the dependency.
	app.Count = 5
	ci.AddMarkedFields("Count")
	ctx.executeRefresh(ci)
	if app.Doubled != 10 {
		t.Errorf("surgical executeRefresh did not recompute Doubled: got %d, want 10", app.Doubled)
	}

	// Full path: no marks → full refresh recomputes everything.
	app.Count = 7
	ctx.executeRefresh(ci)
	if app.Doubled != 14 {
		t.Errorf("full executeRefresh did not recompute Doubled: got %d, want 14", app.Doubled)
	}
}
