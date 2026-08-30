package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func parseFlowQuotaTimeRange(c *gin.Context) (int64, int64, bool) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid start_timestamp")
		return 0, 0, false
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid end_timestamp")
		return 0, 0, false
	}
	if endTimestamp < startTimestamp {
		common.ApiErrorMsg(c, "invalid time range")
		return 0, 0, false
	}
	return startTimestamp, endTimestamp, true
}

func GetAllQuotaDates(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	username := c.Query("username")
	dates, err := model.GetAllQuotaDates(startTimestamp, endTimestamp, username)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetQuotaDatesByUser(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	dates, err := model.GetQuotaDataGroupByUser(startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
}

func GetUserQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 判断时间跨度是否超过 1 个月
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	dates, err := model.GetQuotaDataByUserId(userId, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

const tokenTrendMaxSpanSeconds = int64(32 * 24 * 3600)

// GetTokenTrend returns hourly token-usage buckets (including cache
// read/write tokens parsed from each log's other payload) for the dashboard
// token trend chart. Admins may pass username to scope another user.
func GetTokenTrend(c *gin.Context) {
	startTimestamp, err := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	if err != nil || startTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid start_timestamp")
		return
	}
	endTimestamp, err := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if err != nil || endTimestamp <= 0 {
		common.ApiErrorMsg(c, "invalid end_timestamp")
		return
	}
	if endTimestamp < startTimestamp {
		common.ApiErrorMsg(c, "invalid time range")
		return
	}
	if endTimestamp-startTimestamp > tokenTrendMaxSpanSeconds {
		common.ApiErrorMsg(c, "time range too large")
		return
	}
	userId := c.GetInt("id")
	if username := c.Query("username"); username != "" && c.GetInt("role") >= common.RoleAdminUser {
		var user model.User
		if err := model.DB.Where("username = ?", username).First(&user).Error; err != nil {
			common.ApiErrorMsg(c, "user not found")
			return
		}
		userId = user.Id
	}
	points, err := model.GetTokenTrend(userId, startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    points,
	})
}

func GetAllFlowQuotaDates(c *gin.Context) {
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return
	}
	if !ok {
		return
	}
	username := c.Query("username")
	dates, err := model.GetFlowQuotaData(startTimestamp, endTimestamp, username, 0, c.GetInt("role"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}

func GetUserFlowQuotaDates(c *gin.Context) {
	userId := c.GetInt("id")
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return
	}
	if endTimestamp-startTimestamp > 2592000 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "时间跨度不能超过 1 个月",
		})
		return
	}
	dates, err := model.GetFlowQuotaData(startTimestamp, endTimestamp, "", userId, common.RoleCommonUser)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    dates,
	})
	return
}
