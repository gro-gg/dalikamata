package query_test

import (
	"testing"

	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/domain/query"
)

func TestOwnerKeyRoundTrip(t *testing.T) {
	is := is.New(t)
	key := query.OwnerKey("backend-team", "backend")
	is.Equal(key, "backend-team|backend")

	team, comp, err := query.SplitOwnerKey(key)
	is.NoErr(err)
	is.Equal(team, "backend-team")
	is.Equal(comp, "backend")
}

func TestSplitOwnerKey_MissingSeparator(t *testing.T) {
	is := is.New(t)
	_, _, err := query.SplitOwnerKey("no-separator")
	is.True(err != nil)
}

func TestSplitOwnerKey_ComponentNameCanContainSeparator(t *testing.T) {
	is := is.New(t)
	// Only the first "|" is treated as the separator, so component names
	// (unlike team names) may safely contain it.
	team, comp, err := query.SplitOwnerKey(query.OwnerKey("team", "a|b"))
	is.NoErr(err)
	is.Equal(team, "team")
	is.Equal(comp, "a|b")
}
