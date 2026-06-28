package server

import (
	"log"
	"runtime/debug"

	"github.com/anupshinde/godom/internal/island"
)

// taskEnv returns the engine-provided task-state binding functions for an
// island, injected into the expression environment as ExtraEnv. They read
// loop-owned task state and are only ever evaluated during a render on the
// event loop, so they need no synchronization.
func taskEnv(ci *island.Info) map[string]any {
	// Keys come from island.Bind* — the same constants the template validator
	// checks via IsReservedBinding — so renderer and validator cannot drift.
	return map[string]any{
		island.BindBusy:     func(name string) bool { return ci.TaskBusy(name) },
		island.BindProgress: func(name string) string { return ci.TaskProgress(name) },
		island.BindCrashed:  func(name string) bool { return ci.TaskCrashed(name) },
		island.BindErr: func(name string) any {
			if e := ci.TaskErr(name); e != nil {
				return e.Error()
			}
			return ""
		},
	}
}

// runApply runs an ApplyKind closure on the event loop. For a task apply
// (TaskName set) it recovers panics and routes them to the task's Fail path, so
// a panicking task body or Apply fails the task — never the whole process.
// Unfenced applies (TaskName == "") are engine-internal (e.g. a task start); a
// panic there is a genuine bug and is left to the process-fatal recover.
func (s *serverCtx) runApply(ci *island.Info, evt island.Event) {
	if evt.Apply == nil {
		return
	}
	if evt.TaskName == "" {
		evt.Apply()
		return
	}
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Printf("godom: task %q apply panicked: %v\n%s", evt.TaskName, r, stack)
			ci.FailPanic(evt.TaskName, r, stack)
		}
	}()
	evt.Apply()
}
