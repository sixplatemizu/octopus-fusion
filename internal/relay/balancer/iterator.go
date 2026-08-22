package balancer

import (
	"time"

	"github.com/bestruirui/octopus/internal/model"
)

// Candidates 生成一次请求的完整候选序列:策略排序、粘性渠道前置、剔除熔断中的渠道。
// 调用方按序遍历,遇到第一个可用渠道即使用;全部被剔除时返回空列表。
func Candidates(group model.Group, apiKeyID int, requestModel string) []model.GroupItem {
	if group.Mode == model.GroupModeManual || len(group.Items) == 0 {
		return nil
	}

	candidates := GetBalancer(group.Mode).Candidates(group.Items)

	if group.SessionKeepTime > 0 {
		stickyTTL := time.Duration(group.SessionKeepTime) * time.Second
		if sticky := GetSticky(apiKeyID, requestModel, stickyTTL); sticky != nil {
			for i, item := range candidates {
				if item.ChannelID == sticky.ChannelID {
					if i > 0 {
						stickyItem := candidates[i]
						copy(candidates[1:i+1], candidates[0:i])
						candidates[0] = stickyItem
					}
					break
				}
			}
		}
	}

	available := make([]model.GroupItem, 0, len(candidates))
	for _, item := range candidates {
		if tripped, _ := IsTripped(item.ChannelID, 0, item.ModelName); !tripped {
			available = append(available, item)
		}
	}
	return available
}
