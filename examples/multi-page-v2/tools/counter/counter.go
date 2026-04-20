package counter

import "github.com/anupshinde/godom"

// State is the shared counter state. Multiple islands embed *State to observe
// and display the counter. Modifying State from Counter auto-refreshes any
// other island that embeds the same pointer (godom's shared-pointer refresh).
type State struct {
	Count int
	Step  int
}

type Counter struct {
	godom.Island
	*State
}

func (c *Counter) Increment() { c.Count += c.Step }
func (c *Counter) Decrement() { c.Count -= c.Step }

// New takes an optional shared *State. If nil, Counter gets its own.
func New(s *State) *Counter {
	if s == nil {
		s = &State{Step: 1}
	}
	return &Counter{
		Island: godom.Island{
			TargetName: "counter",
			Template:   "island-templates/counter/index.html",
		},
		State: s,
	}
}
