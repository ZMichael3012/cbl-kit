// Пакет storagex содержит общие утилиты работы с PostgreSQL для сервисов CBL:
// применение goose-миграций, чтобы сервисы не дублировали обвязку.
package storagex

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

// Migrate применяет goose-миграции из каталога dir.
// Миграции только вперед; изменение примененных файлов запрещено процессом CBL-001.
func Migrate(ctx context.Context, db *gorm.DB, dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("storagex: migrations dir %q: %w", dir, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("storagex: sql db: %w", err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("storagex: goose dialect: %w", err)
	}
	goose.SetBaseFS(os.DirFS(dir))
	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		if errors.Is(err, goose.ErrNoNextVersion) {
			return nil
		}
		return fmt.Errorf("storagex: goose up: %w", err)
	}
	return nil
}
