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
	userID, ok := c.Get("id")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication is required"})
		return
	}
	user, err := model.GetUserById(userID.(int), false)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "authentication is required"})
		return
	}
	if user.Role < common.RoleAdminUser && !operation_setting.IsModelMonitorPricingImportUser(user.Id) {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "pricing import permission is required"})
		return
	}

	var request dto.ModelMonitorPricingImportRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model monitor pricing payload"})
		return
	}
	result, err := service.ImportNewAPIModelMonitorPricing(request)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
