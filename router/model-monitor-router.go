package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

func registerModelMonitorRoutes(apiRouter *gin.RouterGroup) {
	modelMonitorRoute := apiRouter.Group("/model-monitor")
	modelMonitorRoute.Use(middleware.AdminAuth())

	for _, route := range modelMonitorPermissionRoutes {
		modelMonitorRoute.Handle(
			route.method,
			route.path,
			middleware.RequirePermission(route.permission),
			route.handler,
		)
	}
}

var modelMonitorPermissionRoutes = []permissionRoute{
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
