package model

// GroupMode 表示分组的渠道选择模式。
type GroupMode int

const (
	GroupModeManual     GroupMode = 0 // 手动：仅使用 ActiveItemID 指定的渠道(默认,兼容旧数据)
	GroupModeRoundRobin GroupMode = 1 // 轮询:依次循环选择渠道
	GroupModeRandom     GroupMode = 2 // 随机:每次随机选择一个渠道
	GroupModeFailover   GroupMode = 3 // 故障转移:按优先级选择,失败时降级到下一个
	GroupModeWeighted   GroupMode = 4 // 加权分配:按权重分配流量
)

// Group 表示客户端模型名称及其可手动切换的上游分组。
type Group struct {
	ID                int         `json:"id" gorm:"primaryKey"`                      // ID 是分组主键。
	Name              string      `json:"name" gorm:"unique;not null"`              // Name 是客户端请求使用的模型名称。
	ActiveItemID      int         `json:"active_item_id" gorm:"not null;default:0"`  // ActiveItemID 是当前手动选中的分组项 ID，0 表示尚未指定；自动模式下未熔断时优先使用。
	Mode              GroupMode   `json:"mode" gorm:"not null;default:0"`            // Mode 是渠道选择模式，0 为手动档(与旧行为一致)。
	RetryInterval     int         `json:"retry_interval" gorm:"not null;default:1"`  // RetryInterval 是上游失败后的重试等待秒数，最小为 1。
	SessionKeepTime   int         `json:"session_keep_time" gorm:"not null;default:0"` // SessionKeepTime 是会话保持时间(秒)，0 表示禁用。
	Items             []GroupItem `json:"items,omitempty" gorm:"foreignKey:GroupID"` // Items 是该分组可手动选择的渠道模型。
}

// GroupItem 表示分组内一个可手动选择的渠道模型。
type GroupItem struct {
	ID        int    `json:"id" gorm:"primaryKey"`                                            // ID 是分组项主键。
	GroupID   int    `json:"group_id" gorm:"not null;index:idx_group_channel_model,unique"`   // GroupID 是所属分组 ID，创建分组时无需携带，更新时需要。
	ChannelID int    `json:"channel_id" gorm:"not null;index:idx_group_channel_model,unique"` // ChannelID 是实际上游渠道 ID。
	ModelName string `json:"model_name" gorm:"not null;index:idx_group_channel_model,unique"` // ModelName 是该渠道实际请求的模型名称。
	Priority  int    `json:"priority"`                                                         // Priority 是 Failover 模式下的优先级(越小越优先)和界面展示顺序。
	Weight    int    `json:"weight"`                                                           // Weight 是 Weighted 模式下的分配权重(小于 1 时按 1 处理)。
}

// GroupUpdateRequest 表示分组普通配置和成员变更请求。
type GroupUpdateRequest struct {
	ID              int                      `json:"id" binding:"required"`      // ID 是待更新的分组主键。
	Name            *string                  `json:"name,omitempty"`             // Name 仅在名称变更时发送。
	Mode            *GroupMode               `json:"mode,omitempty"`             // Mode 仅在选择模式变更时发送。
	RetryInterval   *int                     `json:"retry_interval,omitempty" binding:"omitempty,min=1"` // RetryInterval 仅在重试间隔变更时发送，单位为秒。
	SessionKeepTime *int                     `json:"session_keep_time,omitempty"` // SessionKeepTime 仅在会话保持时间变更时发送，单位为秒。
	ItemsToAdd      []GroupItemAddRequest    `json:"items_to_add,omitempty"`     // ItemsToAdd 是待新增的分组项。
	ItemsToUpdate   []GroupItemUpdateRequest `json:"items_to_update,omitempty"`  // ItemsToUpdate 是待调整展示顺序的分组项。
	ItemsToDelete   []int                    `json:"items_to_delete,omitempty"`  // ItemsToDelete 是待删除的分组项 ID。
}

// GroupActiveItemUpdateRequest 表示切换或清空分组当前渠道的请求。
type GroupActiveItemUpdateRequest struct {
	ItemID *int `json:"item_id" binding:"required"` // ItemID 是待设为当前渠道的分组项 ID，0 表示取消选择。
}

// GroupItemAddRequest 表示新增分组项请求。
type GroupItemAddRequest struct {
	ChannelID int    `json:"channel_id" binding:"required"` // ChannelID 是实际上游渠道 ID。
	ModelName string `json:"model_name" binding:"required"` // ModelName 是该渠道实际请求的模型名称。
	Priority  int    `json:"priority,omitempty"`             // Priority 是分组项的界面展示顺序。
	Weight    int    `json:"weight,omitempty"`               // Weight 是加权模式的分配权重。
}

// GroupItemUpdateRequest 表示分组项展示顺序更新请求。
type GroupItemUpdateRequest struct {
	ID       int `json:"id" binding:"required"` // ID 是待更新的分组项主键。
	Priority int `json:"priority,omitempty"`     // Priority 是新的界面展示顺序。
	Weight   int `json:"weight,omitempty"`       // Weight 是新的分配权重。
}

// GroupIDAndLLMName 表示渠道及其模型名称组合。
type GroupIDAndLLMName struct {
	ChannelID int    // ChannelID 是实际上游渠道 ID。
	ModelName string // ModelName 是该渠道提供的模型名称。
}
