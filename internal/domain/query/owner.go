package query

import (
	"fmt"
	"strings"
)

// OwnerKeySep separates the team and component in a combined owner key. Team
// names may not contain this character (only the first occurrence is treated
// as the separator when splitting).
const OwnerKeySep = "|"

// OwnerKey builds the combined "team|component" pivot key used by the
// RunOwner/TaskOwner projection fields. Aggregating on this correlated key
// (rather than team_name and component_name independently) keeps owner pairs
// intact through a terms aggregation fan-out — see fields.go.
func OwnerKey(team, component string) string {
	return team + OwnerKeySep + component
}

// SplitOwnerKey splits a combined owner key produced by OwnerKey back into its
// team and component parts.
func SplitOwnerKey(key string) (team, component string, err error) {
	team, component, ok := strings.Cut(key, OwnerKeySep)
	if !ok {
		return "", "", fmt.Errorf("owner key %q: missing %q separator", key, OwnerKeySep)
	}
	return team, component, nil
}
