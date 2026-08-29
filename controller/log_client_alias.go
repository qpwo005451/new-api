package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

type UpsertLogClientAliasRequest struct {
	UserAgent string `json:"user_agent"`
	Name      string `json:"name"`
}

// GetLogClientAliases returns the admin-defined User-Agent alias map used to
// label unrecognizable clients in the usage log views.
func GetLogClientAliases(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    operation_setting.GetClientAliases(),
	})
}

// UpdateLogClientAlias upserts one alias (empty name removes it) and returns
// the updated alias map so the caller can refresh its state in one round trip.
func UpdateLogClientAlias(c *gin.Context) {
	var req UpsertLogClientAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "invalid request body",
		})
		return
	}

	aliases, err := operation_setting.NormalizeClientAliases(
		operation_setting.GetClientAliases(), req.UserAgent, req.Name)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	valueBytes, err := common.Marshal(aliases)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "failed to encode aliases",
		})
		return
	}
	if err := model.UpdateOption(operation_setting.ClientAliasOptionKey, string(valueBytes)); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "failed to persist client aliases",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    aliases,
	})
}
