package validate

import (
	"fmt"

	apiv1 "github.com/metal-stack/api/go/metalstack/api/v2"
)

func ValidateAddressFamily(af apiv1.IPAddressFamily) error {
	switch af {
	case apiv1.IPAddressFamily_IP_ADDRESS_FAMILY_V4, apiv1.IPAddressFamily_IP_ADDRESS_FAMILY_V6:
		return nil
	case apiv1.IPAddressFamily_IP_ADDRESS_FAMILY_UNSPECIFIED:
		return fmt.Errorf("unsupported addressfamily: %s", af.String())
	default:
		return fmt.Errorf("unsupported addressfamily: %s", af.String())
	}
}
