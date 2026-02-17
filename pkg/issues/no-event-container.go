package issues

import (
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
)

const (
	TypeNoEventContainer Type = "no-event-container"
)

type (
	issueNoEventContainer struct{}
)

func (i *issueNoEventContainer) Spec() *spec {
	return &spec{
		Type:        TypeNoEventContainer,
		Severity:    SeverityMajor,
		Description: "machine has no event container",
		RefURL:      "https://metal-stack.io/docs/troubleshooting/#no-event-container",
	}
}

func (i *issueNoEventContainer) Evaluate(m *metal.Machine, ec *metal.ProvisioningEventContainer, c *Config) bool {
	return ec.ID == ""
}

func (i *issueNoEventContainer) Details() string {
	return ""
}
