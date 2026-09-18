package objects

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyProfileMapModel_FirstMatchDoesNotChain(t *testing.T) {
	profile := &APIKeyProfile{ModelMappings: []ModelMapping{
		{From: "[invalid", To: "invalid-target"},
		{From: "alias.*", To: "registered"},
		{From: "alias-exact", To: "later-target"},
		{From: "registered", To: "chained-target"},
	}}
	require.Equal(t, "registered", profile.MapModel("alias-exact"))
	require.Equal(t, "unknown", profile.MapModel("unknown"))
	require.Equal(t, "alias-exact", (*APIKeyProfile)(nil).MapModel("alias-exact"))
}
