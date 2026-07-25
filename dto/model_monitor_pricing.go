package dto

type ModelMonitorPricingImportRequest struct {
	SiteName       string                           `json:"site_name"`
	SiteType       string                           `json:"site_type"`
	PricingVersion string                           `json:"pricing_version"`
	GroupRatio     map[string]float64               `json:"group_ratio"`
	Models         []ModelMonitorPricingImportModel `json:"models"`
}

type ModelMonitorPricingImportModel struct {
	ModelName            string   `json:"model_name"`
	QuotaType            int      `json:"quota_type"`
	ModelRatio           *float64 `json:"model_ratio,omitempty"`
	ModelPrice           *float64 `json:"model_price,omitempty"`
	CompletionRatio      *float64 `json:"completion_ratio,omitempty"`
	CacheRatio           *float64 `json:"cache_ratio,omitempty"`
	CreateCacheRatio     *float64 `json:"create_cache_ratio,omitempty"`
	ImageRatio           *float64 `json:"image_ratio,omitempty"`
	AudioRatio           *float64 `json:"audio_ratio,omitempty"`
	AudioCompletionRatio *float64 `json:"audio_completion_ratio,omitempty"`
	EnableGroups         []string `json:"enable_groups,omitempty"`
}
