package state

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
)

// CompareAndSwapControlState prevents concurrent peer writers from losing a
// signed sequence or tombstone. It is used in a separate protected peer database.
func (s *Store) CompareAndSwapControlState(ctx context.Context, schema string, previous, next []byte) error {
	if err := validateControlState(schema, next); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldSchema string
	var old []byte
	err = tx.QueryRowContext(ctx, `SELECT schema_version,payload FROM control_state WHERE singleton = 1`).Scan(&oldSchema, &old)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if len(old) > 0 && oldSchema != schema || !bytes.Equal(old, previous) {
		return errors.New("peer state changed concurrently; reload before retry")
	}
	if err = writeControlState(ctx, tx, schema, next); err != nil {
		return err
	}
	return tx.Commit()
}
