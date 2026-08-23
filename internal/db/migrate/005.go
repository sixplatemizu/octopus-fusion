package migrate

import (
	"fmt"

	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 5,
		Up:      migrateChannelKeepMultiURLAndKey,
	})
}

// migrateChannelKeepMultiURLAndKey 保留多地址与多凭据结构,不再收敛为单地址单凭据。
// 上游版本的该迁移会把 channels.base_urls 与 channel_keys 数据合并进单一字段后删表删列;
// 本版本恢复了多地址多凭据能力,故此迁移不再执行任何清理。
func migrateChannelKeepMultiURLAndKey(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	return nil
}
