package queries

import (
	"fmt"
	"strings"

	"github.com/metal-stack/api/go/enum"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/tag"
)

// IpFilter builds an in-memory filter for IP entities from the given query.
func IpFilter(rq *apiv2.IPQuery) func(map[string]any) bool {
	if rq == nil {
		return nil
	}
	return func(data map[string]any) bool {
		if rq.Project != nil {
			if data["ProjectID"] != *rq.Project {
				return false
			}
		}
		if rq.Ip != nil {
			if data["IPAddress"] != *rq.Ip {
				return false
			}
		}
		if rq.Uuid != nil {
			if data["AllocationUUID"] != *rq.Uuid {
				return false
			}
		}
		if rq.Name != nil {
			if data["Name"] != *rq.Name {
				return false
			}
		}
		if rq.Namespace != nil {
			v, _ := data["Namespace"].(*string)
			if v == nil || *v != *rq.Namespace {
				return false
			}
		}
		if rq.Network != nil {
			if data["NetworkID"] != *rq.Network {
				return false
			}
		}
		if rq.ParentPrefixCidr != nil {
			if data["ParentPrefixCidr"] != *rq.ParentPrefixCidr {
				return false
			}
		}
		if rq.Machine != nil {
			tagStr := fmt.Sprintf("%s=%s", tag.MachineID, *rq.Machine)
			tags, _ := data["Tags"].([]any)
			if !containsAny(tags, tagStr) {
				return false
			}
		}
		if rq.Labels != nil {
			tags, _ := data["Tags"].([]any)
			for key, value := range rq.Labels.Labels {
				tagStr := fmt.Sprintf("%s=%s", key, value)
				if !containsAny(tags, tagStr) {
					return false
				}
			}
		}
		if rq.Type != nil {
			typeString, err := enum.GetStringValue(*rq.Type)
			if err == nil && typeString != nil && data["Type"] != *typeString {
				return false
			}
		}
		if rq.AddressFamily != nil {
			ip, _ := data["IPAddress"].(string)
			switch rq.AddressFamily.String() {
			case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V4.String():
				if !strings.Contains(ip, ".") {
					return false
				}
			case apiv2.IPAddressFamily_IP_ADDRESS_FAMILY_V6.String():
				if !strings.Contains(ip, ":") {
					return false
				}
			}
		}
		return true
	}
}
