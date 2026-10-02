package service

import (
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// A sticky session keeps the channel it starts on for the whole binding TTL, so
// a plain weighted draw spreads live sessions unevenly: the realised split is a
// multinomial over sessions, and with few concurrent sessions it can sit far
// from the configured weights. Load-aware placement starts every new sticky
// session on the candidate that carries the fewest live sessions per unit of
// configured weight, so the session split converges to the configured weights.
//
// The ledger below is per process and expires with the binding TTL. With Redis
// backed bindings a multi-node deployment therefore balances per node instead of
// globally; a single-node deployment is exact.
const (
	// maxAffinityPlacementEntries bounds the in-process placement ledger.
	maxAffinityPlacementEntries = 100_000
	// affinityPlacementFallbackTTL bounds a placement when the rule has no TTL.
	affinityPlacementFallbackTTL = time.Hour
	// affinityPlacementPruneInterval limits how often expired entries are swept.
	affinityPlacementPruneInterval = time.Second
)

type affinityPlacementEntry struct {
	channelID int
	expiresAt time.Time
}

type affinityPlacementLedger struct {
	mu         sync.Mutex
	entries    map[string]affinityPlacementEntry
	lastPruned time.Time
}

var (
	affinityPlacementOnce  sync.Once
	affinityPlacementStore *affinityPlacementLedger
)

func channelAffinityPlacementLedger() *affinityPlacementLedger {
	affinityPlacementOnce.Do(func() {
		affinityPlacementStore = &affinityPlacementLedger{entries: map[string]affinityPlacementEntry{}}
	})
	return affinityPlacementStore
}

// place remembers the channel a session started on until its binding expires.
func (l *affinityPlacementLedger) place(sessionKey string, channelID int, ttl time.Duration, now time.Time) {
	if l == nil || sessionKey == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pruneLocked(now)
	l.entries[sessionKey] = affinityPlacementEntry{channelID: channelID, expiresAt: now.Add(ttl)}
}

// counts reports how many live sessions of one rule scope sit on each channel.
func (l *affinityPlacementLedger) counts(scope string, now time.Time) map[int]int {
	counts := map[int]int{}
	if l == nil || scope == "" {
		return counts
	}
	prefix := scope + ":"
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, entry := range l.entries {
		if !entry.expiresAt.After(now) {
			continue
		}
		if strings.HasPrefix(key, prefix) {
			counts[entry.channelID]++
		}
	}
	return counts
}

func (l *affinityPlacementLedger) pruneLocked(now time.Time) {
	if now.Sub(l.lastPruned) < affinityPlacementPruneInterval && len(l.entries) < maxAffinityPlacementEntries {
		return
	}
	l.lastPruned = now
	for key, entry := range l.entries {
		if !entry.expiresAt.After(now) {
			delete(l.entries, key)
		}
	}
	// The ledger only steers placement, so dropping arbitrary entries beyond the
	// cap keeps memory bounded without affecting stickiness.
	for key := range l.entries {
		if len(l.entries) < maxAffinityPlacementEntries {
			break
		}
		delete(l.entries, key)
	}
}

// splitChannelAffinitySessionKey splits a binding cache key into the scope
// shared by every session of one rule (rule, model and group) and the key of the
// session itself.
func splitChannelAffinitySessionKey(cacheKeyFull string) (string, string, bool) {
	index := strings.LastIndex(cacheKeyFull, ":")
	if index <= 0 || index == len(cacheKeyFull)-1 {
		return "", "", false
	}
	return cacheKeyFull[:index], cacheKeyFull, true
}

// PlaceChannelAffinitySession picks the channel a sticky session without a
// binding should start on, and remembers the choice for the binding TTL. It
// reports false when placement does not apply, so the caller keeps the normal
// weighted draw.
func PlaceChannelAffinitySession(
	c *gin.Context,
	setting *operation_setting.ChannelAffinitySetting,
	cacheKeyFull string,
	modelName string,
	usingGroup string,
	ttlSeconds int,
) (int, bool) {
	if c == nil || setting == nil || modelName == "" || cacheKeyFull == "" {
		return 0, false
	}
	if setting.EffectivePlacement() != operation_setting.ChannelAffinityPlacementBalanced {
		return 0, false
	}
	// An auto-group request resolves its group during selection, so candidates
	// cannot be scoped to a group here.
	if usingGroup == "" || usingGroup == "auto" {
		return 0, false
	}
	scope, sessionKey, ok := splitChannelAffinitySessionKey(cacheKeyFull)
	if !ok {
		return 0, false
	}
	candidates, err := model.GetSatisfiedChannelsInPriorityOrder(usingGroup, modelName, GetChannelConstraints(c).Filters)
	if err != nil || len(candidates) == 0 {
		return 0, false
	}
	candidates, err = FilterModelHealthCandidates(candidates, modelName, usingGroup)
	if err != nil || len(candidates) == 0 {
		return 0, false
	}
	topPriority := operation_setting.EffectiveModelPriority(candidates[0].Id, modelName, candidates[0].GetPriority())
	weights := make(map[int]int, len(candidates))
	tierChannels := make([]*model.Channel, 0, len(candidates))
	hasWeight := false
	for _, channel := range candidates {
		if channel == nil {
			continue
		}
		if operation_setting.EffectiveModelPriority(channel.Id, modelName, channel.GetPriority()) != topPriority {
			continue
		}
		weight := operation_setting.EffectiveModelWeight(channel.Id, modelName, channel.GetWeight())
		if weight > 0 {
			hasWeight = true
		}
		weights[channel.Id] = weight
		tierChannels = append(tierChannels, channel)
	}
	if len(tierChannels) == 0 {
		return 0, false
	}
	now := time.Now()
	counts := channelAffinityPlacementLedger().counts(scope, now)

	bestChannelID := 0
	bestSessions := 0
	bestWeight := 0
	tied := make([]int, 0, len(tierChannels))
	for _, channel := range tierChannels {
		weight := weights[channel.Id]
		if !hasWeight {
			// Every candidate has no configured weight, so they are equivalent.
			weight = 1
		} else if weight <= 0 {
			// A zero weight never wins the weighted draw, so it must not win
			// placement either.
			continue
		}
		sessions := counts[channel.Id] + 1
		if bestChannelID == 0 {
			bestChannelID, bestSessions, bestWeight = channel.Id, sessions, weight
			tied = append(tied, channel.Id)
			continue
		}
		switch {
		case sessions*bestWeight < bestSessions*weight:
			bestChannelID, bestSessions, bestWeight = channel.Id, sessions, weight
			tied = append(tied[:0], channel.Id)
		case sessions*bestWeight == bestSessions*weight:
			tied = append(tied, channel.Id)
		}
	}
	if bestChannelID == 0 {
		return 0, false
	}
	chosen := bestChannelID
	if len(tied) > 1 {
		chosen = tied[rand.Intn(len(tied))]
	}
	rememberChannelAffinityPlacement(sessionKey, chosen, ttlSeconds)
	return chosen, true
}

// rememberChannelAffinityPlacement keeps the placement ledger aligned with the
// binding that is actually enforced for a session.
func rememberChannelAffinityPlacement(cacheKeyFull string, channelID int, ttlSeconds int) {
	_, sessionKey, ok := splitChannelAffinitySessionKey(cacheKeyFull)
	if !ok || channelID <= 0 {
		return
	}
	ttl := time.Duration(ttlSeconds) * time.Second
	if ttl <= 0 {
		ttl = affinityPlacementFallbackTTL
	}
	channelAffinityPlacementLedger().place(sessionKey, channelID, ttl, time.Now())
}
