package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// GetRoutingStats reports how traffic was distributed across channels and how
// often requests had to switch channels. It is read-only: nothing is written
// and no router state is modified.
func GetRoutingStats(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	channelID, _ := strconv.Atoi(c.Query("channel_id"))

	stats, err := model.QueryRoutingStats(model.RoutingStatsQuery{
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ModelName:      c.Query("model_name"),
		Group:          c.Query("group"),
		ChannelID:      channelID,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// Each row reports the routing knobs that produced its traffic so the page
	// can compare the configured split against the observed one. Channels are
	// looked up once per distinct pair, not once per request.
	for i := range stats.ByModelChannel {
		row := &stats.ByModelChannel[i]
		channel, err := model.GetChannelById(row.ChannelID, false)
		if err != nil || channel == nil {
			continue
		}
		priority := operation_setting.EffectiveModelPriority(row.ChannelID, row.ModelName, channel.GetPriority())
		weight := uint(operation_setting.EffectiveModelWeight(row.ChannelID, row.ModelName, channel.GetWeight()))
		row.ChannelName = channel.Name
		row.Priority = &priority
		row.Weight = &weight
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    stats,
	})
}

// GetChannelAffinityBindings lists the sessions that are currently pinned to a
// channel, so the routing page can show how stickiness spreads traffic right
// now. It reads the live affinity cache only.
func GetChannelAffinityBindings(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	bindings := service.ListChannelAffinityBindings(limit)

	// Channels are looked up once per distinct id, not once per binding.
	channelNames := make(map[int]string)
	for i := range bindings.Entries {
		channelID := bindings.Entries[i].ChannelID
		name, cached := channelNames[channelID]
		if !cached {
			channel, err := model.GetChannelById(channelID, false)
			if err == nil && channel != nil {
				name = channel.Name
			}
			channelNames[channelID] = name
		}
		bindings.Entries[i].ChannelName = name
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    bindings,
	})
}
