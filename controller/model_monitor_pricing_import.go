package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

func ImportModelMonitorPricingSnapshot(c *gin.Context) {
	if !authorizeModelMonitorImport(c) {
		return
	}

	var request dto.ModelMonitorPricingImportRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model monitor pricing payload"})
		return
	}
	result, err := service.ImportModelMonitorPricing(request)
	if err != nil {
		service.RecordModelMonitorPricingSyncFailure(request.SiteName, err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func ImportModelMonitorActualCosts(c *gin.Context) {
	if !authorizeModelMonitorImport(c) {
		return
	}

	var request dto.ModelMonitorActualCostImportRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model monitor actual cost payload"})
		return
	}
	result, err := service.ImportModelMonitorActualCosts(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func authorizeModelMonitorImport(c *gin.Context) bool {
	userID, ok := c.Get("id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication is required"})
		return false
	}
	user, err := model.GetUserById(userID.(int), false)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication is required"})
		return false
	}
	if user.Role < common.RoleAdminUser && !operation_setting.IsModelMonitorPricingImportUser(user.Id) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "pricing import permission is required"})
		return false
	}
	return true
}
