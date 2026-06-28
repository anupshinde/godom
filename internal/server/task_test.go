package server

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anupshinde/godom/internal/island"
	"github.com/anupshinde/godom/internal/vdom"
)

// runTaskLoop builds a counter island with a running processEvents loop and
// returns the island plus a stop func. The loop is a real one, so tasks exercise
// the genuine ApplyKind dispatch, generation fence, and refresh path.
func runTaskLoop(t *testing.T) (*island.Info, func()) {
	t.Helper()
	ci := makeCounterCI(&counterApp{Step: 1})
	ci.HTMLBody = counterHTML
	ci.EventCh = make(chan island.Event, 64)
	ci.IDCounter = &vdom.IDCounter{}
	BuildInit(ci)

	ctx := &serverCtx{
		pool:   &connPool{},
		sm:     &sharedPtrMaps{ptrToCompIdx: map[uintptr][]int{}, compIdxToPtr: map[int][]uintptr{}},
		lookup: newNodeLookup(),
		comps:  []*island.Info{ci},
	}
	done := make(chan struct{})
	go func() { ctx.processEvents(ci, 0); close(done) }()

	return ci, func() { close(ci.EventCh); <-done }
}

// snapshotOnLoop runs read on the event loop and returns its result, so
// loop-owned task state can be observed without racing the processor. Because
// the event queue is FIFO, it also observes state after every previously
// enqueued apply has been processed.
func snapshotOnLoop[T any](ci *island.Info, read func() T) T {
	var v T
	done := make(chan struct{})
	ci.EventCh <- island.Event{Kind: island.ApplyKind, Apply: func() { v = read(); close(done) }}
	<-done
	return v
}

func waitBusy(t *testing.T, ci *island.Info, name string, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if snapshotOnLoop(ci, func() bool { return ci.TaskBusy(name) }) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for Busy(%q)==%v", name, want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// A task's Apply must run on the loop and mutate island state, and the task must
// be Busy while running and not Busy (cleanly, no error) once it finishes.
func TestTask_ApplyMutatesStateAndClearsBusy(t *testing.T) {
	ci, stop := runTaskLoop(t)
	defer stop()
	app := ci.Value.Interface().(*counterApp)

	release := make(chan struct{})
	ci.StartTask("work", func(tk *island.Task) {
		<-release // hold the task running so we can observe Busy==true
		tk.Apply(func() { app.Count = 42 })
	})

	waitBusy(t, ci, "work", true) // idle→running set Busy before the body even ran
	close(release)
	waitBusy(t, ci, "work", false)

	snap := snapshotOnLoop(ci, func() [3]any {
		return [3]any{app.Count, ci.TaskErr("work"), ci.TaskCrashed("work")}
	})
	if snap[0] != 42 {
		t.Errorf("Apply did not run on the loop: Count=%v, want 42", snap[0])
	}
	if snap[1] != nil {
		t.Errorf("clean completion should leave Err nil, got %v", snap[1])
	}
	if snap[2] != false {
		t.Errorf("clean completion should leave Crashed false")
	}
}

// The default re-entry policy drops a start while a task of the same name runs:
// exactly one goroutine body executes.
func TestTask_DropPolicyRunsOnlyOne(t *testing.T) {
	ci, stop := runTaskLoop(t)
	defer stop()

	var bodies atomic.Int32
	release := make(chan struct{})
	body := func(tk *island.Task) { bodies.Add(1); <-release }

	ci.StartTask("dup", body)
	waitBusy(t, ci, "dup", true)
	ci.StartTask("dup", body) // dropped — one already running
	ci.StartTask("dup", body) // dropped
	// Give the drops a chance to be (not) processed.
	snapshotOnLoop(ci, func() any { return nil })

	close(release)
	waitBusy(t, ci, "dup", false)

	if n := bodies.Load(); n != 1 {
		t.Errorf("drop policy ran %d task bodies, want 1", n)
	}
}

// Epoch fencing: WithRestart starts a fresh run, and the superseded run's late
// Apply is dropped — it must never clobber the new run's result. This is the
// core correctness property of the async-task design.
func TestTask_RestartFencesStaleApply(t *testing.T) {
	ci, stop := runTaskLoop(t)
	defer stop()
	app := ci.Value.Interface().(*counterApp)

	releaseOld := make(chan struct{})
	staleSent := make(chan struct{})
	var oldCancelled atomic.Bool

	// Run #1 — will be superseded. It blocks, then tries to write a stale value.
	ci.StartTask("job", func(tk *island.Task) {
		<-releaseOld
		oldCancelled.Store(tk.Cancelled())   // should be true after the restart
		tk.Apply(func() { app.Count = 111 }) // STALE write — must be fenced out
		close(staleSent)
	})
	waitBusy(t, ci, "job", true)

	// Run #2 — supersedes #1, writes the fresh value, completes.
	ci.StartTask("job", func(tk *island.Task) {
		tk.Apply(func() { app.Count = 222 })
	}, island.WithRestart())
	waitBusy(t, ci, "job", false) // #2 finished

	if got := snapshotOnLoop(ci, func() int { return app.Count }); got != 222 {
		t.Fatalf("after restart, fresh run should have set Count=222, got %d", got)
	}

	// Now let the stale run attempt its write; it must be dropped by the fence.
	close(releaseOld)
	<-staleSent // the stale Apply is now enqueued
	got := snapshotOnLoop(ci, func() int { return app.Count })
	if got != 222 {
		t.Errorf("stale Apply from superseded run clobbered state: Count=%d, want 222", got)
	}
	if !oldCancelled.Load() {
		t.Errorf("superseded run should observe Cancelled()==true")
	}
}

// A panic in the task body is recovered and surfaced as a crash (distinguishable
// from a clean Fail), Busy is cleared, and the process is NOT killed.
func TestTask_PanicRecoveredAsCrashed(t *testing.T) {
	ci, stop := runTaskLoop(t)
	defer stop()

	ci.StartTask("boom", func(tk *island.Task) {
		panic("kaboom")
	})
	waitBusy(t, ci, "boom", false)

	snap := snapshotOnLoop(ci, func() [2]any {
		return [2]any{ci.TaskCrashed("boom"), ci.TaskErr("boom")}
	})
	if snap[0] != true {
		t.Errorf("panicking task should report Crashed==true")
	}
	tp, ok := snap[1].(*island.TaskPanic)
	if !ok {
		t.Fatalf("Err should be *island.TaskPanic, got %T", snap[1])
	}
	if len(tp.Stack) == 0 {
		t.Errorf("TaskPanic should capture a stack")
	}
}

// A panic inside an Apply closure (on the loop) is also recovered into the task's
// crash state rather than reaching the process-fatal recover.
func TestTask_ApplyPanicRecoveredAsCrashed(t *testing.T) {
	ci, stop := runTaskLoop(t)
	defer stop()

	ci.StartTask("applypanic", func(tk *island.Task) {
		tk.Apply(func() { panic("in-apply") })
	})
	waitBusy(t, ci, "applypanic", false)

	if !snapshotOnLoop(ci, func() bool { return ci.TaskCrashed("applypanic") }) {
		t.Errorf("Apply-closure panic should crash the task, not the process")
	}
}

// Fail records an error surfaced via Err and ends the task without a crash flag.
func TestTask_FailSetsErrNotCrashed(t *testing.T) {
	ci, stop := runTaskLoop(t)
	defer stop()

	ci.StartTask("f", func(tk *island.Task) {
		tk.Fail(errors.New("nope"))
	})
	waitBusy(t, ci, "f", false)

	snap := snapshotOnLoop(ci, func() [2]any {
		return [2]any{ci.TaskErr("f"), ci.TaskCrashed("f")}
	})
	if snap[0] == nil || snap[0].(error).Error() != "nope" {
		t.Errorf("Fail should set Err, got %v", snap[0])
	}
	if snap[1] != false {
		t.Errorf("a clean Fail must not set Crashed")
	}
}

// Progress updates the per-task status read by the Progress(name) binding.
func TestTask_ProgressVisible(t *testing.T) {
	ci, stop := runTaskLoop(t)
	defer stop()

	release := make(chan struct{})
	ci.StartTask("p", func(tk *island.Task) {
		tk.Progress("halfway")
		<-release
	})
	// Wait until progress is observed, then release.
	deadline := time.Now().Add(2 * time.Second)
	for snapshotOnLoop(ci, func() string { return ci.TaskProgress("p") }) != "halfway" {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for Progress to be set")
		}
		time.Sleep(2 * time.Millisecond)
	}
	close(release)
	waitBusy(t, ci, "p", false)
}

// taskEnv (the renderer's bindings) and island.ReservedBindingNames (what the
// validator accepts) must be exactly the same set — otherwise a binding the
// renderer supplies could be rejected by the validator at startup, or vice versa.
func TestTaskEnv_MatchesReservedBindingNames(t *testing.T) {
	env := taskEnv(&island.Info{})
	if len(env) != len(island.ReservedBindingNames) {
		t.Fatalf("taskEnv has %d keys, ReservedBindingNames has %d", len(env), len(island.ReservedBindingNames))
	}
	for _, name := range island.ReservedBindingNames {
		if _, ok := env[name]; !ok {
			t.Errorf("ReservedBindingNames lists %q but taskEnv does not provide it", name)
		}
	}
	for name := range env {
		if !island.IsReservedBinding(name) {
			t.Errorf("taskEnv provides %q but it is not in ReservedBindingNames (validator would reject it)", name)
		}
	}
}
