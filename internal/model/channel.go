package model

import (
	"time"
)

// ChannelProvider 表示渠道使用的上游服务提供方。
type ChannelProvider string

const (
	ChannelProviderOpenAI          ChannelProvider = "openai"
	ChannelProviderOpenAIResponses ChannelProvider = "openai_responses"
	ChannelProviderAnthropic       ChannelProvider = "anthropic"
	ChannelProviderGemini          ChannelProvider = "gemini"
	ChannelProviderVolcengine      ChannelProvider = "volcengine"
)

// Channel 保存单个上游渠道的连接和转发配置。
type Channel struct {
	ID            int             `json:"id" gorm:"primaryKey"`                        // 渠道主键。
	Name          string          `json:"name" gorm:"unique;not null"`                 // 渠道名称。
	Type          ChannelProvider `json:"type"`                                        // 上游服务提供方。
	Enabled       bool            `json:"enabled" gorm:"default:true"`                 // Enabled 表示渠道是否可用。
	BaseURL       string          `json:"base_url"`                                    // 单一上游基础地址。
	Key           string          `json:"key"`                                         // 单一上游访问凭据。
	BaseUrls      []BaseUrl       `json:"base_urls,omitempty" gorm:"serializer:json"`  // 多上游基础地址(可选,非空时优先于 BaseURL)。
	Keys          []ChannelKey    `json:"keys,omitempty" gorm:"foreignKey:ChannelID"`  // 多上游访问凭据(可选,非空时优先于 Key)。
	Model         string          `json:"model"`                                       // 自动同步的模型列表。
	CustomModel   string          `json:"custom_model"`                                // 手动配置的模型列表。
	Proxy         bool            `json:"proxy" gorm:"default:false"`                  // Proxy 表示是否使用代理。
	AutoSync      bool            `json:"auto_sync" gorm:"default:false"`              // AutoSync 表示是否自动同步模型。
	CustomHeader  []CustomHeader  `json:"custom_header" gorm:"serializer:json"`        // 追加到上游请求的 Header。
	ParamOverride *string         `json:"param_override"`                              // 请求参数覆盖配置。
	ChannelProxy  *string         `json:"channel_proxy"`                               // 渠道专用代理地址。
	Stats         *StatsChannel   `json:"stats,omitempty" gorm:"foreignKey:ChannelID"` // 渠道统计信息。
	MatchRegex    *string         `json:"match_regex"`                                 // 模型同步过滤表达式。
}

// BaseUrl 表示渠道的一个上游基础地址及其测速延迟。
type BaseUrl struct {
	URL   string `json:"url"`   // 上游基础地址。
	Delay int    `json:"delay"` // 测速延迟(毫秒),越小越优先。
}

// ChannelKey 表示渠道的一个上游访问凭据及其使用状态。
type ChannelKey struct {
	ID               int     `json:"id" gorm:"primaryKey"`                          // 凭据主键。
	ChannelID        int     `json:"channel_id" gorm:"index"`                       // 所属渠道 ID。
	Enabled          bool    `json:"enabled" gorm:"default:true"`                   // Enabled 表示该凭据是否可用。
	ChannelKey       string  `json:"channel_key"`                                   // 凭据内容。
	StatusCode       int     `json:"status_code"`                                   // 最近一次使用的响应状态码,429 触发临时冷却。
	LastUseTimeStamp int64   `json:"last_use_time_stamp"`                           // 最近一次使用的 Unix 秒级时间戳。
	TotalCost        float64 `json:"total_cost"`                                    // 该凭据累计消费金额,选择时取最低。
	Remark           string  `json:"remark"`                                        // 备注。
}

// CustomHeader 表示追加到上游请求的单个 Header。
type CustomHeader struct {
	HeaderKey   string `json:"header_key"`   // Header 名称。
	HeaderValue string `json:"header_value"` // Header 值。
}

// GetEffectiveURL 返回实际使用的上游基础地址;配置了多地址时取延迟最低的。
func (c *Channel) GetEffectiveURL() string {
	if c == nil || len(c.BaseUrls) == 0 {
		return c.BaseURL
	}
	best := ""
	bestDelay := 0
	bestSet := false
	for _, bu := range c.BaseUrls {
		if bu.URL == "" {
			continue
		}
		if !bestSet || bu.Delay < bestDelay {
			best = bu.URL
			bestDelay = bu.Delay
			bestSet = true
		}
	}
	if !bestSet {
		return c.BaseURL
	}
	return best
}

// GetEffectiveKey 返回实际使用的访问凭据条目;配置了多凭据时跳过禁用和 429 冷却中的,
// 在剩余凭据里取累计消费最低的;未配置多凭据时用单一 Key 字段构造返回。
func (c *Channel) GetEffectiveKey() ChannelKey {
	if c == nil {
		return ChannelKey{}
	}
	if len(c.Keys) > 0 {
		nowSec := time.Now().Unix()
		best := ChannelKey{}
		bestSet := false
		for _, k := range c.Keys {
			if !k.Enabled || k.ChannelKey == "" {
				continue
			}
			if k.StatusCode == 429 && k.LastUseTimeStamp > 0 && nowSec-k.LastUseTimeStamp < int64(5*time.Minute/time.Second) {
				continue
			}
			if !bestSet || k.TotalCost < best.TotalCost {
				best = k
				bestSet = true
			}
		}
		if bestSet {
			return best
		}
	}
	return ChannelKey{ChannelKey: c.Key}
}

// TouchKey 更新指定凭据条目的最近使用时间和状态码,并同步单一 Key 字段的使用记录。
func (c *Channel) TouchKey(keyID int, statusCode int) {
	nowSec := time.Now().Unix()
	for i := range c.Keys {
		if c.Keys[i].ID == keyID {
			c.Keys[i].LastUseTimeStamp = nowSec
			c.Keys[i].StatusCode = statusCode
			return
		}
	}
}

// ChannelUpdateRequest 渠道更新请求 - 仅包含变更的数据
type ChannelUpdateRequest struct {
	ID            int              `json:"id" binding:"required"`    // 待更新渠道的主键。
	Name          *string          `json:"name,omitempty"`           // 新的渠道名称。
	Type          *ChannelProvider `json:"type,omitempty"`           // 新的上游服务提供方。
	Enabled       *bool            `json:"enabled,omitempty"`        // 新的启用状态。
	BaseURL       *string          `json:"base_url,omitempty"`       // 新的上游基础地址。
	Key           *string          `json:"key,omitempty"`            // 新的上游访问凭据。
	BaseUrls      *[]BaseUrl       `json:"base_urls,omitempty"`      // 新的多上游基础地址列表(整体替换)。
	Model         *string          `json:"model,omitempty"`          // 新的自动同步模型列表。
	CustomModel   *string          `json:"custom_model,omitempty"`   // 新的自定义模型列表。
	Proxy         *bool            `json:"proxy,omitempty"`          // 新的代理开关。
	AutoSync      *bool            `json:"auto_sync,omitempty"`      // 新的自动同步开关。
	CustomHeader  *[]CustomHeader  `json:"custom_header,omitempty"`  // 新的自定义 Header。
	ChannelProxy  *string          `json:"channel_proxy,omitempty"`  // 新的渠道代理地址。
	ParamOverride *string          `json:"param_override,omitempty"` // 新的参数覆盖配置。
	MatchRegex    *string          `json:"match_regex,omitempty"`    // 新的模型过滤表达式。

	KeysToAdd    []ChannelKeyAddRequest    `json:"keys_to_add,omitempty"`    // 待新增的凭据。
	KeysToUpdate []ChannelKeyUpdateRequest `json:"keys_to_update,omitempty"` // 待更新的凭据。
	KeysToDelete []int                     `json:"keys_to_delete,omitempty"` // 待删除的凭据 ID。
}

// ChannelKeyAddRequest 表示新增渠道凭据请求。
type ChannelKeyAddRequest struct {
	Enabled    bool   `json:"enabled"`                        // 是否启用该凭据。
	ChannelKey string `json:"channel_key" binding:"required"` // 凭据内容。
	Remark     string `json:"remark"`                         // 备注。
}

// ChannelKeyUpdateRequest 表示更新渠道凭据请求。
type ChannelKeyUpdateRequest struct {
	ID         int     `json:"id" binding:"required"`   // 待更新凭据的主键。
	Enabled    *bool   `json:"enabled,omitempty"`       // 新的启用状态。
	ChannelKey *string `json:"channel_key,omitempty"`   // 新的凭据内容。
	Remark     *string `json:"remark,omitempty"`        // 新的备注。
}
