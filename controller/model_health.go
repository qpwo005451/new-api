package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetModelHealthCooldowns(c *gin.Context) {
	snapshot := service.ListModelHealthCooldowns()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    snapshot,
	})
}

func ResetModelHealthCooldowns(c *gin.Context) {
	cleared := service.ResetModelHealthState()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"cleared": cleared,
		},
	})
}
