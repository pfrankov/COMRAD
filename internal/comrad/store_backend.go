package comrad

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type storeBackend interface {
	Name() string
	Path() string
	Load() (Database, bool, error)
	Save(Database) error
}

type jsonFileBackend struct {
	path string
}

func newJSONFileBackend(path string) jsonFileBackend {
	return jsonFileBackend{path: path}
}

func (b jsonFileBackend) Name() string {
	return "json"
}

func (b jsonFileBackend) Path() string {
	return b.path
}

func (b jsonFileBackend) Load() (Database, bool, error) {
	if err := EnsureDir(filepath.Dir(b.path)); err != nil {
		return Database{}, false, err
	}
	data, err := os.ReadFile(b.path)
	if err != nil {
		if os.IsNotExist(err) {
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

func (b jsonFileBackend) Save(db Database) error {
	if err := EnsureDir(filepath.Dir(b.path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, b.path)
}
