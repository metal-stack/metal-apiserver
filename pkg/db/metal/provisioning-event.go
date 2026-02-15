package metal

import (
	"fmt"
	"time"

	"github.com/metal-stack/api/go/enum"
	infrav2 "github.com/metal-stack/api/go/metalstack/infra/v2"
)

type (
	ProvisioningEventType string
	ProvisioningEvents    []ProvisioningEvent

	ProvisioningEvent struct {
		Time    time.Time             `rethinkdb:"time"`
		Event   ProvisioningEventType `rethinkdb:"event"`
		Message string                `rethinkdb:"message"`
	}

	ProvisioningEventContainer struct {
		Base
		Liveliness           MachineLiveliness  `rethinkdb:"liveliness"`
		Events               ProvisioningEvents `rethinkdb:"events"`
		LastEventTime        *time.Time         `rethinkdb:"last_event_time"`
		LastErrorEvent       *ProvisioningEvent `rethinkdb:"last_error_event"`
		CrashLoop            bool               `rethinkdb:"crash_loop"`
		FailedMachineReclaim bool               `rethinkdb:"failed_machine_reclaim"`
	}
)

func (t ProvisioningEventType) String() string {
	return string(t)
}

// ProvisioningEventsByID creates a map of event provisioning containers with the id as the index.
func ProvisioningEventsByID(pecs []*ProvisioningEventContainer) map[string]*ProvisioningEventContainer {
	res := make(map[string]*ProvisioningEventContainer)
	for i, f := range pecs {
		res[f.ID] = pecs[i]
	}
	return res
}

const (
	ProvisioningEventAlive            = ProvisioningEventType("Alive")
	ProvisioningEventCrashed          = ProvisioningEventType("Crashed")
	ProvisioningEventPXEBooting       = ProvisioningEventType("PXE Booting")
	ProvisioningEventPlannedReboot    = ProvisioningEventType("Planned Reboot")
	ProvisioningEventPreparing        = ProvisioningEventType("Preparing")
	ProvisioningEventRegistering      = ProvisioningEventType("Registering")
	ProvisioningEventWaiting          = ProvisioningEventType("Waiting")
	ProvisioningEventInstalling       = ProvisioningEventType("Installing")
	ProvisioningEventBootingNewKernel = ProvisioningEventType("Booting New Kernel")
	ProvisioningEventPhonedHome       = ProvisioningEventType("Phoned Home")
	ProvisioningEventMachineReclaim   = ProvisioningEventType("Machine Reclaim")
)

var (
	AllProvisioningEventTypes = map[ProvisioningEventType]bool{
		ProvisioningEventAlive:            true,
		ProvisioningEventCrashed:          true,
		ProvisioningEventPlannedReboot:    true,
		ProvisioningEventPXEBooting:       true,
		ProvisioningEventPreparing:        true,
		ProvisioningEventRegistering:      true,
		ProvisioningEventWaiting:          true,
		ProvisioningEventInstalling:       true,
		ProvisioningEventBootingNewKernel: true,
		ProvisioningEventPhonedHome:       true,
		ProvisioningEventMachineReclaim:   true,
	}
)

func ToProvisioningEventType(t infrav2.ProvisioningEventType) (ProvisioningEventType, error) {
	strVal, err := enum.GetStringValue(t)
	if err != nil {
		return ProvisioningEventType(""), err
	}
	return ProvisioningEventType(*strVal), nil
}

func FromProvisioningEventType(t ProvisioningEventType) (infrav2.ProvisioningEventType, error) {
	infrav2Type, err := enum.GetEnum[infrav2.ProvisioningEventType](string(t))
	if err != nil {
		return infrav2.ProvisioningEventType_PROVISIONING_EVENT_TYPE_UNSPECIFIED, fmt.Errorf("provisioning event type %q is invalid", t)
	}
	return infrav2Type, nil
}

func (p *ProvisioningEventContainer) TrimEvents(maxCount int) {
	if len(p.Events) > maxCount {
		p.Events = p.Events[:maxCount]
	}
}

func (c *ProvisioningEventContainer) Validate() error {
	if c == nil || len(c.Events) == 0 {
		return nil
	}

	lastEventTime := c.Events[0].Time

	// LastEventTime field in container may be equal or later than the time of the last event
	// because some events will update the field but not be appended
	if c.LastEventTime == nil || lastEventTime.After(*c.LastEventTime) {
		return fmt.Errorf("last event time not up to date in provisioning event container for machine %s", c.ID)
	}

	for _, e := range c.Events {
		if e.Time.After(lastEventTime) {
			return fmt.Errorf("provisioning event container for machine %s is not chronologically sorted", c.ID)
		}

		lastEventTime = e.Time
	}

	return nil
}
