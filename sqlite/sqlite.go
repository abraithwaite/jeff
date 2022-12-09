package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

type (
	// Option specifies an option to alter the behavior of Store.
	Option func(*Store)

	// Store satisfies the jeff.Storage interface.
	Store struct {
		db              *sql.DB
		cleanupInterval time.Duration
		tableName       string
	}
)

const sqliteDatetimeFormat = "2006-01-02 15:04:05"

// CleanupInterval specifies the interval with which to remove expired sessions
// from the SQLite database. Defaults to five minutes.
func CleanupInterval(d time.Duration) func(*Store) {
	return func(s *Store) {
		s.cleanupInterval = d
	}
}

// TableName specifies the name to use for the SQLite table to store sessions.
// Defaults to "sessions".
func TableName(name string) func(*Store) {
	return func(s *Store) {
		s.tableName = name
	}
}

// New initializes a new sqlite Storage for jeff.
func New(db *sql.DB, opts ...Option) (*Store, error) {
	s := &Store{db: db}

	s.defaults()
	for _, o := range opts {
		o(s)
	}

	if _, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			key TEXT PRIMARY KEY,
			value BLOB,
			expires_at TEXT NOT NULL
		)`, s.tableName)); err != nil {
		return nil, err
	}
	if s.cleanupInterval > 0 {
		go s.startCleanup(s.cleanupInterval)
	}
	return s, nil
}

// Store satisfies the jeff.Store.Store method.
func (s *Store) Store(_ context.Context, key, value []byte, exp time.Time) error {
	_, err := s.db.Exec(fmt.Sprintf(`
		INSERT OR REPLACE INTO
				%s
		(
				key,
				value,
				expires_at
		)
		VALUES
		(
				?,
				?,
				?
		)`, s.tableName),
		string(key), value, exp.Format(sqliteDatetimeFormat))
	if err != nil {
		return err
	}
	return nil
}

// Fetch satisfies the jeff.Store.Fetch method.
func (s *Store) Fetch(_ context.Context, key []byte) ([]byte, error) {
	var value []byte
	if err := s.db.QueryRow(fmt.Sprintf(`
	SELECT
		value
	FROM
		%s
	WHERE
		key = ? AND
		expires_at > datetime('now', 'localtime')`, s.tableName), string(key)).Scan(&value); err != nil {
		// Not found sessions must return nil value, nil error.
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return value, nil
}

// Delete satisfies the jeff.Store.Delete method.
func (s *Store) Delete(_ context.Context, key []byte) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE key = ?`, string(key))
	return err
}

func (s *Store) defaults() {
	s.cleanupInterval = 5 * time.Minute
	s.tableName = "sessions"
}

func (s *Store) startCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		if err := s.deleteExpired(); err != nil {
			log.Printf("failed to delete expired sessions from SQLite: %v", err)
		}
	}
}

func (s *Store) deleteExpired() error {
	_, err := s.db.Exec(fmt.Sprintf(`
		DELETE FROM
			%s
		WHERE
			expires_at <= datetime('now', 'localtime')`, s.tableName))
	return err
}
