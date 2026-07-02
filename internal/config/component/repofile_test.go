package component_test

import (
	"testing"

	"github.com/matryer/is"

	"codeberg.org/aeforged/dalikamata/internal/config/component"
)

const goldenRepoYAML = `version: "1"
team: payments
component: payment-service
`

func TestParseRepoFile_Golden(t *testing.T) {
	is := is.New(t)

	f, err := component.ParseRepoFile([]byte(goldenRepoYAML))
	is.NoErr(err)
	is.Equal(f.Version, "1")
	is.Equal(f.Team, "payments")
	is.Equal(f.Component, "payment-service")
}

func TestParseRepoFile_Invalid(t *testing.T) {
	cases := map[string]string{
		"bad version": `version: "2"
team: payments
component: payment-service
`,
		"missing team": `version: "1"
component: payment-service
`,
		"missing component": `version: "1"
team: payments
`,
		"malformed yaml": `: : :`,
	}

	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			is := is.New(t)
			_, err := component.ParseRepoFile([]byte(content))
			is.True(err != nil)
		})
	}
}

func TestRepoFile_ToRepoOnboarding(t *testing.T) {
	is := is.New(t)

	f, err := component.ParseRepoFile([]byte(goldenRepoYAML))
	is.NoErr(err)

	o, err := f.ToRepoOnboarding("PROJ/backend-api")
	is.NoErr(err)
	is.Equal(o.RepoID, "PROJ/backend-api")
	is.Equal(o.Component, "payment-service")
	is.Equal(o.Team, "payments")
}
