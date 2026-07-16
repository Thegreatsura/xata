package api

import (
	"fmt"
	"net/netip"

	"xata/services/projects/api/spec"
)

const (
	// maxIPFilteringCIDRs is the maximum number of CIDR entries allowed per project.
	maxIPFilteringCIDRs = 64
	// maxCIDRLength is the maximum length of a single CIDR string. The longest
	// valid value is a full IPv6 address with a prefix ("xxxx:...:xxxx/128", 43 chars).
	maxCIDRLength = 64
	// maxCIDRDescriptionLength is the maximum length of a CIDR entry description.
	maxCIDRDescriptionLength = 100
)

// validateIPFiltering validates an IP filtering configuration from the API.
// Each entry must be a valid IPv4 or IPv6 address or CIDR block. Allow-all
// prefixes (/0) are rejected: they would silently disable filtering while it
// appears enabled. Unparseable entries are rejected here rather than being
// silently dropped by the gateway at enforcement time.
func validateIPFiltering(cfg *spec.IPFilteringConfiguration) error {
	if cfg == nil {
		return nil
	}

	if len(cfg.Cidr) > maxIPFilteringCIDRs {
		return ErrorInvalidParam{
			Param:   "ipFiltering.cidr",
			Message: fmt.Sprintf("too many CIDR entries: %d (maximum is %d)", len(cfg.Cidr), maxIPFilteringCIDRs),
		}
	}

	for i, entry := range cfg.Cidr {
		if err := validateCIDREntry(entry); err != nil {
			return ErrorInvalidParam{
				Param:   fmt.Sprintf("ipFiltering.cidr[%d]", i),
				Message: err.Error(),
			}
		}
	}

	return nil
}

func validateCIDREntry(entry spec.CidrEntry) error {
	if entry.Description != nil && len(*entry.Description) > maxCIDRDescriptionLength {
		return fmt.Errorf("description is too long: %d characters (maximum is %d)", len(*entry.Description), maxCIDRDescriptionLength)
	}

	cidr := entry.Cidr
	if cidr == "" {
		return fmt.Errorf("CIDR must not be empty")
	}
	if len(cidr) > maxCIDRLength {
		return fmt.Errorf("CIDR is too long: %d characters (maximum is %d)", len(cidr), maxCIDRLength)
	}

	// Accept both a CIDR block ("10.0.0.0/24", "2001:db8::/32") and a bare
	// IP address ("10.0.0.1", "2001:db8::1"), matching what the gateway
	// enforcement accepts (a bare address is treated as /32 or /128).
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		addr, addrErr := netip.ParseAddr(cidr)
		if addrErr != nil {
			return fmt.Errorf("%q is not a valid IP address or CIDR block", cidr)
		}
		if addr.Zone() != "" {
			return fmt.Errorf("%q must not contain a zone identifier", cidr)
		}
		return nil
	}

	if prefix.Bits() == 0 {
		return fmt.Errorf("%q allows all addresses; disable IP filtering instead of using a /0 prefix", cidr)
	}

	return nil
}
