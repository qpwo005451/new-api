package authz

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMonitorPermissionsAreRegisteredForAdmins(t *testing.T) {
	var resource ResourceDefinition
	for _, candidate := range Catalog() {
		if candidate.Resource == ResourceModelMonitor {
			resource = candidate
			break
		}
	}
	require.Equal(t, ResourceModelMonitor, resource.Resource)
	require.Len(t, resource.Actions, 3)

	adminPermissions := PermissionsForRole(BuiltInRoleAdmin)
	assert.Contains(t, adminPermissions, ModelMonitorRead)
	assert.Contains(t, adminPermissions, ModelMonitorOperate)
	assert.Contains(t, adminPermissions, ModelMonitorWrite)
}
