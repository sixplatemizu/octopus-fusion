package migrate

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 6,
		Up:      migrateDropLegacyGroupColumns,
	})
}

// migrateDropLegacyGroupColumns 移除旧版分组中已废弃的字段。
// groups 表的 match_regex、first_token_time_out 不再使用;mode 是新版路由模式的根级列,
// session_keep_time 已并入 relay_config,均不在清理列表中;全新库按版本重放迁移时,
// 若在此删除 mode 列会导致后续迁移找不到该列而初始化失败。
func migrateDropLegacyGroupColumns(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	if db.Migrator().HasTable("groups") {
		legacyGroupColumns := []string{"match_regex", "first_token_time_out"}
		for _, column := range legacyGroupColumns {
			if err := dropColumnIfExists(db, &model.Group{}, "groups", column); err != nil {
				return err
			}
		}
	}

	return nil
}
