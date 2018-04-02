package vault

import (
	"fmt"
	"time"

	"github.com/hashicorp/vault/logical"
)

func calculateTTL(sysView logical.SystemView, increment, backendTTL, period, backendMaxTTL, explicitMaxTTL time.Duration, startTime time.Time) (ttl time.Duration, warnings []string, errors error) {
	// Truncate all times to the second since that is the lowest precision for
	// TTLs
	now := time.Now().Truncate(time.Second)
	if startTime.IsZero() {
		startTime = now
	} else {
		startTime = startTime.Truncate(time.Second)
	}

	// Start off with the sys default value, and update according to period/TTL
	// from resp.Auth
	ttl = sysView.DefaultLeaseTTL()

	// Use the mount's configured max unless the backend specifies
	// something more restrictive (perhaps from a role configuration
	// parameter)
	maxTTL := sysView.MaxLeaseTTL()
	if backendMaxTTL > 0 && backendMaxTTL < maxTTL {
		maxTTL = backendMaxTTL
	}
	if explicitMaxTTL > 0 && explicitMaxTTL < maxTTL {
		maxTTL = explicitMaxTTL
	}

	// Should never happen, but guard anyways
	if maxTTL < 0 {
		return 0, nil, fmt.Errorf("max TTL is negative")
	}

	// Determine the correct increment to use
	if increment <= 0 {
		if backendTTL > 0 {
			increment = backendTTL
		} else {
			increment = sysView.DefaultLeaseTTL()
		}
	}

	var maxValidTime time.Time
	switch {
	case period > 0:
		// Cap the period value to the sys max_ttl value
		if period > maxTTL {
			warnings = append(warnings,
				fmt.Sprintf("Period of %q exceeded the effective max_ttl of %q; Period value is capped accordingly", period, maxTTL))
			period = maxTTL
		}
		ttl = period

		if explicitMaxTTL > 0 {
			maxValidTime = startTime.Add(explicitMaxTTL)
		}
	case increment > 0:
		// We cannot go past this time
		maxValidTime = startTime.Add(maxTTL)
		ttl = increment
	}

	if !maxValidTime.IsZero() {
		// Determine the max valid TTL
		maxValidTTL := maxValidTime.Sub(now)

		// If we are past the max TTL, we shouldn't be in this function...but
		// fast path out if we are
		if maxValidTTL < 0 {
			return 0, nil, fmt.Errorf("past the max TTL, cannot renew")
		}

		// If the proposed expiration is after the maximum TTL of the lease,
		// cap the increment to whatever is left, with a small buffer due to
		// time elapsed
		if maxValidTTL-ttl < 0 {
			warnings = append(warnings,
				fmt.Sprintf("TTL of %q exceeded the effective max_ttl of %q; TTL value is capped accordingly", ttl, maxValidTTL))
			ttl = maxValidTTL
		}
	}

	return ttl, warnings, nil
}