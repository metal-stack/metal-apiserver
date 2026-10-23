package fsm

import (
	"testing"

	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/metal-stack/metal-apiserver/pkg/fsm/states"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootEventsRestoreBootingState(t *testing.T) {
	for _, event := range []metal.ProvisioningEventType{metal.ProvisioningEventPXEBooting, metal.ProvisioningEventISOBooting} {
		t.Run(event.String(), func(t *testing.T) {
			container := &metal.ProvisioningEventContainer{
				Events: metal.ProvisioningEvents{{Event: event}},
			}
			assert.Equal(t, states.Booting.String(), initialStateFromEventContainer(container))
		})
	}
}

func TestEventsProperlyDefined(t *testing.T) {
	events := Events()
	allStates := states.AllStates(&states.StateConfig{})
	allStates[SelfTransitionState] = nil

	for _, e := range events {
		require.NotEmpty(t, e.Dst)
		assert.Contains(t, allStates, e.Dst)
		assert.NotEmpty(t, e.Src)
		assert.NotEmpty(t, e.Name)
	}
}
