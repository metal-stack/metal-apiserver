package issues

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/metal-stack/metal-apiserver/pkg/db/metal"
	"github.com/stretchr/testify/require"
)

func TestFindIssues(t *testing.T) {
	machineTemplate := func(id string) *metal.Machine {
		return &metal.Machine{
			Base: metal.Base{
				ID: id,
			},
			PartitionID: "a",
			IPMI: metal.IPMI{
				Address:     "1.2.3.4",
				MacAddress:  "aa:bb:00",
				LastUpdated: time.Now().Add(-1 * time.Minute),
			},
		}
	}
	eventContainerTemplate := func(id string) *metal.ProvisioningEventContainer {
		return &metal.ProvisioningEventContainer{
			Base: metal.Base{
				ID: id,
			},
			Liveliness: metal.MachineLivelinessAlive,
		}
	}

	tests := []struct {
		name string
		only []Type

		machines        func() []*metal.Machine
		eventContainers func() []*metal.ProvisioningEventContainer

		want func(machines []*metal.Machine) MachineIssues
	}{
		{
			name: "good machine has no issues",
			machines: func() []*metal.Machine {
				return []*metal.Machine{
					machineTemplate("good"),
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				return []*metal.ProvisioningEventContainer{
					eventContainerTemplate("good"),
				}
			},
			want: nil,
		},
		{
			name: "no partition",
			only: []Type{TypeNoPartition},
			machines: func() []*metal.Machine {
				noPartitionMachine := machineTemplate("no-partition")
				noPartitionMachine.PartitionID = ""

				return []*metal.Machine{
					noPartitionMachine,
					machineTemplate("good"),
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				return []*metal.ProvisioningEventContainer{
					eventContainerTemplate("no-partition"),
					eventContainerTemplate("good"),
				}
			},
			want: func(machines []*metal.Machine) MachineIssues {
				return MachineIssues{
					{
						Machine: machines[0],
						Issues: Issues{
							toIssue(&issueNoPartition{}),
						},
					},
				}
			},
		},
		{
			name: "liveliness dead",
			only: []Type{TypeLivelinessDead},
			machines: func() []*metal.Machine {
				return []*metal.Machine{
					machineTemplate("dead"),
					machineTemplate("good"),
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				dead := eventContainerTemplate("dead")
				dead.Liveliness = metal.MachineLivelinessDead

				return []*metal.ProvisioningEventContainer{
					dead,
					eventContainerTemplate("good"),
				}
			},
			want: func(machines []*metal.Machine) MachineIssues {
				return MachineIssues{
					{
						Machine: machines[0],
						Issues: Issues{
							toIssue(&issueLivelinessDead{}),
						},
					},
				}
			},
		},
		{
			name: "liveliness unknown",
			only: []Type{TypeLivelinessUnknown},
			machines: func() []*metal.Machine {
				return []*metal.Machine{
					machineTemplate("unknown"),
					machineTemplate("good"),
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				unknown := eventContainerTemplate("unknown")
				unknown.Liveliness = metal.MachineLivelinessUnknown

				return []*metal.ProvisioningEventContainer{
					unknown,
					eventContainerTemplate("good"),
				}
			},
			want: func(machines []*metal.Machine) MachineIssues {
				return MachineIssues{
					{
						Machine: machines[0],
						Issues: Issues{
							toIssue(&issueLivelinessUnknown{}),
						},
					},
				}
			},
		},
		{
			name: "liveliness not available",
			only: []Type{TypeLivelinessNotAvailable},
			machines: func() []*metal.Machine {
				return []*metal.Machine{
					machineTemplate("n/a"),
					machineTemplate("good"),
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				na := eventContainerTemplate("n/a")
				na.Liveliness = metal.MachineLiveliness("")

				return []*metal.ProvisioningEventContainer{
					na,
					eventContainerTemplate("good"),
				}
			},
			want: func(machines []*metal.Machine) MachineIssues {
				return MachineIssues{
					{
						Machine: machines[0],
						Issues: Issues{
							toIssue(&issueLivelinessNotAvailable{}),
						},
					},
				}
			},
		},
		{
			name: "failed machine reclaim",
			only: []Type{TypeFailedMachineReclaim},
			machines: func() []*metal.Machine {
				failedOld := machineTemplate("failed-old")

				return []*metal.Machine{
					machineTemplate("good"),
					machineTemplate("failed"),
					failedOld,
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				failed := eventContainerTemplate("failed")
				failed.FailedMachineReclaim = true

				failedOld := eventContainerTemplate("failed-old")
				failedOld.Events = metal.ProvisioningEvents{
					{
						Event: metal.ProvisioningEventPhonedHome,
					},
				}

				return []*metal.ProvisioningEventContainer{
					failed,
					eventContainerTemplate("good"),
					failedOld,
				}
			},
			want: func(machines []*metal.Machine) MachineIssues {
				return MachineIssues{
					{
						Machine: machines[1],
						Issues: Issues{
							toIssue(&issueFailedMachineReclaim{}),
						},
					},
					{
						Machine: machines[2],
						Issues: Issues{
							toIssue(&issueFailedMachineReclaim{}),
						},
					},
				}
			},
		},
		{
			name: "crashloop",
			only: []Type{TypeCrashLoop},
			machines: func() []*metal.Machine {
				return []*metal.Machine{
					machineTemplate("good"),
					machineTemplate("crash"),
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				crash := eventContainerTemplate("crash")
				crash.CrashLoop = true

				return []*metal.ProvisioningEventContainer{
					crash,
					eventContainerTemplate("good"),
				}
			},
			want: func(machines []*metal.Machine) MachineIssues {
				return MachineIssues{
					{
						Machine: machines[1],
						Issues: Issues{
							toIssue(&issueCrashLoop{}),
						},
					},
				}
			},
		},
		// FIXME:
		// {
		// 	name: "last event error",
		// 	only: []IssueType{IssueTypeLastEventError},
		// 	machines: func() []*metal.Machine {
		// 		lastEventErrorMachine := machineTemplate("last")

		// 		return []*metal.Machine{
		// 			machineTemplate("good"),
		// 			lastEventErrorMachine,
		// 		}
		// 	},
		// 	eventContainers: func() []*metal.ProvisioningEventContainer {
		// 		last := eventContainerTemplate("last")
		// 		last.LastErrorEvent = &metal.ProvisioningEvent{
		// 			Time: time.Now().Add(-5 * time.Minute),
		// 		}
		// 		return []*metal.ProvisioningEventContainer{
		// 			last,
		// 			eventContainerTemplate("good"),
		// 		}
		// 	},
		// 	want: func(machines []*metal.Machine) MachineIssues {
		// 		return MachineIssues{
		// 			{
		// 				Machine: &machines[1],
		// 				Issues: Issues{
		// 					toIssue(&IssueLastEventError{details: "occurred 5m0s ago"}),
		// 				},
		// 			},
		// 		}
		// 	},
		// },
		{
			name: "bmc without mac",
			only: []Type{TypeBMCWithoutMAC},
			machines: func() []*metal.Machine {
				noMac := machineTemplate("no-mac")
				noMac.IPMI.MacAddress = ""

				return []*metal.Machine{
					machineTemplate("good"),
					noMac,
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				crash := eventContainerTemplate("crash")
				crash.CrashLoop = true

				return []*metal.ProvisioningEventContainer{
					eventContainerTemplate("no-mac"),
					eventContainerTemplate("good"),
				}
			},
			want: func(machines []*metal.Machine) MachineIssues {
				return MachineIssues{
					{
						Machine: machines[1],
						Issues: Issues{
							toIssue(&issueBMCWithoutMAC{}),
						},
					},
				}
			},
		},
		{
			name: "bmc without ip",
			only: []Type{TypeBMCWithoutIP},
			machines: func() []*metal.Machine {
				noIP := machineTemplate("no-ip")
				noIP.IPMI.Address = ""

				return []*metal.Machine{
					machineTemplate("good"),
					noIP,
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				crash := eventContainerTemplate("crash")
				crash.CrashLoop = true

				return []*metal.ProvisioningEventContainer{
					eventContainerTemplate("no-ip"),
					eventContainerTemplate("good"),
				}
			},
			want: func(machines []*metal.Machine) MachineIssues {
				return MachineIssues{
					{
						Machine: machines[1],
						Issues: Issues{
							toIssue(&issueBMCWithoutIP{}),
						},
					},
				}
			},
		},
		// FIXME:
		// {
		// 	name: "bmc info outdated",
		// 	only: []IssueType{IssueTypeBMCInfoOutdated},
		// 	machines: func() []*metal.Machine {
		// 		outdated := machineTemplate("outdated")
		// 		outdated.IPMI.LastUpdated = time.Now().Add(-3 * 60 * time.Minute)

		// 		return []*metal.Machine{
		// 			machineTemplate("good"),
		// 			outdated,
		// 		}
		// 	},
		// 	eventContainers: func() []*metal.ProvisioningEventContainer {
		// 		return []*metal.ProvisioningEventContainer{
		// 			eventContainerTemplate("outdated"),
		// 			eventContainerTemplate("good"),
		// 		}
		// 	},
		// 	want: func(machines []*metal.Machine) MachineIssues {
		// 		return MachineIssues{
		// 			{
		// 				Machine: &machines[1],
		// 				Issues: Issues{
		// 					toIssue(&IssueBMCInfoOutdated{
		// 						details: "last updated 3h0m0s ago",
		// 					}),
		// 				},
		// 			},
		// 		}
		// 	},
		// },
		{
			name: "asn shared",
			only: []Type{TypeASNUniqueness},
			machines: func() []*metal.Machine {
				shared1 := machineTemplate("shared1")
				shared1.Allocation = &metal.MachineAllocation{
					Role: metal.RoleFirewall,
					MachineNetworks: []*metal.MachineNetwork{
						{
							ASN: 0,
						},
						{
							ASN: 100,
						},
						{
							ASN: 200,
						},
					},
				}

				shared2 := machineTemplate("shared2")
				shared2.Allocation = &metal.MachineAllocation{
					Role: metal.RoleFirewall,
					MachineNetworks: []*metal.MachineNetwork{
						{
							ASN: 1,
						},
						{
							ASN: 100,
						},
						{
							ASN: 200,
						},
					},
				}

				return []*metal.Machine{
					shared1,
					shared2,
					machineTemplate("good"),
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				return []*metal.ProvisioningEventContainer{
					eventContainerTemplate("shared1"),
					eventContainerTemplate("shared2"),
					eventContainerTemplate("good"),
				}
			},
			want: func(machines []*metal.Machine) MachineIssues {
				return MachineIssues{
					{
						Machine: machines[0],
						Issues: Issues{
							toIssue(&issueASNUniqueness{
								details: fmt.Sprintf("- ASN (100) not unique, shared with [%[1]s]\n- ASN (200) not unique, shared with [%[1]s]", machines[1].ID),
							}),
						},
					},
					{
						Machine: machines[1],
						Issues: Issues{
							toIssue(&issueASNUniqueness{
								details: fmt.Sprintf("- ASN (100) not unique, shared with [%[1]s]\n- ASN (200) not unique, shared with [%[1]s]", machines[0].ID),
							}),
						},
					},
				}
			},
		},
		{
			name: "non distinct bmc ip",
			only: []Type{TypeNonDistinctBMCIP},
			machines: func() []*metal.Machine {
				bmc1 := machineTemplate("bmc1")
				bmc1.IPMI.Address = "127.0.0.1"

				bmc2 := machineTemplate("bmc2")
				bmc2.IPMI.Address = "127.0.0.1"

				return []*metal.Machine{
					bmc1,
					bmc2,
					machineTemplate("good"),
				}
			},
			eventContainers: func() []*metal.ProvisioningEventContainer {
				return []*metal.ProvisioningEventContainer{
					eventContainerTemplate("bmc1"),
					eventContainerTemplate("bmc2"),
					eventContainerTemplate("good"),
				}
			},
			want: func(machines []*metal.Machine) MachineIssues {
				return MachineIssues{
					{
						Machine: machines[0],
						Issues: Issues{
							toIssue(&issueNonDistinctBMCIP{
								details: fmt.Sprintf("BMC IP (127.0.0.1) not unique, shared with [%[1]s]", machines[1].ID),
							}),
						},
					},
					{
						Machine: machines[1],
						Issues: Issues{
							toIssue(&issueNonDistinctBMCIP{
								details: fmt.Sprintf("BMC IP (127.0.0.1) not unique, shared with [%[1]s]", machines[0].ID),
							}),
						},
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := tt.machines()

			got, err := Find(&Config{
				Machines:           ms,
				EventContainers:    tt.eventContainers(),
				Only:               tt.only,
				LastErrorThreshold: DefaultLastErrorThreshold(),
			})
			require.NoError(t, err)

			var want MachineIssues
			if tt.want != nil {
				want = tt.want(ms)
			}

			if diff := cmp.Diff(want, got.ToList(), cmp.AllowUnexported(issueLastEventError{}, issueASNUniqueness{}, issueNonDistinctBMCIP{})); diff != "" {
				t.Errorf("diff (+got -want):\n %s", diff)
			}
		})
	}
}

func TestAllIssues(t *testing.T) {
	issuesTypes := map[Type]bool{}
	for _, i := range All() {
		issuesTypes[i.Type] = true
	}

	for _, ty := range AllIssueTypes() {
		if _, ok := issuesTypes[ty]; !ok {
			t.Errorf("issue of type %s not contained in all issues", ty)
		}
	}
}
