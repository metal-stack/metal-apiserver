package states

import (
	"context"

	"github.com/looplab/fsm"
)

type BootingState struct {
	*FSMState
}

func newBooting(c *StateConfig) *BootingState {
	return &BootingState{
		FSMState: &FSMState{
			container: c.Container,
			event:     c.Event,
			log:       c.Log,
		},
	}
}

func (p *BootingState) OnTransition(ctx context.Context, e *fsm.Event) {
	if p.swallowBufferedPhonedHome(e) {
		return
	}
	p.container.FailedMachineReclaim = false

	if e.Src == Booting.String() && len(p.container.Events) > 0 && p.container.Events[0].Event == p.event.Event {
		updateTimeAndLiveliness(p.event, p.container)
		return
	}

	appendEventToContainer(p.event, p.container)
}
