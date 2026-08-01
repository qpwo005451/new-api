package controller

import (
	"errors"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type modelMonitorAlertConfig struct {
	Enabled                    bool                                      `json:"enabled"`
	EmailEnabled               bool                                      `json:"email_enabled"`
	EmailRecipients            string                                    `json:"email_recipients"`
	TelegramEnabled            bool                                      `json:"telegram_enabled"`
	TelegramBotToken           string                                    `json:"telegram_bot_token,omitempty"`
	TelegramBotTokenConfigured bool                                      `json:"telegram_bot_token_configured"`
	TelegramChatID             string                                    `json:"telegram_chat_id"`
	Rules                      []operation_setting.ModelMonitorAlertRule `json:"rules"`
}

func GetModelMonitorAlertConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": loadModelMonitorAlertConfig()})
}

func UpdateModelMonitorAlertConfig(c *gin.Context) {
	var request modelMonitorAlertConfig
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model monitor alert config payload"})
		return
	}
	existingToken := operation_setting.GetModelMonitorAlertSetting().TelegramNotifyBotToken
	if err := validateModelMonitorAlertConfig(request, existingToken); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := saveModelMonitorAlertConfig(request); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "model_monitor.alert_config_update", map[string]interface{}{"rule_count": len(request.Rules)})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": loadModelMonitorAlertConfig()})
}

func TestModelMonitorAlerts(c *gin.Context) {
	if !operation_setting.GetModelMonitorAlertSetting().Enabled {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "model monitor alerts are disabled"})
		return
	}
	result, success := service.TestModelMonitorAlertTransports(c.Request.Context())
	if !success {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "one or more model monitor notification transports failed",
			"data":    result,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func loadModelMonitorAlertConfig() modelMonitorAlertConfig {
	setting := operation_setting.GetModelMonitorAlertSetting()
	return modelMonitorAlertConfig{
		Enabled:                    setting.Enabled,
		EmailEnabled:               setting.EmailEnabled,
		EmailRecipients:            setting.EmailRecipients,
		TelegramEnabled:            setting.TelegramEnabled,
		TelegramBotTokenConfigured: strings.TrimSpace(setting.TelegramNotifyBotToken) != "",
		TelegramChatID:             setting.TelegramChatID,
		Rules:                      append([]operation_setting.ModelMonitorAlertRule(nil), setting.Rules...),
	}
}

func validateModelMonitorAlertConfig(config modelMonitorAlertConfig, existingTelegramToken string) error {
	const (
		maxEmailRecipientsLength = 512
		maxTelegramTokenLength   = 512
		maxTelegramChatIDLength  = 128
		maxAlertRules            = 100
		maxModelSelectorLength   = 255
	)
	if !config.Enabled {
		return nil
	}
	if !config.EmailEnabled && !config.TelegramEnabled {
		return errors.New("at least one model monitor notification transport must be enabled")
	}
	if config.EmailEnabled && strings.TrimSpace(config.EmailRecipients) == "" {
		return errors.New("model monitor alert email recipients are required")
	}
	if config.EmailEnabled {
		recipients := strings.TrimSpace(config.EmailRecipients)
		if len(recipients) > maxEmailRecipientsLength || strings.ContainsAny(recipients, "\r\n") {
			return errors.New("model monitor alert email recipients are invalid")
		}
		for _, recipient := range strings.Split(recipients, ";") {
			parsed, err := mail.ParseAddress(strings.TrimSpace(recipient))
			if err != nil || parsed.Address == "" {
				return errors.New("model monitor alert email recipients are invalid")
			}
		}
	}
	if config.TelegramEnabled {
		token := strings.TrimSpace(config.TelegramBotToken)
		if token == "" {
			token = strings.TrimSpace(existingTelegramToken)
		}
		if token == "" {
			return errors.New("model monitor Telegram bot token is required")
		}
		if len(token) > maxTelegramTokenLength {
			return errors.New("model monitor Telegram bot token is invalid")
		}
		chatID := strings.TrimSpace(config.TelegramChatID)
		if chatID == "" {
			return errors.New("model monitor Telegram chat id is required")
		}
		if len(chatID) > maxTelegramChatIDLength || strings.ContainsAny(chatID, "\r\n") {
			return errors.New("model monitor Telegram chat id is invalid")
		}
	}
	if len(config.Rules) > maxAlertRules {
		return errors.New("too many model monitor alert rules")
	}
	enabledRules := 0
	for _, rule := range config.Rules {
		if !rule.Enabled {
			continue
		}
		enabledRules++
		if rule.SiteID <= 0 || rule.ChannelID <= 0 {
			return errors.New("model monitor alert rule site and channel are required")
		}
		hasPrefix := strings.TrimSpace(rule.ModelPrefix) != ""
		hasName := strings.TrimSpace(rule.ModelName) != ""
		if hasPrefix == hasName {
			return errors.New("model monitor alert rule must have exactly one model selector")
		}
		if len(strings.TrimSpace(rule.ModelPrefix)) > maxModelSelectorLength ||
			len(strings.TrimSpace(rule.ModelName)) > maxModelSelectorLength {
			return errors.New("model monitor alert rule model selector is too long")
		}
	}
	if enabledRules == 0 {
		return errors.New("at least one model monitor alert rule must be enabled")
	}
	return nil
}

func saveModelMonitorAlertConfig(config modelMonitorAlertConfig) error {
	current := operation_setting.GetModelMonitorAlertSetting()
	token := strings.TrimSpace(config.TelegramBotToken)
	if token == "" {
		token = current.TelegramNotifyBotToken
	}
	rules := append([]operation_setting.ModelMonitorAlertRule(nil), config.Rules...)
	for index := range rules {
		rules[index].ModelPrefix = strings.TrimSpace(rules[index].ModelPrefix)
		rules[index].ModelName = strings.TrimSpace(rules[index].ModelName)
	}
	rulesJSON, err := common.Marshal(rules)
	if err != nil {
		return err
	}
	optionValues := map[string]string{
		"model_monitor_alert_setting.enabled":                strconv.FormatBool(config.Enabled),
		"model_monitor_alert_setting.email_enabled":          strconv.FormatBool(config.EmailEnabled),
		"model_monitor_alert_setting.email_recipients":       strings.TrimSpace(config.EmailRecipients),
		"model_monitor_alert_setting.telegram_enabled":       strconv.FormatBool(config.TelegramEnabled),
		"model_monitor_alert_setting.TelegramNotifyBotToken": token,
		"model_monitor_alert_setting.telegram_chat_id":       strings.TrimSpace(config.TelegramChatID),
		"model_monitor_alert_setting.rules":                  string(rulesJSON),
	}

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			var relationCount int64
			if err := tx.Model(&model.ModelMonitorSiteChannel{}).
				Where("site_id = ? AND channel_id = ?", rule.SiteID, rule.ChannelID).
				Count(&relationCount).Error; err != nil {
				return err
			}
			if relationCount == 0 {
				return errors.New("model monitor alert rule does not reference a configured site channel")
			}
		}
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
		return nil
	})
	if err != nil {
		return err
	}
	*current = operation_setting.ModelMonitorAlertSetting{
		Enabled:                config.Enabled,
		EmailEnabled:           config.EmailEnabled,
		EmailRecipients:        strings.TrimSpace(config.EmailRecipients),
		TelegramEnabled:        config.TelegramEnabled,
		TelegramNotifyBotToken: token,
		TelegramChatID:         strings.TrimSpace(config.TelegramChatID),
		Rules:                  rules,
	}
	return nil
}
