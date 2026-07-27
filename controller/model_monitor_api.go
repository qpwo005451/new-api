package controller

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type modelMonitorConfigResponse struct {
	Setting operation_setting.ModelMonitorSetting `json:"setting"`
	Sites   []modelMonitorSiteConfig              `json:"sites"`
}

type modelMonitorSiteConfig struct {
	ID           int64                      `json:"id,omitempty"`
	Name         string                     `json:"name"`
	SiteType     string                     `json:"site_type"`
	PricingGroup string                     `json:"pricing_group,omitempty"`
	Enabled      bool                       `json:"enabled"`
	ChannelIDs   []int                      `json:"channel_ids"`
	Targets      []modelMonitorTargetConfig `json:"targets"`
}

type modelMonitorTargetConfig struct {
	ID           int64  `json:"id,omitempty"`
	ModelName    string `json:"model_name"`
	EndpointType string `json:"endpoint_type"`
	Weight       int    `json:"weight"`
	Enabled      bool   `json:"enabled"`
}

type modelMonitorSiteAPIResponse struct {
	Site             model.ModelMonitorSite          `json:"site"`
	Summary          model.ModelMonitorSiteSummary   `json:"summary"`
	ChannelIDs       []int                           `json:"channel_ids"`
	LatestObservedAt int64                           `json:"latest_observed_at"`
	FreshnessSeconds *int64                          `json:"freshness_seconds"`
	Observations     []model.ModelMonitorObservation `json:"observations,omitempty"`
}

func GetModelMonitorSummary(c *gin.Context) {
	sites, err := buildModelMonitorSiteResponses(false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled": operation_setting.GetModelMonitorSetting().Enabled,
			"sites":   sites,
		},
	})
}

func GetModelMonitorSites(c *gin.Context) {
	sites, err := buildModelMonitorSiteResponses(false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": sites})
}

func GetModelMonitorSite(c *gin.Context) {
	siteID, err := strconv.ParseInt(c.Param("site_id"), 10, 64)
	if err != nil || siteID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model monitor site id"})
		return
	}
	response, err := buildModelMonitorSiteResponse(siteID, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "model monitor site not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func GetModelMonitorModel(c *gin.Context) {
	siteID, err := strconv.ParseInt(c.Param("site_id"), 10, 64)
	if err != nil || siteID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model monitor site id"})
		return
	}
	modelName, err := url.PathUnescape(c.Param("model"))
	if err != nil || strings.TrimSpace(modelName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model monitor model"})
		return
	}

	var site model.ModelMonitorSite
	if err := model.DB.First(&site, siteID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "model monitor site not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	var target model.ModelMonitorTarget
	if err := model.DB.Where("site_id = ? AND model_name = ?", siteID, modelName).First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "model monitor target not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	observations := make([]model.ModelMonitorObservation, 0)
	if err := model.DB.Where("site_id = ? AND model_name = ?", siteID, modelName).
		Order("observed_at DESC, id DESC").
		Limit(200).
		Find(&observations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	summary := model.BuildModelMonitorSiteSummary(
		[]model.ModelMonitorTarget{target},
		observations,
		common.GetTimestamp(),
		int64(operation_setting.GetModelMonitorSetting().UnknownGraceMinutes*60),
	)
	var priceSnapshot model.ModelMonitorPriceSnapshot
	priceSnapshotQuery := model.DB.Where(
		"site_id = ? AND model_name = ? AND source = ?",
		siteID,
		modelName,
		model.ModelMonitorPriceSourceUpstreamCatalog,
	).Order("captured_at DESC, id DESC").Limit(1).Find(&priceSnapshot)
	if priceSnapshotQuery.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": priceSnapshotQuery.Error.Error()})
		return
	}
	var pricingMetadata any
	if priceSnapshot.ID > 0 {
		pricingMetadata = gin.H{
			"snapshot_id":   priceSnapshot.ID,
			"source":        priceSnapshot.Source,
			"version":       priceSnapshot.Version,
			"model_family":  priceSnapshot.ModelFamily,
			"modality":      priceSnapshot.Modality,
			"billing_class": priceSnapshot.BillingClass,
			"captured_at":   priceSnapshot.CapturedAt,
		}
	}
	aggregates := make([]model.ModelMonitorAggregateHourly, 0)
	if err := model.DB.Where("site_id = ? AND model_name = ? AND hour_start >= ?", siteID, modelName, common.GetTimestamp()-7*24*60*60).
		Order("hour_start ASC, channel_id ASC").
		Find(&aggregates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"site":         site,
			"target":       target,
			"summary":      summary,
			"pricing":      pricingMetadata,
			"aggregates":   aggregates,
			"observations": observations,
		},
	})
}

func GetModelMonitorConfig(c *gin.Context) {
	config, err := loadModelMonitorConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": config})
}

func UpdateModelMonitorConfig(c *gin.Context) {
	var request modelMonitorConfigResponse
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model monitor config payload"})
		return
	}
	if err := validateModelMonitorConfig(request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := saveModelMonitorConfig(request); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "model_monitor.config_update", map[string]interface{}{"site_count": len(request.Sites)})
	config, err := loadModelMonitorConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": config})
}

func EnqueueModelMonitorRun(c *gin.Context) {
	if !operation_setting.GetModelMonitorSetting().Enabled {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "model monitor is disabled"})
		return
	}
	task, created, err := enqueueModelMonitorRun()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"data": gin.H{
			"task_id": task.TaskID,
			"created": created,
		},
	})
}

func buildModelMonitorSiteResponses(includeHistory bool) ([]modelMonitorSiteAPIResponse, error) {
	sites := make([]model.ModelMonitorSite, 0)
	if err := model.DB.Where("enabled = ?", true).Order("id ASC").Find(&sites).Error; err != nil {
		return nil, err
	}
	responses := make([]modelMonitorSiteAPIResponse, 0, len(sites))
	for _, site := range sites {
		response, err := buildModelMonitorSiteResponse(site.ID, includeHistory)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func buildModelMonitorSiteResponse(siteID int64, includeHistory bool) (modelMonitorSiteAPIResponse, error) {
	var site model.ModelMonitorSite
	if err := model.DB.First(&site, siteID).Error; err != nil {
		return modelMonitorSiteAPIResponse{}, err
	}
	targets := make([]model.ModelMonitorTarget, 0)
	if err := model.DB.Where("site_id = ? AND enabled = ?", siteID, true).Order("model_name ASC").Find(&targets).Error; err != nil {
		return modelMonitorSiteAPIResponse{}, err
	}
	observations := make([]model.ModelMonitorObservation, 0)
	query := model.DB.Where("site_id = ?", siteID).Order("observed_at DESC, id DESC")
	if includeHistory {
		query = query.Limit(200)
	}
	if err := query.Find(&observations).Error; err != nil {
		return modelMonitorSiteAPIResponse{}, err
	}
	current := latestModelMonitorPathObservations(observations)
	channelIDs := make([]int, 0)
	if err := model.DB.Model(&model.ModelMonitorSiteChannel{}).
		Where("site_id = ?", siteID).
		Order("channel_id ASC").
		Pluck("channel_id", &channelIDs).Error; err != nil {
		return modelMonitorSiteAPIResponse{}, err
	}

	response := modelMonitorSiteAPIResponse{
		Site:       site,
		Summary:    model.BuildModelMonitorSiteSummary(targets, observations, common.GetTimestamp(), int64(operation_setting.GetModelMonitorSetting().UnknownGraceMinutes*60)),
		ChannelIDs: channelIDs,
	}
	for _, observation := range current {
		if observation.ObservedAt > response.LatestObservedAt {
			response.LatestObservedAt = observation.ObservedAt
		}
	}
	if response.LatestObservedAt > 0 {
		freshness := common.GetTimestamp() - response.LatestObservedAt
		if freshness < 0 {
			freshness = 0
		}
		response.FreshnessSeconds = &freshness
	}
	if includeHistory {
		response.Observations = observations
	}
	return response, nil
}

func latestModelMonitorPathObservations(observations []model.ModelMonitorObservation) []model.ModelMonitorObservation {
	latest := make([]model.ModelMonitorObservation, 0)
	seen := make(map[[2]int64]struct{})
	for _, observation := range observations {
		key := [2]int64{observation.TargetID, int64(observation.ChannelID)}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		latest = append(latest, observation)
	}
	return latest
}

func loadModelMonitorConfig() (modelMonitorConfigResponse, error) {
	response := modelMonitorConfigResponse{
		Setting: *operation_setting.GetModelMonitorSetting(),
		Sites:   make([]modelMonitorSiteConfig, 0),
	}
	sites := make([]model.ModelMonitorSite, 0)
	if err := model.DB.Order("id ASC").Find(&sites).Error; err != nil {
		return response, err
	}
	for _, site := range sites {
		config := modelMonitorSiteConfig{
			ID:           site.ID,
			Name:         site.Name,
			SiteType:     site.SiteType,
			PricingGroup: site.PricingGroup,
			Enabled:      site.Enabled,
			ChannelIDs:   make([]int, 0),
			Targets:      make([]modelMonitorTargetConfig, 0),
		}
		if err := model.DB.Model(&model.ModelMonitorSiteChannel{}).
			Where("site_id = ?", site.ID).
			Order("channel_id ASC").
			Pluck("channel_id", &config.ChannelIDs).Error; err != nil {
			return response, err
		}
		targets := make([]model.ModelMonitorTarget, 0)
		if err := model.DB.Where("site_id = ?", site.ID).Order("model_name ASC").Find(&targets).Error; err != nil {
			return response, err
		}
		hasEnabledTarget := false
		for _, target := range targets {
			hasEnabledTarget = hasEnabledTarget || target.Enabled
			config.Targets = append(config.Targets, modelMonitorTargetConfig{
				ID:           target.ID,
				ModelName:    target.ModelName,
				EndpointType: target.EndpointType,
				Weight:       target.Weight,
				Enabled:      target.Enabled,
			})
		}
		if !site.Enabled && len(config.ChannelIDs) == 0 && !hasEnabledTarget {
			continue
		}
		response.Sites = append(response.Sites, config)
	}
	return response, nil
}

func validateModelMonitorConfig(config modelMonitorConfigResponse) error {
	if config.Setting.AutoProbeIntervalMinutes < 1 {
		return errors.New("model monitor auto probe interval must be positive")
	}
	if config.Setting.UnknownGraceMinutes < 1 {
		return errors.New("model monitor unknown grace must be positive")
	}
	siteNames := make(map[string]struct{}, len(config.Sites))
	assignedChannelIDs := make(map[int]string)
	for _, site := range config.Sites {
		name := strings.TrimSpace(site.Name)
		if name == "" {
			return errors.New("model monitor site name is required")
		}
		if _, ok := siteNames[name]; ok {
			return errors.New("model monitor site name must be unique")
		}
		siteNames[name] = struct{}{}
		if site.SiteType != model.ModelMonitorSiteTypeNewAPI && site.SiteType != model.ModelMonitorSiteTypeSub2API {
			return errors.New("model monitor site type is unsupported")
		}
		channelIDs := make(map[int]struct{}, len(site.ChannelIDs))
		for _, channelID := range site.ChannelIDs {
			if channelID <= 0 {
				return errors.New("model monitor channel id is invalid")
			}
			if _, ok := channelIDs[channelID]; ok {
				return errors.New("model monitor channel id must be unique per site")
			}
			if assignedSite, ok := assignedChannelIDs[channelID]; ok {
				return errors.New("model monitor channel id is already assigned to site " + assignedSite)
			}
			channelIDs[channelID] = struct{}{}
			assignedChannelIDs[channelID] = name
		}
		targetNames := make(map[string]struct{}, len(site.Targets))
		for _, target := range site.Targets {
			candidate := model.ModelMonitorTarget{
				SiteID:    1,
				ModelName: target.ModelName,
				Weight:    target.Weight,
			}
			if err := model.ValidateModelMonitorTarget(candidate); err != nil {
				return err
			}
			if _, ok := targetNames[target.ModelName]; ok {
				return errors.New("model monitor target model must be unique per site")
			}
			targetNames[target.ModelName] = struct{}{}
		}
	}
	return nil
}

func saveModelMonitorConfig(config modelMonitorConfigResponse) error {
	pricingImportUserIDs, err := common.Marshal(config.Setting.PricingImportUserIDs)
	if err != nil {
		return err
	}
	optionValues := map[string]string{
		"model_monitor_setting.enabled":                     strconv.FormatBool(config.Setting.Enabled),
		"model_monitor_setting.auto_probe_enabled":          strconv.FormatBool(config.Setting.AutoProbeEnabled),
		"model_monitor_setting.auto_probe_interval_minutes": strconv.Itoa(config.Setting.AutoProbeIntervalMinutes),
		"model_monitor_setting.unknown_grace_minutes":       strconv.Itoa(config.Setting.UnknownGraceMinutes),
		"model_monitor_setting.pricing_import_user_ids":     string(pricingImportUserIDs),
	}

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		for key, value := range optionValues {
			option := model.Option{Key: key}
			if err := tx.FirstOrCreate(&option, model.Option{Key: key}).Error; err != nil {
				return err
			}
			option.Value = value
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}

		configuredSiteIDs := make([]int64, 0, len(config.Sites))
		for _, siteConfig := range config.Sites {
			site, err := upsertModelMonitorSite(tx, siteConfig)
			if err != nil {
				return err
			}
			configuredSiteIDs = append(configuredSiteIDs, site.ID)
			if err := replaceModelMonitorSiteChannels(tx, site.ID, siteConfig.ChannelIDs); err != nil {
				return err
			}
			if err := upsertModelMonitorTargets(tx, site.ID, siteConfig.Targets); err != nil {
				return err
			}
		}
		query := tx.Model(&model.ModelMonitorSite{})
		if len(configuredSiteIDs) > 0 {
			query = query.Where("id NOT IN ?", configuredSiteIDs)
		}
		removedSiteIDs := make([]int64, 0)
		if err := query.Pluck("id", &removedSiteIDs).Error; err != nil {
			return err
		}
		if len(removedSiteIDs) == 0 {
			return nil
		}
		if err := tx.Model(&model.ModelMonitorSite{}).
			Where("id IN ?", removedSiteIDs).
			Update("enabled", false).Error; err != nil {
			return err
		}
		if err := tx.Where("site_id IN ?", removedSiteIDs).
			Delete(&model.ModelMonitorSiteChannel{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ModelMonitorTarget{}).
			Where("site_id IN ?", removedSiteIDs).
			Update("enabled", false).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	*operation_setting.GetModelMonitorSetting() = config.Setting
	return nil
}

func upsertModelMonitorSite(tx *gorm.DB, config modelMonitorSiteConfig) (model.ModelMonitorSite, error) {
	site := model.ModelMonitorSite{}
	var err error
	if config.ID > 0 {
		err = tx.First(&site, config.ID).Error
	} else {
		err = tx.Where("name = ?", strings.TrimSpace(config.Name)).First(&site).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		site = model.ModelMonitorSite{}
	} else if err != nil {
		return site, err
	}
	site.Name = strings.TrimSpace(config.Name)
	site.SiteType = config.SiteType
	site.PricingGroup = strings.TrimSpace(config.PricingGroup)
	site.Enabled = config.Enabled
	site.UpdatedAt = common.GetTimestamp()
	if site.ID == 0 {
		err = tx.Create(&site).Error
	} else {
		err = tx.Save(&site).Error
	}
	return site, err
}

func replaceModelMonitorSiteChannels(tx *gorm.DB, siteID int64, channelIDs []int) error {
	if err := tx.Where("site_id = ?", siteID).Delete(&model.ModelMonitorSiteChannel{}).Error; err != nil {
		return err
	}
	for _, channelID := range channelIDs {
		var count int64
		if err := tx.Model(&model.Channel{}).Where("id = ?", channelID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New("model monitor channel does not exist")
		}
		if err := tx.Create(&model.ModelMonitorSiteChannel{SiteID: siteID, ChannelID: channelID}).Error; err != nil {
			return err
		}
	}
	return nil
}

func upsertModelMonitorTargets(tx *gorm.DB, siteID int64, targets []modelMonitorTargetConfig) error {
	configuredTargetIDs := make([]int64, 0, len(targets))
	for _, targetConfig := range targets {
		target := model.ModelMonitorTarget{}
		var err error
		if targetConfig.ID > 0 {
			err = tx.Where("id = ? AND site_id = ?", targetConfig.ID, siteID).First(&target).Error
		} else {
			err = tx.Where("site_id = ? AND model_name = ?", siteID, strings.TrimSpace(targetConfig.ModelName)).First(&target).Error
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			target = model.ModelMonitorTarget{SiteID: siteID}
		} else if err != nil {
			return err
		}
		target.ModelName = strings.TrimSpace(targetConfig.ModelName)
		target.EndpointType = strings.TrimSpace(targetConfig.EndpointType)
		target.Weight = targetConfig.Weight
		target.Enabled = targetConfig.Enabled
		target.UpdatedAt = common.GetTimestamp()
		if target.ID == 0 {
			err = tx.Create(&target).Error
		} else {
			err = tx.Save(&target).Error
		}
		if err != nil {
			return err
		}
		configuredTargetIDs = append(configuredTargetIDs, target.ID)
	}
	query := tx.Model(&model.ModelMonitorTarget{}).Where("site_id = ?", siteID)
	if len(configuredTargetIDs) > 0 {
		query = query.Where("id NOT IN ?", configuredTargetIDs)
	}
	return query.Update("enabled", false).Error
}
