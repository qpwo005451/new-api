package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	BalanceProtectionStateDisabled         = "disabled"
	BalanceProtectionStatePending          = "pending"
	BalanceProtectionStateNormal           = "normal"
	BalanceProtectionStateProtected        = "protected"
	BalanceProtectionStateUnknown          = "unknown"
	BalanceProtectionStateInvalidAllowlist = "invalid_allowlist"

	DefaultBalanceProtectionTriggerBalance  = 2.0
	DefaultBalanceProtectionRecoveryBalance = 5.0
	DefaultBalanceProtectionCheckInterval   = 1
	BalanceProtectionFailureLimit           = 10
)

type ChannelBalanceProtection struct {
	ChannelId            int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	Enabled              bool    `json:"enabled"`
	TriggerBalance       float64 `json:"trigger_balance"`
	RecoveryBalance      float64 `json:"recovery_balance"`
	CheckIntervalMinutes int     `json:"check_interval_minutes"`
	FreeModels           string  `json:"-" gorm:"type:text"`
	NotifyEnabled        bool    `json:"notify_enabled"`
	State                string  `json:"state" gorm:"type:varchar(32);index"`
	ConsecutiveFailures  int     `json:"consecutive_failures"`
	LastCheckTime        int64   `json:"last_check_time" gorm:"bigint"`
	LastSuccessTime      int64   `json:"last_success_time" gorm:"bigint"`
	LastTransitionTime   int64   `json:"last_transition_time" gorm:"bigint"`
	LastError            string  `json:"last_error" gorm:"type:text"`
	CreatedAt            int64   `json:"created_at" gorm:"bigint"`
	UpdatedAt            int64   `json:"updated_at" gorm:"bigint"`

	freeModelSet map[string]struct{} `json:"-" gorm:"-"`
}

type ChannelBalanceProtectionView struct {
	Supported            bool     `json:"supported"`
	Enabled              bool     `json:"enabled"`
	Active               bool     `json:"active"`
	TriggerBalance       float64  `json:"trigger_balance"`
	RecoveryBalance      float64  `json:"recovery_balance"`
	CheckIntervalMinutes int      `json:"check_interval_minutes"`
	FreeModels           []string `json:"free_models"`
	NotifyEnabled        bool     `json:"notify_enabled"`
	State                string   `json:"state"`
	ConsecutiveFailures  int      `json:"consecutive_failures"`
	LastCheckTime        int64    `json:"last_check_time"`
	LastSuccessTime      int64    `json:"last_success_time"`
	LastTransitionTime   int64    `json:"last_transition_time"`
	LastError            string   `json:"last_error"`
}

type BalanceProtectionTransition struct {
	Before *ChannelBalanceProtection
	After  *ChannelBalanceProtection
}

func DefaultChannelBalanceProtectionView(supported bool) *ChannelBalanceProtectionView {
	return &ChannelBalanceProtectionView{
		Supported:            supported,
		Enabled:              false,
		Active:               false,
		TriggerBalance:       DefaultBalanceProtectionTriggerBalance,
		RecoveryBalance:      DefaultBalanceProtectionRecoveryBalance,
		CheckIntervalMinutes: DefaultBalanceProtectionCheckInterval,
		FreeModels:           []string{},
		NotifyEnabled:        true,
		State:                BalanceProtectionStateDisabled,
	}
}

func normalizeBalanceProtectionModels(models []string) []string {
	normalized := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, ok := seen[modelName]; ok {
			continue
		}
		seen[modelName] = struct{}{}
		normalized = append(normalized, modelName)
	}
	return normalized
}

func encodeBalanceProtectionModels(models []string) (string, error) {
	data, err := common.Marshal(normalizeBalanceProtectionModels(models))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (protection *ChannelBalanceProtection) GetFreeModels() []string {
	if protection == nil || strings.TrimSpace(protection.FreeModels) == "" {
		return []string{}
	}
	var models []string
	if err := common.UnmarshalJsonStr(protection.FreeModels, &models); err != nil {
		common.SysLog(fmt.Sprintf("failed to decode channel balance protection models: channel_id=%d error=%v", protection.ChannelId, err))
		return []string{}
	}
	return normalizeBalanceProtectionModels(models)
}

func (protection *ChannelBalanceProtection) prepareFreeModelSet() {
	if protection == nil || protection.freeModelSet != nil {
		return
	}
	protection.freeModelSet = make(map[string]struct{})
	for _, modelName := range protection.GetFreeModels() {
		protection.freeModelSet[modelName] = struct{}{}
	}
}

func (protection *ChannelBalanceProtection) IsActive() bool {
	if protection == nil || !protection.Enabled {
		return false
	}
	return protection.State != BalanceProtectionStateNormal &&
		protection.State != BalanceProtectionStateDisabled
}

func (protection *ChannelBalanceProtection) AllowsModel(modelName string) bool {
	if protection == nil || !protection.IsActive() {
		return true
	}
	protection.prepareFreeModelSet()
	_, ok := protection.freeModelSet[strings.TrimSpace(modelName)]
	return ok
}

func (protection *ChannelBalanceProtection) ToView(supported bool) *ChannelBalanceProtectionView {
	if protection == nil {
		return DefaultChannelBalanceProtectionView(supported)
	}
	return &ChannelBalanceProtectionView{
		Supported:            supported,
		Enabled:              protection.Enabled,
		Active:               protection.IsActive(),
		TriggerBalance:       protection.TriggerBalance,
		RecoveryBalance:      protection.RecoveryBalance,
		CheckIntervalMinutes: protection.CheckIntervalMinutes,
		FreeModels:           protection.GetFreeModels(),
		NotifyEnabled:        protection.NotifyEnabled,
		State:                protection.State,
		ConsecutiveFailures:  protection.ConsecutiveFailures,
		LastCheckTime:        protection.LastCheckTime,
		LastSuccessTime:      protection.LastSuccessTime,
		LastTransitionTime:   protection.LastTransitionTime,
		LastError:            protection.LastError,
	}
}

func validateChannelBalanceProtection(channel *Channel, view *ChannelBalanceProtectionView) ([]string, error) {
	if channel == nil || view == nil {
		return nil, errors.New("channel balance protection is required")
	}
	if view.TriggerBalance < 0 {
		return nil, errors.New("balance protection trigger balance must not be negative")
	}
	if view.RecoveryBalance <= view.TriggerBalance {
		return nil, errors.New("balance protection recovery balance must be greater than trigger balance")
	}
	if view.CheckIntervalMinutes < 1 || view.CheckIntervalMinutes > 60 {
		return nil, errors.New("balance protection check interval must be between 1 and 60 minutes")
	}
	freeModels := normalizeBalanceProtectionModels(view.FreeModels)
	if view.Enabled && channel.ChannelInfo.IsMultiKey {
		return nil, errors.New("multi-key channels do not support balance protection")
	}
	if view.Enabled && len(freeModels) == 0 {
		return nil, errors.New("balance protection requires at least one free model")
	}
	exposedModels := make(map[string]struct{})
	for _, modelName := range channel.GetModels() {
		exposedModels[strings.TrimSpace(modelName)] = struct{}{}
	}
	for _, modelName := range freeModels {
		if _, ok := exposedModels[modelName]; !ok {
			return nil, fmt.Errorf("balance protection free model is not exposed by the channel: %s", modelName)
		}
	}
	return freeModels, nil
}

func saveChannelBalanceProtection(tx *gorm.DB, channel *Channel, view *ChannelBalanceProtectionView) (bool, error) {
	freeModels, err := validateChannelBalanceProtection(channel, view)
	if err != nil {
		return false, err
	}
	encodedModels, err := encodeBalanceProtectionModels(freeModels)
	if err != nil {
		return false, err
	}

	now := common.GetTimestamp()
	var existing ChannelBalanceProtection
	err = tx.Where("channel_id = ?", channel.Id).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	isNew := errors.Is(err, gorm.ErrRecordNotFound)
	needsImmediateCheck := false
	next := existing
	if isNew {
		next = ChannelBalanceProtection{
			ChannelId:          channel.Id,
			State:              BalanceProtectionStateDisabled,
			NotifyEnabled:      true,
			CreatedAt:          now,
			LastTransitionTime: now,
		}
	}

	wasEnabled := existing.Enabled
	thresholdChanged := !isNew &&
		(existing.TriggerBalance != view.TriggerBalance ||
			existing.RecoveryBalance != view.RecoveryBalance)

	next.Enabled = view.Enabled
	next.TriggerBalance = view.TriggerBalance
	next.RecoveryBalance = view.RecoveryBalance
	next.CheckIntervalMinutes = view.CheckIntervalMinutes
	next.FreeModels = encodedModels
	next.NotifyEnabled = view.NotifyEnabled
	next.UpdatedAt = now
	next.freeModelSet = nil

	if !view.Enabled {
		if next.State != BalanceProtectionStateDisabled {
			next.LastTransitionTime = now
		}
		next.State = BalanceProtectionStateDisabled
		next.ConsecutiveFailures = 0
		next.LastError = ""
	} else if !wasEnabled || thresholdChanged {
		if next.State != BalanceProtectionStatePending {
			next.LastTransitionTime = now
		}
		next.State = BalanceProtectionStatePending
		next.ConsecutiveFailures = 0
		next.LastError = ""
		needsImmediateCheck = true
	}

	if isNew {
		if err := tx.Create(&next).Error; err != nil {
			return false, err
		}
	} else if err := tx.Save(&next).Error; err != nil {
		return false, err
	}
	return needsImmediateCheck, nil
}

func GetChannelBalanceProtection(channelId int) (*ChannelBalanceProtection, error) {
	var protection ChannelBalanceProtection
	if err := DB.Where("channel_id = ?", channelId).First(&protection).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	protection.prepareFreeModelSet()
	return &protection, nil
}

func GetChannelBalanceProtections(channelIds []int) (map[int]*ChannelBalanceProtection, error) {
	result := make(map[int]*ChannelBalanceProtection, len(channelIds))
	if len(channelIds) == 0 {
		return result, nil
	}
	var protections []*ChannelBalanceProtection
	if err := DB.Where("channel_id IN ?", channelIds).Find(&protections).Error; err != nil {
		return nil, err
	}
	for _, protection := range protections {
		protection.prepareFreeModelSet()
		result[protection.ChannelId] = protection
	}
	return result, nil
}

func GetAllChannelBalanceProtections() ([]*ChannelBalanceProtection, error) {
	var protections []*ChannelBalanceProtection
	if err := DB.Find(&protections).Error; err != nil {
		return nil, err
	}
	for _, protection := range protections {
		protection.prepareFreeModelSet()
	}
	return protections, nil
}

func HasEnabledChannelBalanceProtection() bool {
	var count int64
	err := DB.Model(&ChannelBalanceProtection{}).Where("enabled = ?", true).Count(&count).Error
	return err == nil && count > 0
}

func ListDueBalanceProtectionChannels(now int64) ([]*Channel, error) {
	var protections []*ChannelBalanceProtection
	if err := DB.Where("enabled = ?", true).Find(&protections).Error; err != nil {
		return nil, err
	}
	channelIds := make([]int, 0, len(protections))
	for _, protection := range protections {
		intervalSeconds := int64(protection.CheckIntervalMinutes * 60)
		if protection.LastCheckTime == 0 || now-protection.LastCheckTime >= intervalSeconds {
			channelIds = append(channelIds, protection.ChannelId)
		}
	}
	if len(channelIds) == 0 {
		return []*Channel{}, nil
	}
	var channels []*Channel
	if err := DB.Where("id IN ? AND status = ?", channelIds, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, err
	}
	return channels, nil
}

func freeModelsIntersectChannel(protection *ChannelBalanceProtection, channel *Channel) bool {
	if protection == nil || channel == nil {
		return false
	}
	exposedModels := make(map[string]struct{})
	for _, modelName := range channel.GetModels() {
		exposedModels[strings.TrimSpace(modelName)] = struct{}{}
	}
	for _, modelName := range protection.GetFreeModels() {
		if _, ok := exposedModels[modelName]; ok {
			return true
		}
	}
	return false
}

func RecordChannelBalanceProtectionCheck(channelId int, balance *float64, checkError string) (*BalanceProtectionTransition, error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	var protection ChannelBalanceProtection
	query := lockForUpdate(tx).Where("channel_id = ?", channelId)
	if err := query.First(&protection).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if !protection.Enabled {
		tx.Rollback()
		return nil, nil
	}

	before := protection
	now := common.GetTimestamp()
	protection.LastCheckTime = now
	if checkError != "" {
		protection.ConsecutiveFailures++
		protection.LastError = checkError
		if protection.ConsecutiveFailures >= BalanceProtectionFailureLimit {
			protection.State = BalanceProtectionStateUnknown
		}
	} else {
		if balance == nil {
			tx.Rollback()
			return nil, errors.New("balance protection successful check requires balance")
		}
		var channel Channel
		if err := tx.First(&channel, "id = ?", channelId).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		protection.ConsecutiveFailures = 0
		protection.LastError = ""
		protection.LastSuccessTime = now
		if !freeModelsIntersectChannel(&protection, &channel) {
			protection.State = BalanceProtectionStateInvalidAllowlist
		} else if *balance < protection.TriggerBalance {
			protection.State = BalanceProtectionStateProtected
		} else if before.State == BalanceProtectionStateNormal {
			protection.State = BalanceProtectionStateNormal
		} else if *balance >= protection.RecoveryBalance {
			protection.State = BalanceProtectionStateNormal
		} else {
			protection.State = BalanceProtectionStateProtected
		}
	}
	if before.State != protection.State {
		protection.LastTransitionTime = now
	}
	protection.UpdatedAt = now
	protection.freeModelSet = nil
	if err := tx.Save(&protection).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &BalanceProtectionTransition{Before: &before, After: &protection}, nil
}

func ActivateChannelBalanceProtection(channelId int, reason string) (*BalanceProtectionTransition, error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	var protection ChannelBalanceProtection
	if err := lockForUpdate(tx).Where("channel_id = ?", channelId).First(&protection).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if !protection.Enabled {
		tx.Rollback()
		return nil, nil
	}
	before := protection
	now := common.GetTimestamp()
	protection.State = BalanceProtectionStateProtected
	protection.LastError = strings.TrimSpace(reason)
	protection.UpdatedAt = now
	if before.State != protection.State {
		protection.LastTransitionTime = now
	}
	if err := tx.Save(&protection).Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}
	return &BalanceProtectionTransition{Before: &before, After: &protection}, nil
}

func CopyChannelBalanceProtection(sourceChannelId int, targetChannelId int) error {
	source, err := GetChannelBalanceProtection(sourceChannelId)
	if err != nil || source == nil {
		return err
	}
	now := common.GetTimestamp()
	target := *source
	target.ChannelId = targetChannelId
	target.Enabled = false
	target.State = BalanceProtectionStateDisabled
	target.ConsecutiveFailures = 0
	target.LastCheckTime = 0
	target.LastSuccessTime = 0
	target.LastTransitionTime = now
	target.LastError = ""
	target.CreatedAt = now
	target.UpdatedAt = now
	target.freeModelSet = nil
	return DB.Create(&target).Error
}

func DeleteChannelBalanceProtection(tx *gorm.DB, channelIds []int) error {
	if len(channelIds) == 0 {
		return nil
	}
	return tx.Where("channel_id IN ?", channelIds).Delete(&ChannelBalanceProtection{}).Error
}
