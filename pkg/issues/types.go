package issues

import (
	"fmt"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
)

type (
	Type string
)

func AllIssueTypes() []Type {
	return []Type{
		TypeNoPartition,
		TypeLivelinessDead,
		TypeLivelinessUnknown,
		TypeLivelinessNotAvailable,
		TypeFailedMachineReclaim,
		TypeCrashLoop,
		TypeLastEventError,
		TypeBMCWithoutMAC,
		TypeBMCWithoutIP,
		TypeBMCInfoOutdated,
		TypeASNUniqueness,
		TypeNonDistinctBMCIP,
		TypeNoEventContainer,
	}
}

func ToAPIV2Type(issueType Type) (apiv2.MachineIssueType, error) {
	switch issueType {
	case TypeASNUniqueness:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_ASN_UNIQUENESS, nil
	case TypeBMCInfoOutdated:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_INFO_OUTDATED, nil
	case TypeBMCWithoutIP:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_WITHOUT_IP, nil
	case TypeBMCWithoutMAC:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_WITHOUT_MAC, nil
	case TypeCrashLoop:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_CRASH_LOOP, nil
	case TypeFailedMachineReclaim:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_FAILED_MACHINE_RECLAIM, nil
	case TypeLastEventError:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LAST_EVENT_ERROR, nil
	case TypeLivelinessDead:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_DEAD, nil
	case TypeLivelinessNotAvailable:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_NOT_AVAILABLE, nil
	case TypeLivelinessUnknown:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_UNKNOWN, nil
	case TypeNoEventContainer:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_NO_EVENT_CONTAINER, nil
	case TypeNoPartition:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_NO_PARTITION, nil
	case TypeNonDistinctBMCIP:
		return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_NON_DISTINCT_IP, nil
	}
	return apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_UNSPECIFIED, fmt.Errorf("unknown issue type: %s", issueType)
}

func FromAPIV2Type(issueType apiv2.MachineIssueType) (Type, error) {
	switch issueType {
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_ASN_UNIQUENESS:
		return TypeASNUniqueness, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_INFO_OUTDATED:
		return TypeBMCInfoOutdated, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_NON_DISTINCT_IP:
		return TypeNonDistinctBMCIP, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_WITHOUT_IP:
		return TypeBMCWithoutIP, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_BMC_WITHOUT_MAC:
		return TypeBMCWithoutMAC, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_CRASH_LOOP:
		return TypeCrashLoop, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_FAILED_MACHINE_RECLAIM:
		return TypeFailedMachineReclaim, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LAST_EVENT_ERROR:
		return TypeLastEventError, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_DEAD:
		return TypeLivelinessDead, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_NOT_AVAILABLE:
		return TypeLivelinessNotAvailable, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_LIVELINESS_UNKNOWN:
		return TypeLivelinessUnknown, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_NO_EVENT_CONTAINER:
		return TypeNoEventContainer, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_NO_PARTITION:
		return TypeNoPartition, nil
	case apiv2.MachineIssueType_MACHINE_ISSUE_TYPE_UNSPECIFIED:
		return "", fmt.Errorf("unknown issue type: %s", issueType)
	}
	return "", fmt.Errorf("unknown issue type: %s", issueType)
}

func NewIssueFromType(t Type) (issue, error) {
	switch t {
	case TypeNoPartition:
		return &issueNoPartition{}, nil
	case TypeLivelinessDead:
		return &issueLivelinessDead{}, nil
	case TypeLivelinessUnknown:
		return &issueLivelinessUnknown{}, nil
	case TypeLivelinessNotAvailable:
		return &issueLivelinessNotAvailable{}, nil
	case TypeFailedMachineReclaim:
		return &issueFailedMachineReclaim{}, nil
	case TypeCrashLoop:
		return &issueCrashLoop{}, nil
	case TypeLastEventError:
		return &issueLastEventError{}, nil
	case TypeBMCWithoutMAC:
		return &issueBMCWithoutMAC{}, nil
	case TypeBMCWithoutIP:
		return &issueBMCWithoutIP{}, nil
	case TypeBMCInfoOutdated:
		return &issueBMCInfoOutdated{}, nil
	case TypeASNUniqueness:
		return &issueASNUniqueness{}, nil
	case TypeNonDistinctBMCIP:
		return &issueNonDistinctBMCIP{}, nil
	case TypeNoEventContainer:
		return &issueNoEventContainer{}, nil
	default:
		return nil, fmt.Errorf("unknown issue type: %s", t)
	}
}
