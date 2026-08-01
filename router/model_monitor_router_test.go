package router

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/stretchr/testify/assert"
)

func TestModelMonitorRoutesUseDedicatedPermissions(t *testing.T) {
	expected := []permissionRoute{
		{method: http.MethodGet, path: "/summary", permission: authz.ModelMonitorRead, handler: controller.GetModelMonitorSummary},
		{method: http.MethodGet, path: "/sites", permission: authz.ModelMonitorRead, handler: controller.GetModelMonitorSites},
		{method: http.MethodGet, path: "/sites/:site_id", permission: authz.ModelMonitorRead, handler: controller.GetModelMonitorSite},
		{method: http.MethodGet, path: "/sites/:site_id/models/:model", permission: authz.ModelMonitorRead, handler: controller.GetModelMonitorModel},
		{method: http.MethodGet, path: "/config", permission: authz.ModelMonitorRead, handler: controller.GetModelMonitorConfig},
		{method: http.MethodPut, path: "/config", permission: authz.ModelMonitorWrite, handler: controller.UpdateModelMonitorConfig},
		{method: http.MethodPost, path: "/runs", permission: authz.ModelMonitorOperate, handler: controller.EnqueueModelMonitorRun},
		{method: http.MethodGet, path: "/alerts/config", permission: authz.ModelMonitorRead, handler: controller.GetModelMonitorAlertConfig},
		{method: http.MethodPut, path: "/alerts/config", permission: authz.ModelMonitorWrite, handler: controller.UpdateModelMonitorAlertConfig},
		{method: http.MethodPost, path: "/alerts/test", permission: authz.ModelMonitorOperate, handler: controller.TestModelMonitorAlerts},
	}

	if assert.Len(t, modelMonitorPermissionRoutes, len(expected)) {
		for index, route := range expected {
			assert.Equal(t, route.method, modelMonitorPermissionRoutes[index].method)
			assert.Equal(t, route.path, modelMonitorPermissionRoutes[index].path)
			assert.Equal(t, route.permission, modelMonitorPermissionRoutes[index].permission)
			assert.Equal(t, reflect.ValueOf(route.handler).Pointer(), reflect.ValueOf(modelMonitorPermissionRoutes[index].handler).Pointer())
		}
	}
}
