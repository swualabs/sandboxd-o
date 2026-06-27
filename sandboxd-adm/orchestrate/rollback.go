package orchestrate

import (
	"context"
	"slices"
	"time"

	"sandboxd-o/sandboxd-adm/stepper"
)

type rollbackAction struct {
	desc string
	fn   func(ctx context.Context) error
}

// rollbackStack runs cleanup actions LIFO; call clear() once the operation
// has fully succeeded.
type rollbackStack struct {
	actions []rollbackAction
	cleared bool
}

func (r *rollbackStack) add(desc string, fn func(ctx context.Context) error) {
	r.actions = append(r.actions, rollbackAction{desc: desc, fn: fn})
}

func (r *rollbackStack) clear() {
	r.cleared = true
}

func (r *rollbackStack) run(s *stepper.Stepper) {
	if r.cleared || len(r.actions) == 0 {
		return
	}

	s.Step("rolling back partially created resources")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	for _, a := range slices.Backward(r.actions) {

		if err := a.fn(ctx); err != nil {
			s.Warn("rollback step failed (%s): %v", a.desc, err)
			continue
		}
		s.Done("rolled back: %s", a.desc)
	}
}
