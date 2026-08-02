package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ModelMonitorAlertTransportEmail    = "email"
	ModelMonitorAlertTransportTelegram = "telegram"

	ModelMonitorAlertDeliveryPending = "pending"
	ModelMonitorAlertDeliverySent    = "sent"
	ModelMonitorAlertDeliveryDead    = "dead"
)

type ModelMonitorPathState struct {
	ID                   int64  `json:"id" gorm:"primaryKey"`
	SiteID               int64  `json:"site_id" gorm:"not null;uniqueIndex:uk_model_monitor_path_state,priority:1;index"`
	TargetID             int64  `json:"target_id" gorm:"not null;uniqueIndex:uk_model_monitor_path_state,priority:2;index"`
	ChannelID            int    `json:"channel_id" gorm:"not null;uniqueIndex:uk_model_monitor_path_state,priority:3;index"`
	ModelName            string `json:"model_name" gorm:"type:varchar(255);not null;index"`
	Status               string `json:"status" gorm:"type:varchar(32);not null;index"`
	ConsecutiveFailures  int    `json:"consecutive_failures"`
	ConsecutiveSuccesses int    `json:"consecutive_successes"`
	LastObservationID    int64  `json:"last_observation_id" gorm:"index"`
	LastObservedAt       int64  `json:"last_observed_at" gorm:"bigint;index"`
	LastTransitionAt     int64  `json:"last_transition_at" gorm:"bigint;index"`
	TransitionVersion    int64  `json:"transition_version" gorm:"bigint"`
	LastFailureType      string `json:"last_failure_type" gorm:"type:varchar(64)"`
	LastErrorSummary     string `json:"last_error_summary" gorm:"type:text"`
	CreatedAt            int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt            int64  `json:"updated_at" gorm:"bigint;index"`
}

type ModelMonitorAlertOutbox struct {
	ID                int64  `json:"id" gorm:"primaryKey"`
	EventKey          string `json:"event_key" gorm:"type:varchar(255);not null;uniqueIndex"`
	SiteID            int64  `json:"site_id" gorm:"not null;index"`
	TargetID          int64  `json:"target_id" gorm:"not null;index"`
	ChannelID         int    `json:"channel_id" gorm:"not null;index"`
	ModelName         string `json:"model_name" gorm:"type:varchar(255);not null;index"`
	PreviousStatus    string `json:"previous_status" gorm:"type:varchar(32)"`
	Status            string `json:"status" gorm:"type:varchar(32);not null;index"`
	FailureType       string `json:"failure_type" gorm:"type:varchar(64)"`
	ErrorSummary      string `json:"error_summary" gorm:"type:text"`
	ObservedAt        int64  `json:"observed_at" gorm:"bigint;not null;index"`
	TransitionVersion int64  `json:"transition_version" gorm:"bigint;not null"`
	Transport         string `json:"transport" gorm:"type:varchar(32);not null;index"`
	DeliveryStatus    string `json:"delivery_status" gorm:"type:varchar(32);not null;index"`
	Attempts          int    `json:"attempts"`
	NextAttemptAt     int64  `json:"next_attempt_at" gorm:"bigint;index"`
	ClaimedBy         string `json:"claimed_by" gorm:"type:varchar(128);index"`
	ClaimedUntil      int64  `json:"claimed_until" gorm:"bigint;index"`
	LastError         string `json:"last_error" gorm:"type:text"`
	SentAt            int64  `json:"sent_at" gorm:"bigint;index"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt         int64  `json:"updated_at" gorm:"bigint;index"`
}

func (state *ModelMonitorPathState) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if state.Status == "" {
		state.Status = ModelMonitorStatusUnknown
	}
	if state.CreatedAt == 0 {
		state.CreatedAt = now
	}
	if state.UpdatedAt == 0 {
		state.UpdatedAt = now
	}
	return nil
}

func (event *ModelMonitorAlertOutbox) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if event.DeliveryStatus == "" {
		event.DeliveryStatus = ModelMonitorAlertDeliveryPending
	}
	if event.NextAttemptAt == 0 {
		event.NextAttemptAt = now
	}
	if event.CreatedAt == 0 {
		event.CreatedAt = now
	}
	if event.UpdatedAt == 0 {
		event.UpdatedAt = now
	}
	return nil
}

func RecordModelMonitorObservation(observation *ModelMonitorObservation, alertTransports []string) ([]ModelMonitorAlertOutbox, error) {
	if observation == nil {
		return nil, fmt.Errorf("model monitor observation is required")
	}

	events := make([]ModelMonitorAlertOutbox, 0, len(alertTransports))
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(observation).Error; err != nil {
			return err
		}

		seed := ModelMonitorPathState{
			SiteID:    observation.SiteID,
			TargetID:  observation.TargetID,
			ChannelID: observation.ChannelID,
			ModelName: observation.ModelName,
			Status:    ModelMonitorStatusUnknown,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}

		var state ModelMonitorPathState
		if err := lockForUpdate(tx).
			Where("site_id = ? AND target_id = ? AND channel_id = ?", observation.SiteID, observation.TargetID, observation.ChannelID).
			First(&state).Error; err != nil {
			return err
		}

		previousStatus := state.Status
		nextStatus := previousStatus
		switch observation.Status {
		case ModelMonitorStatusAvailable:
			state.ConsecutiveFailures = 0
			state.ConsecutiveSuccesses++
			if previousStatus == ModelMonitorStatusUnknown ||
				previousStatus == ModelMonitorStatusAvailable ||
				state.ConsecutiveSuccesses >= 2 {
				nextStatus = ModelMonitorStatusAvailable
			}
		case ModelMonitorStatusLimited:
			state.ConsecutiveFailures = 0
			state.ConsecutiveSuccesses = 0
			nextStatus = ModelMonitorStatusLimited
		case ModelMonitorStatusUnavailable:
			state.ConsecutiveSuccesses = 0
			state.ConsecutiveFailures++
			if state.ConsecutiveFailures >= 3 {
				nextStatus = ModelMonitorStatusUnavailable
			} else {
				nextStatus = ModelMonitorStatusLimited
			}
		}

		state.ModelName = observation.ModelName
		state.LastObservationID = observation.ID
		state.LastObservedAt = observation.ObservedAt
		state.LastFailureType = observation.FailureType
		state.LastErrorSummary = observation.ErrorSummary
		state.UpdatedAt = common.GetTimestamp()
		if nextStatus != previousStatus {
			state.Status = nextStatus
			state.TransitionVersion++
			state.LastTransitionAt = observation.ObservedAt
		}
		if err := tx.Save(&state).Error; err != nil {
			return err
		}

		shouldAlert := nextStatus != previousStatus &&
			(nextStatus == ModelMonitorStatusUnavailable ||
				(previousStatus == ModelMonitorStatusUnavailable && nextStatus == ModelMonitorStatusAvailable))
		if !shouldAlert {
			return nil
		}

		seenTransports := make(map[string]struct{}, len(alertTransports))
		for _, transport := range alertTransports {
			transport = strings.TrimSpace(transport)
			if transport != ModelMonitorAlertTransportEmail && transport != ModelMonitorAlertTransportTelegram {
				continue
			}
			if _, exists := seenTransports[transport]; exists {
				continue
			}
			seenTransports[transport] = struct{}{}
			event := ModelMonitorAlertOutbox{
				EventKey: fmt.Sprintf(
					"model-monitor:%d:%d:%d:%d:%s",
					observation.SiteID,
					observation.TargetID,
					observation.ChannelID,
					state.TransitionVersion,
					transport,
				),
				SiteID:            observation.SiteID,
				TargetID:          observation.TargetID,
				ChannelID:         observation.ChannelID,
				ModelName:         observation.ModelName,
				PreviousStatus:    previousStatus,
				Status:            nextStatus,
				FailureType:       observation.FailureType,
				ErrorSummary:      observation.ErrorSummary,
				ObservedAt:        observation.ObservedAt,
				TransitionVersion: state.TransitionVersion,
				Transport:         transport,
				DeliveryStatus:    ModelMonitorAlertDeliveryPending,
				NextAttemptAt:     common.GetTimestamp(),
			}
			if err := tx.Create(&event).Error; err != nil {
				return err
			}
			events = append(events, event)
		}
		return nil
	})
	return events, err
}

func QueueDueModelMonitorTelegramRepeats(
	now int64,
	intervalSeconds int64,
	matches func(siteID int64, channelID int, modelName string) bool,
) (int, error) {
	if intervalSeconds <= 0 || matches == nil {
		return 0, nil
	}
	var states []ModelMonitorPathState
	if err := dueModelMonitorTelegramRepeatQuery(now, intervalSeconds).
		Find(&states).Error; err != nil {
		return 0, err
	}

	created := 0
	for _, state := range states {
		if !matches(state.SiteID, state.ChannelID, state.ModelName) {
			continue
		}
		event := ModelMonitorAlertOutbox{
			EventKey: fmt.Sprintf(
				"model-monitor-repeat:%d:%d:%d:%d:%d:%s",
				state.SiteID,
				state.TargetID,
				state.ChannelID,
				state.TransitionVersion,
				now,
				ModelMonitorAlertTransportTelegram,
			),
			SiteID:            state.SiteID,
			TargetID:          state.TargetID,
			ChannelID:         state.ChannelID,
			ModelName:         state.ModelName,
			PreviousStatus:    ModelMonitorStatusUnavailable,
			Status:            ModelMonitorStatusUnavailable,
			FailureType:       state.LastFailureType,
			ErrorSummary:      state.LastErrorSummary,
			ObservedAt:        state.LastTransitionAt,
			TransitionVersion: state.TransitionVersion,
			Transport:         ModelMonitorAlertTransportTelegram,
			DeliveryStatus:    ModelMonitorAlertDeliveryPending,
			NextAttemptAt:     now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		result := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&event)
		if result.Error != nil {
			return created, result.Error
		}
		if result.RowsAffected > 0 {
			created++
		}
	}
	return created, nil
}

func HasDueModelMonitorTelegramRepeat(
	now int64,
	intervalSeconds int64,
	matches func(siteID int64, channelID int, modelName string) bool,
) (bool, error) {
	if intervalSeconds <= 0 || matches == nil {
		return false, nil
	}
	var states []ModelMonitorPathState
	if err := dueModelMonitorTelegramRepeatQuery(now, intervalSeconds).
		Find(&states).Error; err != nil {
		return false, err
	}
	for _, state := range states {
		if matches(state.SiteID, state.ChannelID, state.ModelName) {
			return true, nil
		}
	}
	return false, nil
}

func dueModelMonitorTelegramRepeatQuery(now int64, intervalSeconds int64) *gorm.DB {
	recentRepeats := DB.Model(&ModelMonitorAlertOutbox{}).
		Select("1").
		Where(
			"model_monitor_alert_outboxes.site_id = model_monitor_path_states.site_id AND "+
				"model_monitor_alert_outboxes.target_id = model_monitor_path_states.target_id AND "+
				"model_monitor_alert_outboxes.channel_id = model_monitor_path_states.channel_id AND "+
				"model_monitor_alert_outboxes.transition_version = model_monitor_path_states.transition_version AND "+
				"model_monitor_alert_outboxes.transport = ? AND "+
				"model_monitor_alert_outboxes.event_key LIKE ? AND "+
				"model_monitor_alert_outboxes.created_at > ?",
			ModelMonitorAlertTransportTelegram,
			"model-monitor-repeat:%",
			now-intervalSeconds,
		)
	return DB.Where(
		"status = ? AND last_transition_at > 0 AND last_transition_at <= ? AND NOT EXISTS (?)",
		ModelMonitorStatusUnavailable,
		now-intervalSeconds,
		recentRepeats,
	)
}

func IsCurrentModelMonitorUnavailableTransition(event ModelMonitorAlertOutbox) (bool, error) {
	var state ModelMonitorPathState
	err := DB.
		Where(
			"site_id = ? AND target_id = ? AND channel_id = ?",
			event.SiteID,
			event.TargetID,
			event.ChannelID,
		).
		First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return state.Status == ModelMonitorStatusUnavailable &&
		state.TransitionVersion == event.TransitionVersion, nil
}

func HasDueModelMonitorAlertOutbox(now int64) (bool, error) {
	var count int64
	err := DB.Model(&ModelMonitorAlertOutbox{}).
		Where("delivery_status = ? AND next_attempt_at <= ? AND (claimed_until = 0 OR claimed_until < ?)",
			ModelMonitorAlertDeliveryPending, now, now).
		Limit(1).
		Count(&count).Error
	return count > 0, err
}

func ClaimDueModelMonitorAlertOutbox(now int64, claimedUntil int64, claimedBy string, limit int) ([]ModelMonitorAlertOutbox, error) {
	if limit <= 0 {
		limit = 20
	}
	claimed := make([]ModelMonitorAlertOutbox, 0, limit)
	err := DB.Transaction(func(tx *gorm.DB) error {
		candidates := make([]ModelMonitorAlertOutbox, 0, limit)
		if err := lockForUpdate(tx).
			Where("delivery_status = ? AND next_attempt_at <= ? AND (claimed_until = 0 OR claimed_until < ?)",
				ModelMonitorAlertDeliveryPending, now, now).
			Order("id asc").
			Limit(limit).
			Find(&candidates).Error; err != nil {
			return err
		}
		for _, candidate := range candidates {
			result := tx.Model(&ModelMonitorAlertOutbox{}).
				Where("id = ? AND delivery_status = ? AND next_attempt_at <= ? AND (claimed_until = 0 OR claimed_until < ?)",
					candidate.ID, ModelMonitorAlertDeliveryPending, now, now).
				Updates(map[string]any{
					"attempts":      gorm.Expr("attempts + 1"),
					"claimed_by":    claimedBy,
					"claimed_until": claimedUntil,
					"updated_at":    now,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			var stored ModelMonitorAlertOutbox
			if err := tx.First(&stored, candidate.ID).Error; err != nil {
				return err
			}
			claimed = append(claimed, stored)
		}
		return nil
	})
	return claimed, err
}

func CompleteModelMonitorAlertOutbox(id int64, claimedBy string, sentAt int64) error {
	result := DB.Model(&ModelMonitorAlertOutbox{}).
		Where("id = ? AND delivery_status = ? AND claimed_by = ?", id, ModelMonitorAlertDeliveryPending, claimedBy).
		Updates(map[string]any{
			"delivery_status": ModelMonitorAlertDeliverySent,
			"sent_at":         sentAt,
			"claimed_by":      "",
			"claimed_until":   0,
			"last_error":      "",
			"updated_at":      sentAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("model monitor alert outbox claim lost")
	}
	return nil
}

func RetryModelMonitorAlertOutbox(id int64, claimedBy string, nextAttemptAt int64, errorMessage string, maxAttempts int) error {
	var event ModelMonitorAlertOutbox
	if err := DB.Where("id = ? AND delivery_status = ? AND claimed_by = ?", id, ModelMonitorAlertDeliveryPending, claimedBy).
		First(&event).Error; err != nil {
		return err
	}
	deliveryStatus := ModelMonitorAlertDeliveryPending
	if maxAttempts > 0 && event.Attempts >= maxAttempts {
		deliveryStatus = ModelMonitorAlertDeliveryDead
	}
	result := DB.Model(&ModelMonitorAlertOutbox{}).
		Where("id = ? AND delivery_status = ? AND claimed_by = ?", id, ModelMonitorAlertDeliveryPending, claimedBy).
		Updates(map[string]any{
			"delivery_status": deliveryStatus,
			"next_attempt_at": nextAttemptAt,
			"claimed_by":      "",
			"claimed_until":   0,
			"last_error":      errorMessage,
			"updated_at":      common.GetTimestamp(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("model monitor alert outbox claim lost")
	}
	return nil
}
