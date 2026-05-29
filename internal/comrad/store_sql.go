package comrad

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const (
	StorageModeAuto     = "auto"
	StorageModeSQLite   = "sqlite"
	StorageModePostgres = "postgres"
)

type StoreConfig struct {
	Mode        string
	DatabaseURL string
	SQLitePath  string
}

type sqlBackend struct {
	name       string
	path       string
	db         *sql.DB
	createSQL  string
	selectSQL  string
	upsertSQL  string
	deleteHook func()
}

func OpenConfiguredStore(cfg StoreConfig) (*Store, error) {
	mode := normalizeStorageMode(cfg.Mode)
	switch mode {
	case StorageModePostgres:
		return openRequiredPostgresStore(cfg.DatabaseURL)
	case StorageModeSQLite:
		return OpenSQLiteStore(sqlitePath(cfg.SQLitePath))
	default:
		return openAutoStore(cfg)
	}
}

func OpenSQLiteStore(path string) (*Store, error) {
	path = sqlitePath(path)
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	backend, err := newSQLBackend(sqlBackendConfig{
		name:      StorageModeSQLite,
		driver:    "sqlite",
		dsn:       path,
		path:      path,
		createSQL: "create table if not exists comrad_state (id integer primary key, data text not null, updated_at text not null)",
		selectSQL: "select data from comrad_state where id = 1",
		upsertSQL: "insert into comrad_state (id, data, updated_at) values (1, ?, current_timestamp) on conflict(id) do update set data = excluded.data, updated_at = current_timestamp",
	})
	if err != nil {
		return nil, err
	}
	return openStoreWithBackend(backend)
}

func OpenPostgresStore(databaseURL string) (*Store, error) {
	backend, err := newPostgresBackend(databaseURL)
	if err != nil {
		return nil, err
	}
	return openStoreWithBackend(backend)
}

func openRequiredPostgresStore(databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("COMRAD_DATABASE_URL is required when storage mode is postgres")
	}
	return OpenPostgresStore(databaseURL)
}

func openAutoStore(cfg StoreConfig) (*Store, error) {
	if cfg.DatabaseURL != "" {
		store, err := OpenPostgresStore(cfg.DatabaseURL)
		if err == nil {
			return store, nil
		}
	}
	store, err := OpenSQLiteStore(sqlitePath(cfg.SQLitePath))
	if err != nil {
		return nil, err
	}
	if cfg.DatabaseURL != "" {
		_ = store.Audit("storage.fallback", "system", "sqlite", map[string]any{"from": StorageModePostgres})
	}
	return store, nil
}

func newPostgresBackend(databaseURL string) (storeBackend, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("postgres database URL is required")
	}
	return newSQLBackend(sqlBackendConfig{
		name:      StorageModePostgres,
		driver:    "pgx",
		dsn:       databaseURL,
		path:      "postgresql",
		createSQL: "create table if not exists comrad_state (id integer primary key, data jsonb not null, updated_at timestamptz not null)",
		selectSQL: "select data from comrad_state where id = 1",
		upsertSQL: "insert into comrad_state (id, data, updated_at) values (1, $1::jsonb, now()) on conflict(id) do update set data = excluded.data, updated_at = now()",
	})
}

type sqlBackendConfig struct {
	name      string
	driver    string
	dsn       string
	path      string
	createSQL string
	selectSQL string
	upsertSQL string
}

func newSQLBackend(cfg sqlBackendConfig) (*sqlBackend, error) {
	db, err := sql.Open(cfg.driver, cfg.dsn)
	if err != nil {
		return nil, err
	}
	if cfg.name == StorageModeSQLite {
		db.SetMaxOpenConns(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, cfg.createSQL); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &sqlBackend{name: cfg.name, path: cfg.path, db: db, createSQL: cfg.createSQL, selectSQL: cfg.selectSQL, upsertSQL: cfg.upsertSQL}, nil
}

func (b *sqlBackend) Name() string {
	return b.name
}

func (b *sqlBackend) Path() string {
	return b.path
}

func (b *sqlBackend) Load() (Database, bool, error) {
	var data []byte
	err := b.db.QueryRow(b.selectSQL).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return Database{}, false, nil
		}
		return Database{}, false, err
	}
	if len(data) == 0 {
		return Database{}, false, nil
	}
	var db Database
	if err := json.Unmarshal(data, &db); err != nil {
		return Database{}, false, fmt.Errorf("read database: %w", err)
	}
	return db, true, nil
}

func (b *sqlBackend) Save(db Database) error {
	data, err := json.Marshal(db)
	if err != nil {
		return err
	}
	_, err = b.db.Exec(b.upsertSQL, string(data))
	return err
}

func normalizeStorageMode(mode string) string {
	switch mode {
	case StorageModePostgres, StorageModeSQLite:
		return mode
	default:
		return StorageModeAuto
	}
}

func sqlitePath(path string) string {
	if path == "" {
		return "data/comrad.sqlite"
	}
	return path
}
