package island

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// startLoop runs a minimal event loop that mirrors the server's ApplyKind
// dispatch — generation fence + panic-recovery into the task's Fail path — so
// the island-side task state machine can be exercised in isolation.
func startLoop(ci *Info) (stop func()) {
	done := make(chan struct{})
	go func() {
		for evt := range ci.EventCh {
			if evt.Kind != ApplyKind || evt.Apply == nil {
				continue
			}
			if evt.TaskName != "" && !ci.TaskGenCurrent(evt.TaskName, evt.TaskGen) {
				continue // fenced: superseded run's apply
			}
			func() {
				defer func() {
					if r := recover(); r != nil && evt.TaskName != "" {
						ci.FailPanic(evt.TaskName, r, []byte("stack"))
					}
				}()
				evt.Apply()
			}()
		}
		close(done)
	}()
	return func() { close(ci.EventCh); <-done }
}

func snap[T any](ci *Info, read func() T) T {
	var v T
	done := make(chan struct{})
	ci.EventCh <- Event{Kind: ApplyKind, Apply: func() { v = read(); close(done) }}
	<-done
	return v
}

func waitBusy(t *testing.T, ci *Info, name string, want bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if snap(ci, func() bool { return ci.TaskBusy(name) }) == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for Busy(%q)==%v", name, want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func newLoopInfo() *Info { return &Info{EventCh: make(chan Event, 64)} }

func TestTask_ApplyAndCleanCompletion(t *testing.T) {
	ci := newLoopInfo()
	stop := startLoop(ci)
	defer stop()

	var value int
	release := make(chan struct{})
	ci.StartTask("w", func(tk *Task) {
		<-release
		tk.Apply(func() { value = 7 })
	})
	waitBusy(t, ci, "w", true)
	close(release)
	waitBusy(t, ci, "w", false)

	got := snap(ci, func() [3]any { return [3]any{value, ci.TaskErr("w"), ci.TaskCrashed("w")} })
	if got[0] != 7 {
		t.Errorf("Apply did not run: value=%v", got[0])
	}
	if got[1] != nil || got[2] != false {
		t.Errorf("clean completion: Err=%v Crashed=%v, want nil/false", got[1], got[2])
	}
}

func TestTask_DropPolicy(t *testing.T) {
	ci := newLoopInfo()
	stop := startLoop(ci)
	defer stop()

	var bodies atomic.Int32
	release := make(chan struct{})
	body := func(tk *Task) { bodies.Add(1); <-release }

	ci.StartTask("d", body)
	waitBusy(t, ci, "d", true)
	ci.StartTask("d", body) // dropped
	ci.StartTask("d", body) // dropped
	snap(ci, func() any { return nil })
	close(release)
	waitBusy(t, ci, "d", false)

	if n := bodies.Load(); n != 1 {
		t.Errorf("drop ran %d bodies, want 1", n)
	}
}

func TestTask_RestartFencesStaleApplyAndCancels(t *testing.T) {
	ci := newLoopInfo()
	stop := startLoop(ci)
	defer stop()

	var value int32
	releaseOld := make(chan struct{})
	staleSent := make(chan struct{})
	var cancelled atomic.Bool

	ci.StartTask("j", func(tk *Task) {
		<-releaseOld
		cancelled.Store(tk.Cancelled())
		tk.Apply(func() { atomic.StoreInt32(&value, 111) }) // stale
		close(staleSent)
	})
	waitBusy(t, ci, "j", true)

	ci.StartTask("j", func(tk *Task) {
		tk.Apply(func() { atomic.StoreInt32(&value, 222) })
	}, WithRestart())
	waitBusy(t, ci, "j", false)
	if got := snap(ci, func() int32 { return atomic.LoadInt32(&value) }); got != 222 {
		t.Fatalf("fresh run should set 222, got %d", got)
	}

	close(releaseOld)
	<-staleSent
	if got := snap(ci, func() int32 { return atomic.LoadInt32(&value) }); got != 222 {
		t.Errorf("stale apply clobbered state: %d, want 222", got)
	}
	if !cancelled.Load() {
		t.Errorf("superseded run should see Cancelled()==true")
	}
}

func TestTask_QueuePolicyRunsAfterCurrent(t *testing.T) {
	ci := newLoopInfo()
	stop := startLoop(ci)
	defer stop()

	var order []int // mutated only inside Applies, i.e. on the loop
	release1 := make(chan struct{})

	ci.StartTask("q", func(tk *Task) {
		<-release1
		tk.Apply(func() { order = append(order, 1) })
	})
	waitBusy(t, ci, "q", true)
	ci.StartTask("q", func(tk *Task) {
		tk.Apply(func() { order = append(order, 2) })
	}, WithQueue())

	close(release1)
	// Wait until both ran (queued task launches after the first finishes).
	deadline := time.Now().Add(2 * time.Second)
	for snap(ci, func() int { return len(order) }) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("queued task did not run")
		}
		time.Sleep(2 * time.Millisecond)
	}
	got := snap(ci, func() []int { return append([]int(nil), order...) })
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("queue order = %v, want [1 2]", got)
	}
}

func TestTask_BodyPanicBecomesCrash(t *testing.T) {
	ci := newLoopInfo()
	stop := startLoop(ci)
	defer stop()

	ci.StartTask("boom", func(tk *Task) { panic("x") })
	waitBusy(t, ci, "boom", false)

	got := snap(ci, func() [2]any { return [2]any{ci.TaskCrashed("boom"), ci.TaskErr("boom")} })
	if got[0] != true {
		t.Errorf("body panic should set Crashed")
	}
	tp, ok := got[1].(*TaskPanic)
	if !ok || len(tp.Stack) == 0 || tp.Error() == "" {
		t.Errorf("Err should be a *TaskPanic with a stack and message, got %v", got[1])
	}
}

func TestTask_FailSetsErrWithoutCrash(t *testing.T) {
	ci := newLoopInfo()
	stop := startLoop(ci)
	defer stop()

	ci.StartTask("f", func(tk *Task) { tk.Fail(errors.New("bad")) })
	waitBusy(t, ci, "f", false)

	got := snap(ci, func() [2]any { return [2]any{ci.TaskErr("f"), ci.TaskCrashed("f")} })
	if got[0] == nil || got[0].(error).Error() != "bad" || got[1] != false {
		t.Errorf("Fail: Err=%v Crashed=%v, want bad/false", got[0], got[1])
	}
}

func TestTask_Progress(t *testing.T) {
	ci := newLoopInfo()
	stop := startLoop(ci)
	defer stop()

	release := make(chan struct{})
	ci.StartTask("p", func(tk *Task) { tk.Progress("50%"); <-release })
	deadline := time.Now().Add(2 * time.Second)
	for snap(ci, func() string { return ci.TaskProgress("p") }) != "50%" {
		if time.Now().After(deadline) {
			t.Fatal("Progress not set")
		}
		time.Sleep(2 * time.Millisecond)
	}
	close(release)
	waitBusy(t, ci, "p", false)
}

// Task methods and StartTask are safe no-ops before the island is serving (nil
// EventCh) — they must not panic or block.
func TestTask_NilEventChNoOp(t *testing.T) {
	ci := &Info{} // no EventCh
	ci.StartTask("x", func(tk *Task) { t.Error("body should never run") })
	tk := &Task{ci: ci, name: "x"}
	tk.Apply(func() { t.Error("apply should not enqueue") })
	tk.Progress("nope")
	tk.Fail(errors.New("nope"))
	if ci.TaskBusy("x") || ci.TaskProgress("x") != "" || ci.TaskErr("x") != nil || ci.TaskCrashed("x") {
		t.Errorf("no task should exist for a nil-EventCh island")
	}
}

// A burst of Progress calls must coalesce to a single queued apply (not one per
// call) and apply the latest value — otherwise a hot status loop re-renders once
// per tick. This drains the channel manually (no loop) to count applies directly.
func TestTask_ProgressCoalescesLatestWins(t *testing.T) {
	ci := &Info{EventCh: make(chan Event, 256)}
	ci.tasks = map[string]*taskState{"p": {gen: 1, running: true}}
	tk := &Task{ci: ci, name: "p", gen: 1, ctx: context.Background()}

	const N = 100
	for i := 0; i < N; i++ {
		tk.Progress(itoa(i)) // no drain happening, so all but the first should coalesce
	}

	// Count and run the queued applies.
	applies := 0
	for {
		select {
		case e := <-ci.EventCh:
			applies++
			if e.Apply != nil {
				e.Apply()
			}
		default:
			goto drained
		}
	}
drained:
	if applies == 0 || applies >= N {
		t.Fatalf("expected Progress to coalesce to far fewer than %d applies, got %d", N, applies)
	}
	if got := ci.TaskProgress("p"); got != itoa(N-1) {
		t.Errorf("coalesced progress should be the latest value %q, got %q", itoa(N-1), got)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
