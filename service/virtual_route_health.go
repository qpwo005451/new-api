package service

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// virtualRouteHealthEntries tracks how each pool entry has been behaving.
// It is deliberately process-local: the signal comes from relayed traffic, so
// no probe, table, or cross-instance state is needed.
var virtualRouteHealthEntries sync.Map

type virtualRouteHealthEntry struct {
	mutex             sync.Mutex
	consecutiveErrors int
	cooldownUntil     time.Time
}

// IsVirtualRouteUnavailableStatus reports whether an upstream status means the
// pool entry stopped serving, as opposed to a client-side request problem.
func IsVirtualRouteUnavailableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusNotFound ||
		statusCode >= http.StatusInternalServerError
}

func virtualRouteHealthKey(virtualModel string, channelID int, upstreamModel string) string {
	return strings.ToLower(strings.TrimSpace(virtualModel)) + "|" +
		strconv.Itoa(channelID) + "|" +
		strings.ToLower(strings.TrimSpace(upstreamModel))
}

func virtualRouteHealthFor(key string) *virtualRouteHealthEntry {
	value, _ := virtualRouteHealthEntries.LoadOrStore(key, &virtualRouteHealthEntry{})
	return value.(*virtualRouteHealthEntry)
}

// RecordVirtualRouteFailure cools the pool entry that just failed down, so the
// healthy entries of the pool are preferred for the next requests.
func RecordVirtualRouteFailure(c *gin.Context, channelID int, virtualModel string, statusCode int) {
	entry, health, ok := virtualRouteHealthTarget(c, channelID, virtualModel)
	if !ok || !IsVirtualRouteUnavailableStatus(statusCode) {
		return
	}
	entry.recordFailure(health)
}

// RecordVirtualRouteSuccess clears the cooldown of the pool entry that served
// the request, so a recovered entry returns to the healthy part of the pool.
func RecordVirtualRouteSuccess(c *gin.Context, channelID int, virtualModel string) {
	entry, _, ok := virtualRouteHealthTarget(c, channelID, virtualModel)
	if !ok {
		return
	}
	entry.recordSuccess()
}

func virtualRouteHealthTarget(c *gin.Context, channelID int, virtualModel string) (*virtualRouteHealthEntry, operation_setting.VirtualModelRouteHealth, bool) {
	route := operation_setting.GetVirtualModelRoute(virtualModel)
	if !route.Health.Enabled {
		return nil, operation_setting.VirtualModelRouteHealth{}, false
	}
	upstreamModel := common.GetContextKeyString(c, constant.ContextKeyVirtualUpstreamModel)
	if upstreamModel == "" {
		return nil, operation_setting.VirtualModelRouteHealth{}, false
	}
	return virtualRouteHealthFor(virtualRouteHealthKey(virtualModel, channelID, upstreamModel)), route.Health, true
}

// isCoolingDown reports whether the entry should be tried only after the
// healthy entries of the pool.
func (entry *virtualRouteHealthEntry) isCoolingDown(now time.Time) bool {
	entry.mutex.Lock()
	defer entry.mutex.Unlock()
	return now.Before(entry.cooldownUntil)
}

func (entry *virtualRouteHealthEntry) recordFailure(health operation_setting.VirtualModelRouteHealth) {
	entry.mutex.Lock()
	defer entry.mutex.Unlock()

	entry.consecutiveErrors++
	if entry.consecutiveErrors < health.FailureThreshold {
		return
	}
	cooldown := time.Duration(health.CooldownSeconds) * time.Second
	for step := entry.consecutiveErrors - health.FailureThreshold; step > 0; step-- {
		cooldown *= 2
		if cooldown >= time.Duration(health.MaxCooldownSeconds)*time.Second {
			break
		}
	}
	if maxCooldown := time.Duration(health.MaxCooldownSeconds) * time.Second; cooldown > maxCooldown {
		cooldown = maxCooldown
	}
	entry.cooldownUntil = time.Now().Add(cooldown)
}

func (entry *virtualRouteHealthEntry) recordSuccess() {
	entry.mutex.Lock()
	defer entry.mutex.Unlock()

	entry.consecutiveErrors = 0
	entry.cooldownUntil = time.Time{}
}
