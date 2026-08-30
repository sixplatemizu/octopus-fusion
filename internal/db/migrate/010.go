package migrate

import (
	"encoding/json"
	"fmt"

	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 10,
		Up:      migrateFixGroupRoutingForDirectUpgrade,
	})
}

// migrateFixGroupRoutingForDirectUpgrade 兜底修复从 v0.9.28 直升新版时 007 未生效导致的分组状态异常。
// v0.9.28 的 groups 表无 relay_config 列，AutoMigrate 重建表时会丢弃 retry_interval，
// 导致 007 入口 HasColumn(retry_interval)==false 而提前退出，留下 mode='3' 与 relay_config=NULL。
// 本迁移将非法 mode 统一为 failover，并为缺失的 relay_config 写入默认配置。
func migrateFixGroupRoutingForDirectUpgrade(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if !db.Migrator().HasTable("groups") {
		return nil
	}
	if !db.Migrator().HasColumn("groups", "mode") || !db.Migrator().HasColumn("groups", "relay_config") {
		return nil
	}

	// 非法 mode（旧 int 字符串如 '3'）统一为 failover，保留用户原 failover 语义。
	if err := db.Table("groups").Where("mode NOT IN ?", []string{string(model.GroupModeManual), string(model.GroupModeFailover)}).Update("mode", string(model.GroupModeFailover)).Error; err != nil {
		return fmt.Errorf("failed to normalize groups.mode: %w", err)
	}

	defaultCfg, err := json.Marshal(model.DefaultGroupRelayConfig())
	if err != nil {
		return fmt.Errorf("failed to marshal default relay config: %w", err)
	}
	if err := db.Table("groups").Where("relay_config IS NULL OR relay_config = '' OR relay_config = 'null'").Update("relay_config", string(defaultCfg)).Error; err != nil {
		return fmt.Errorf("failed to backfill groups.relay_config: %w", err)
	}
	return nil
}
