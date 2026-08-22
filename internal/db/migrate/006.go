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
// groups 表的 match_regex、first_token_time_out 不再使用;mode 与 session_keep_time
// 已重新启用,group_items 的 weight 同样保留,均不在清理列表中。
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

func dropColumnIfExists(db *gorm.DB, model interface{}, table, column string) error {
	if !db.Migrator().HasColumn(model, column) {
		return nil
	}
	if db.Dialector.Name() == "sqlite" {
		if err := db.Exec(fmt.Sprintf(`ALTER TABLE %q DROP COLUMN %q`, table, column)).Error; err != nil {
			return fmt.Errorf("failed to drop %s.%s: %w", table, column, err)
		}
		return nil
	}
	if err := db.Migrator().DropColumn(model, column); err != nil {
		return fmt.Errorf("failed to drop %s.%s: %w", table, column, err)
	}
	return nil
}
