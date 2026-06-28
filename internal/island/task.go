package island

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
)

// Task is the handle passed to an async-task closure. The closure runs off the
// island event loop (so it may block) and must never touch island state
// directly; all state changes are marshaled back onto the loop via Apply, which
// serializes them with renders. Cancellation is cooperative via Cancelled /
// Context.
type Task struct {
	ci   *Info
	name string
	gen  uint64
	ctx  context.Context
}

// Context returns the task's context, cancelled when the task is superseded
// (WithRestart) or the island is torn down.
func (t *Task) Context() context.Context { return t.ctx }

// Cancelled reports whether the task has been superseded/cancelled. Long-running
// task bodies should check it periodically and return early when true.
func (t *Task) Cancelled() bool { return t.ctx.Err() != nil }

// Apply marshals a state mutation back onto the event loop. The closure runs on
// the loop (so it may safely read/write island fields and call MarkRefresh),
// after which the engine emits one refresh. Apply is fire-and-forget with
// ordering preserved; it does not block the task goroutine.
func (t *Task) Apply(fn func()) { t.enqueue(fn) }

// Progress sets the task's status string, surfaced via the Progress(name)
// binding.
func (t *Task) Progress(msg string) {
	name := t.name
	t.enqueue(func() { t.ci.setTaskProgress(name, msg) })
}

// Fail records an error for the task, surfaced via the Err(name) binding, and
// moves the task to a terminal (not-busy) state.
func (t *Task) Fail(err error) {
	name := t.name
	t.enqueue(func() { t.ci.failTask(name, err) })
}

func (t *Task) enqueue(fn func()) {
	if t.ci.EventCh == nil {
		return
	}
	t.ci.EventCh <- Event{Kind: ApplyKind, Apply: fn, TaskName: t.name, TaskGen: t.gen}
}

// TaskPanic wraps a recovered panic from a task body or one of its Apply
// closures. It is surfaced through the Err(name) binding as a distinguishable
// type and flagged by Crashed(name); the engine also always logs the stack.
type TaskPanic struct {
	Value any
	Stack []byte
}

func (e *TaskPanic) Error() string { return fmt.Sprintf("task panic: %v", e.Value) }

// taskPolicy is the re-entry policy when Task(name, …) is started while a task
// of the same name is already running.
type taskPolicy int

const (
	policyDrop    taskPolicy = iota // ignore the new start (default)
	policyRestart                   // cancel the running one and start fresh
	policyQueue                     // run the new one after the current finishes
)

type taskOpts struct{ policy taskPolicy }

// TaskOption configures a Task start.
type TaskOption func(*taskOpts)

// WithRestart cancels an in-flight task of the same name and starts a fresh run.
// The superseded run's late Applies are fenced out, so its results can never
// clobber the new run.
func WithRestart() TaskOption { return func(o *taskOpts) { o.policy = policyRestart } }

// WithQueue runs the new task after the current one of the same name finishes,
// instead of dropping it.
func WithQueue() TaskOption { return func(o *taskOpts) { o.policy = policyQueue } }

type queuedTask struct {
	fn   func(*Task)
	opts taskOpts
}

// taskState is the loop-owned state for one named task.
type taskState struct {
	gen      uint64             // current generation; bumped on each (re)start
	running  bool               // true between idle→running and running→terminal
	progress string             // latest Progress(msg)
	err      error              // set by Fail / panic; nil otherwise
	panicked bool               // true when err is a *TaskPanic
	cancel   context.CancelFunc // cancels the running goroutine's context
	queued   []queuedTask       // WithQueue backlog
}

// StartTask is invoked by the public Island.Task. It enqueues the start onto the
// event loop so that all task-registry access stays on the single loop
// goroutine — even when Task() is called from a background goroutine. A nil
// EventCh (not yet serving) makes it a no-op.
func (ci *Info) StartTask(name string, fn func(*Task), opts ...TaskOption) {
	if ci.EventCh == nil {
		return
	}
	o := taskOpts{}
	for _, opt := range opts {
		opt(&o)
	}
	ci.EventCh <- Event{Kind: ApplyKind, Apply: func() {
		ci.startTaskOnLoop(name, fn, o)
	}}
}

// startTaskOnLoop runs on the event loop and applies the re-entry policy.
func (ci *Info) startTaskOnLoop(name string, fn func(*Task), o taskOpts) {
	if ci.tasks == nil {
		ci.tasks = make(map[string]*taskState)
	}
	st := ci.tasks[name]
	if st != nil && st.running {
		switch o.policy {
		case policyDrop:
			return
		case policyRestart:
			if st.cancel != nil {
				st.cancel()
			}
		case policyQueue:
			st.queued = append(st.queued, queuedTask{fn: fn, opts: o})
			return
		}
	}
	if st == nil {
		st = &taskState{}
		ci.tasks[name] = st
	}
	ci.launchTask(name, st, fn)
}

// launchTask bumps the generation, resets state to a fresh running run, and
// starts the off-loop goroutine. Runs on the loop.
func (ci *Info) launchTask(name string, st *taskState, fn func(*Task)) {
	st.gen++
	st.running = true
	st.progress = ""
	st.err = nil
	st.panicked = false
	ctx, cancel := context.WithCancel(context.Background())
	st.cancel = cancel
	t := &Task{ci: ci, name: name, gen: st.gen, ctx: ctx}
	go ci.runTask(t, fn)
}

// runTask is the off-loop goroutine body. It recovers panics from the task and
// routes them to the Fail path (never the process-fatal recover), and always
// posts a terminal transition when the body returns.
func (ci *Info) runTask(t *Task, fn func(*Task)) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Printf("godom: task %q panicked: %v\n%s", t.name, r, stack)
			val := r
			t.enqueue(func() { t.ci.FailPanic(t.name, val, stack) })
			return
		}
		t.enqueue(func() { t.ci.doneTask(t.name) })
	}()
	fn(t)
}

// --- loop-owned state mutators (run inside an Apply on the event loop) ---

func (ci *Info) setTaskProgress(name, msg string) {
	if st := ci.tasks[name]; st != nil {
		st.progress = msg
	}
}

func (ci *Info) failTask(name string, err error) {
	if st := ci.tasks[name]; st != nil {
		st.running = false
		st.err = err
		_, st.panicked = err.(*TaskPanic)
	}
	ci.startNextQueued(name)
}

// FailPanic records a recovered panic as the task's terminal error. Exported so
// the server can route an Apply-closure panic through the same path. Runs on the
// loop.
func (ci *Info) FailPanic(name string, val any, stack []byte) {
	if st := ci.tasks[name]; st != nil {
		st.running = false
		st.err = &TaskPanic{Value: val, Stack: stack}
		st.panicked = true
	}
	ci.startNextQueued(name)
}

func (ci *Info) doneTask(name string) {
	if st := ci.tasks[name]; st != nil {
		st.running = false
	}
	ci.startNextQueued(name)
}

func (ci *Info) startNextQueued(name string) {
	st := ci.tasks[name]
	if st == nil || st.running || len(st.queued) == 0 {
		return
	}
	next := st.queued[0]
	st.queued = st.queued[1:]
	ci.launchTask(name, st, next.fn)
}

// --- generation fence + binding readers (called on the loop) ---

// TaskGenCurrent reports whether gen is the live generation for the named task.
// The server uses it to drop fenced (superseded) Applies before running them.
func (ci *Info) TaskGenCurrent(name string, gen uint64) bool {
	st := ci.tasks[name]
	return st != nil && st.gen == gen
}

// TaskBusy backs the Busy(name) binding.
func (ci *Info) TaskBusy(name string) bool {
	st := ci.tasks[name]
	return st != nil && st.running
}

// TaskProgress backs the Progress(name) binding.
func (ci *Info) TaskProgress(name string) string {
	if st := ci.tasks[name]; st != nil {
		return st.progress
	}
	return ""
}

// TaskErr backs the Err(name) binding.
func (ci *Info) TaskErr(name string) error {
	if st := ci.tasks[name]; st != nil {
		return st.err
	}
	return nil
}

// TaskCrashed backs the Crashed(name) binding: true when the task's error is a
// recovered panic rather than a clean Fail.
func (ci *Info) TaskCrashed(name string) bool {
	st := ci.tasks[name]
	return st != nil && st.panicked
}

// Names of the engine-provided task-state bindings. These are injected into the
// expression environment as ExtraEnv functions at render time (see the server's
// taskEnv), so they are valid in templates even though they are neither struct
// fields nor methods. Keeping them here as the single source of truth lets the
// template validator accept the same names the renderer resolves — the two
// cannot drift.
const (
	BindBusy     = "Busy"     // Busy(name) bool
	BindProgress = "Progress" // Progress(name) string
	BindErr      = "Err"      // Err(name) -> error text or ""
	BindCrashed  = "Crashed"  // Crashed(name) bool
)

// ReservedBindingNames is the set of engine-provided expression-function names.
var ReservedBindingNames = []string{BindBusy, BindProgress, BindErr, BindCrashed}

// IsReservedBinding reports whether name is an engine-provided binding function
// (a name the renderer supplies via ExtraEnv). The template validator uses it to
// accept these names instead of rejecting them as unknown methods.
func IsReservedBinding(name string) bool {
	for _, n := range ReservedBindingNames {
		if n == name {
			return true
		}
	}
	return false
}
