package issues

import "github.com/metal-stack/metal-apiserver/pkg/db/metal"

const (
	TypeNoPartition Type = "no-partition"
)

type (
	issueNoPartition struct{}
)

func (i *issueNoPartition) Spec() *spec {
	return &spec{
		Type:        TypeNoPartition,
		Severity:    SeverityMajor,
		Description: "machine with no partition",
		RefURL:      "https://metal-stack.io/docs/troubleshooting/#no-partition",
	}
}

func (i *issueNoPartition) Evaluate(m *metal.Machine, ec *metal.ProvisioningEventContainer, c *Config) bool {
	return m.PartitionID == ""
}

func (i *issueNoPartition) Details() string {
	return ""
}
