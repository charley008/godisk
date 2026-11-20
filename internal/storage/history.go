package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Entry struct {
	Kind, Label string
	At          time.Time
	Payload     json.RawMessage
}

type Store struct {
	db   *sql.DB
	Path string
}

func Open() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "goDisk")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return OpenAt(filepath.Join(dir, "history.sqlite"))
}

func OpenAt(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, Path: path}, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout=3000"); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("历史数据库版本 %d 高于当前程序支持版本", version)
	}
	if version == 0 {
		for _, query := range []string{
			"CREATE TABLE history (id INTEGER PRIMARY KEY, at TEXT NOT NULL, kind TEXT NOT NULL, label TEXT NOT NULL, payload BLOB NOT NULL)",
			"CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)",
			"PRAGMA user_version=1",
		} {
			if _, err := tx.ExecContext(ctx, query); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Add(ctx context.Context, kind, label string, at time.Time, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO history (at,kind,label,payload) VALUES (?,?,?,?)",
		at.UTC().Format(time.RFC3339Nano), kind, label, payload)
	return err
}

func (s *Store) RecentBenchmarks(ctx context.Context) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT at,kind,label,payload FROM history WHERE kind = 'benchmark' ORDER BY id DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var item Entry
		var at string
		if err := rows.Scan(&at, &item.Kind, &item.Label, &item.Payload); err != nil {
			return nil, err
		}
		if item.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, err
		}
		entries = append(entries, item)
	}
	return entries, rows.Err()
}
