package service

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// virtualRoutePoolEntry is one addressable member of a virtual model pool. A
// static target is served by any channel that offers the model, while a source
// entry stays with the channel it came from.
type virtualRoutePoolEntry struct {
	model           string
	channelId       int
	reasoningEffort string
	capacity        int
	weight          float64
	capacityGroup   string
}

// virtualRoutePool expands a route into its pool. Static targets come first, so
// a pinned target keeps winning, then every model of the configured source
// channels follows the channel list automatically.
func virtualRoutePool(route operation_setting.VirtualModelRoute, requestReasoningEffort string) []virtualRoutePoolEntry {
	entries := make([]virtualRoutePoolEntry, 0, len(route.Targets))
	seen := make(map[string]struct{})
	groupCapacity := make(map[string]int)
	recordGroupCapacity := func(group string, capacity int) {
		group = strings.ToLower(strings.TrimSpace(group))
		if group == "" || capacity <= groupCapacity[group] {
			return
		}
		groupCapacity[group] = capacity
	}
	for _, target := range route.Targets {
		recordGroupCapacity(target.SharedCapacityGroup, target.Capacity)
	}
	for _, source := range route.Sources {
		recordGroupCapacity(source.SharedCapacityGroup, source.Capacity)
	}
	capacityForGroup := func(group string, fallback int) int {
		group = strings.TrimSpace(group)
		if group == "" {
			return fallback
		}
		for name, configured := range route.CapacityGroups {
			if strings.EqualFold(strings.TrimSpace(name), group) {
				if configured.Capacity > 0 {
					return configured.Capacity
				}
				return fallback
			}
		}
		if capacity := groupCapacity[strings.ToLower(group)]; capacity > 0 {
			return capacity
		}
		return fallback
	}
	add := func(modelName string, channelId int, reasoningEffort string, capacity int, weight float64, capacityGroup string) {
		key := strconv.Itoa(channelId) + "|" + strings.ToLower(modelName)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		entries = append(entries, virtualRoutePoolEntry{
			model:           modelName,
			channelId:       channelId,
			reasoningEffort: reasoningEffort,
			capacity:        capacityForGroup(capacityGroup, capacity),
			weight:          weight,
			capacityGroup:   capacityGroup,
		})
	}

	for _, target := range route.Targets {
		modelName := strings.TrimSpace(target.Model)
		if modelName == "" {
			continue
		}
		add(modelName, target.ChannelId,
			operation_setting.MapVirtualModelReasoningEffort(target, requestReasoningEffort),
			target.Capacity, target.Weight, target.SharedCapacityGroup)
	}
	for _, source := range route.Sources {
		channel, err := model.CacheGetChannel(source.ChannelId)
		if err != nil || channel == nil {
			continue
		}
		for _, modelName := range strings.Split(channel.Models, ",") {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			add(modelName, source.ChannelId, "", source.Capacity, source.Weight, source.SharedCapacityGroup)
		}
	}
	return entries
}
