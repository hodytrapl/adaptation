// Package sql инкапсулирует работу с миграциями БД.
// Файлы миграций эмбедятся в бинарь, поэтому на проде не нужно
// тащить отдельный каталог migrations/ рядом с бинарём.
package sql

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	// Регистрируем database/sql-драйвер под именем "pgx".
	// Если у тебя уже используется pgx в других слоях — импорт можно вынести в один файл пакета.
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Config struct {
	// DSN -> postgres://user:pass@host:5432/db?sslmode=disable
	DSN string
	MigrationsTable string
	Timeout time.Duration
}

// RunUp накатывает все недостающие миграции.
// Возвращает nil, если накатывать нечего (ErrNoChange).
func RunUp(cfg Config) error {
	m, cleanup, err := newMigrator(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Printf("[migrate] no changes, schema is up to date")
			return nil
		}
		return fmt.Errorf("migrate up: %w", err)
	}

	v, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("migrate version: %w", err)
	}
	log.Printf("[migrate] up to version=%d dirty=%v", v, dirty)
	return nil
}

// RunDown откатывает ВСЁ вниз (обычно только для dev).
func RunDown(cfg Config) error {
	m, cleanup, err := newMigrator(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate down: %w", err)
	}
	log.Printf("[migrate] down ok")
	return nil
}

// Version возвращает текущую версию и флаг "dirty".
func Version(cfg Config) (uint, bool, error) {
	m, cleanup, err := newMigrator(cfg)
	if err != nil {
		return 0, false, err
	}
	defer cleanup()

	v, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("migrate version: %w", err)
	}
	return v, dirty, nil
}

// Force выставляет версию вручную. Нужно после "dirty" миграции,
// когда руками поправили схему и надо отвязать migrate от упавшей версии:
//
//	Force(cfg, 3) // schema_migrations.clean = true, version = 3
func Force(cfg Config, version int) error {
	m, cleanup, err := newMigrator(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := m.Force(version); err != nil {
		return fmt.Errorf("migrate force: %w", err)
	}
	log.Printf("[migrate] forced version=%d", version)
	return nil
}

// newMigrator собирает migrate.Migrate поверх embed-FS и живой *sql.DB.
// Вариант "свой *sql.DB" выбран, чтобы:
//   - переиспользовать пул соединений приложения (если он уже есть);
//   - не парсить DSN дважды;
//   - один раз регистрировать pgx.
func newMigrator(cfg Config) (*migrate.Migrate, func(), error) {
	if cfg.DSN == "" {
		return nil, nil, errors.New("migrate: empty DSN")
	}

	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, nil, fmt.Errorf("migrate: open db: %w", err)
	}

	if cfg.Timeout > 0 {
		if err := pingWithTimeout(db, cfg.Timeout); err != nil {
			_ = db.Close()
			return nil, nil, fmt.Errorf("migrate: ping db: %w", err)
		}
	}

	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migrate: iofs source: %w", err)
	}

	dbDriver, err := postgres.WithInstance(db, &postgres.Config{
		MigrationsTable: cfg.MigrationsTable,
	})
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migrate: postgres driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", dbDriver)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migrate: new instance: %w", err)
	}

	cleanup := func() {
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			log.Printf("[migrate] close: src=%v db=%v", srcErr, dbErr)
		}
		// db закрывает сам m.Close() через dbDriver — отдельно не закрываем.
	}
	return m, cleanup, nil
}

func pingWithTimeout(db *sql.DB, d time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- db.Ping() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		return errors.New("ping timeout")
	}
}