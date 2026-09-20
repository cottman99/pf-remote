package state

import (
	"context"
	"database/sql"
	"errors"
)

// Update coordination has its own additive table. It must never overwrite the
// singleton enrollment authority, and old binaries can ignore it on rollback.
func (s *Store) LoadUpdateState(ctx context.Context, key string) ([]byte, error) {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS update_state (key TEXT PRIMARY KEY, payload BLOB NOT NULL)`); err != nil {
		return nil, errors.New("update storage unavailable")
	}
	var b []byte
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM update_state WHERE key = ?`, key).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("update state unavailable")
	}
	return b, nil
}
func (s *Store) SaveUpdateState(ctx context.Context, key string, b []byte) error {
	if len(b) > 1<<20 {
		return errors.New("update state exceeds limit")
	}
	if _, err := s.LoadUpdateState(ctx, key); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO update_state(key,payload) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET payload=excluded.payload`, key, b)
	if err != nil {
		return errors.New("update state could not be saved")
	}
	return nil
}
