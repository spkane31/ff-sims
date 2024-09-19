package espn

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient(t *testing.T) {
	client := NewClient(345674, nil)
	require.NotNil(t, client)

	teams, err := client.GetTeams()
	require.NoError(t, err)
	require.NotNil(t, teams)

	rosters, err := client.GetRosters()
	require.NoError(t, err)
	require.NotNil(t, rosters)

	matchups, err := client.GetMatchups()
	require.NoError(t, err)
	require.NotNil(t, matchups)
}
