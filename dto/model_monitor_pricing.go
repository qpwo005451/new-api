package dto

type ModelMonitorPricingImportRequest struct {
	SiteName                string                           `json:"site_name"`
	SiteType                string                           `json:"site_type"`
	PricingVersion          string                           `json:"pricing_version"`
	QuotaPerUnit            *float64                         `json:"quota_per_unit,omitempty"`
	PricingGroup            string                           `json:"pricing_group,omitempty"`
	GroupRatio              map[string]float64               `json:"group_ratio"`
	Models                  []ModelMonitorPricingImportModel `json:"models"`
	Sub2APIChannels         []ModelMonitorSub2APIChannel     `json:"sub2api_channels,omitempty"`
	Sub2APICustomGroupRates map[string]float64               `json:"sub2api_custom_group_rates,omitempty"`
}

type ModelMonitorActualCostImportRequest struct {
	SiteName string                         `json:"site_name"`
	Records  []ModelMonitorActualCostRecord `json:"records"`
}

type ModelMonitorActualCostRecord struct {
	RequestID  string  `json:"request_id"`
	ActualCost float64 `json:"actual_cost"`
}

type ModelMonitorPricingImportModel struct {
	ModelName              string   `json:"model_name"`
	QuotaType              int      `json:"quota_type"`
	ModelRatio             *float64 `json:"model_ratio,omitempty"`
	ModelPrice             *float64 `json:"model_price,omitempty"`
	CompletionRatio        *float64 `json:"completion_ratio,omitempty"`
	CacheRatio             *float64 `json:"cache_ratio,omitempty"`
	CreateCacheRatio       *float64 `json:"create_cache_ratio,omitempty"`
	ImageRatio             *float64 `json:"image_ratio,omitempty"`
	AudioRatio             *float64 `json:"audio_ratio,omitempty"`
	AudioCompletionRatio   *float64 `json:"audio_completion_ratio,omitempty"`
	EnableGroups           []string `json:"enable_groups,omitempty"`
	SupportedEndpointTypes []string `json:"supported_endpoint_types,omitempty"`
}

type ModelMonitorSub2APIChannel struct {
	Name      string                               `json:"name"`
	Platforms []ModelMonitorSub2APIPlatformSection `json:"platforms"`
}

type ModelMonitorSub2APIPlatformSection struct {
	Platform        string                     `json:"platform"`
	Groups          []ModelMonitorSub2APIGroup `json:"groups"`
	SupportedModels []ModelMonitorSub2APIModel `json:"supported_models"`
}

type ModelMonitorSub2APIGroup struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	RateMultiplier     float64 `json:"rate_multiplier"`
	PeakRateEnabled    bool    `json:"peak_rate_enabled"`
	PeakStart          string  `json:"peak_start"`
	PeakEnd            string  `json:"peak_end"`
	PeakRateMultiplier float64 `json:"peak_rate_multiplier"`
}

type ModelMonitorSub2APIModel struct {
	Name     string                           `json:"name"`
	Platform string                           `json:"platform"`
	Pricing  *ModelMonitorSub2APIModelPricing `json:"pricing"`
}

type ModelMonitorSub2APIModelPricing struct {
	BillingMode      string                            `json:"billing_mode"`
	InputPrice       *float64                          `json:"input_price"`
	OutputPrice      *float64                          `json:"output_price"`
	CacheWritePrice  *float64                          `json:"cache_write_price"`
	CacheReadPrice   *float64                          `json:"cache_read_price"`
	ImageOutputPrice *float64                          `json:"image_output_price"`
	PerRequestPrice  *float64                          `json:"per_request_price"`
	Intervals        []ModelMonitorSub2APIPricingRange `json:"intervals"`
}

type ModelMonitorSub2APIPricingRange struct {
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens"`
	InputPrice      *float64 `json:"input_price"`
	OutputPrice     *float64 `json:"output_price"`
	CacheWritePrice *float64 `json:"cache_write_price"`
	CacheReadPrice  *float64 `json:"cache_read_price"`
	PerRequestPrice *float64 `json:"per_request_price"`
}
