// Package storagex contains shared PostgreSQL utilities for CBL services:
// applying goose migrations so that services do not duplicate the plumbing.
package storagex

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

// Migrate applies goose migrations from the dir directory.
// Migrations are forward only; modifying applied files is forbidden by the CBL-001 process.
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
