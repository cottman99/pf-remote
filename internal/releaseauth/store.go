package releaseauth

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store keeps update trust separate from application data and program versions.
// Bootstrap is an explicit installer action. Open never creates missing state.
// The caller must place this file in its OS-protected, per-user data directory.
type Store struct{ db *sql.DB }

func Bootstrap(path string, key ed25519.PublicKey, channel, platform string) error {
	if len(key) != ed25519.PublicKeySize || !validChannel(channel) || !validPlatform(platform) {
		return fail("TRUST_STATE_INVALID")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fail("TRUST_BOOTSTRAP_FAILED")
	}
	if err = f.Close(); err != nil {
		return fail("TRUST_BOOTSTRAP_FAILED")
	}
	// A failed bootstrap deliberately leaves a file, requiring explicit repair.
	s, err := openStore(path)
	if err != nil {
		return err
	}
	defer s.Close()
	_, err = s.db.Exec(`CREATE TABLE update_trust (
		id INTEGER PRIMARY KEY CHECK(id=1), schema_version INTEGER NOT NULL,
		publisher TEXT NOT NULL, channel TEXT NOT NULL, platform TEXT NOT NULL,
		sequence TEXT NOT NULL, digest TEXT NOT NULL, observed_at TEXT NOT NULL);
		INSERT INTO update_trust VALUES(1,1,?,?,?,0,'','');`,
		base64.StdEncoding.EncodeToString(key), channel, platform)
	if err != nil {
		return fail("TRUST_BOOTSTRAP_FAILED")
	}
	return nil
}

func OpenStore(path string) (*Store, error) { return openStore(path) }

func openStore(path string) (*Store, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fail("TRUST_STATE_UNAVAILABLE")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fail("TRUST_STATE_UNAVAILABLE")
	}
	urlPath := filepath.ToSlash(abs)
	if !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	u := url.URL{Scheme: "file", Path: urlPath}
	q := url.Values{"mode": {"rw"}, "_pragma": {"busy_timeout(5000)", "synchronous(FULL)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fail("TRUST_STATE_UNAVAILABLE")
	}
	db.SetMaxOpenConns(1)
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Accept re-verifies against durable state inside a transaction. A release is
// returned only after its replay checkpoint commits. Concurrent writers cannot
// authorize an older release over a newer checkpoint. Clock observations persist
// even when a document is rejected; moving the clock back cannot renew expiry.
func (s *Store) Accept(ctx context.Context, document []byte, policy Policy) (Release, error) {
	if policy.Now.IsZero() {
		return Release{}, fail("TRUST_STATE_INVALID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Release{}, fail("TRUST_STATE_UNAVAILABLE")
	}
	defer tx.Rollback()
	var schema int
	var publisher, channel, platform, observed string
	var previous Checkpoint
	err = tx.QueryRowContext(ctx, `SELECT schema_version,publisher,channel,platform,sequence,digest,observed_at FROM update_trust WHERE id=1`).Scan(
		&schema, &publisher, &channel, &platform, &previous.Sequence, &previous.Digest, &observed)
	key, keyErr := base64.StdEncoding.Strict().DecodeString(publisher)
	if err != nil || schema != 1 || keyErr != nil || !bytes.Equal(key, policy.PublicKey) || channel != policy.Channel || platform != policy.Platform {
		return Release{}, fail("TRUST_STATE_INVALID")
	}
	if observed != "" {
		last, err := time.Parse(time.RFC3339Nano, observed)
		if err != nil {
			return Release{}, fail("TRUST_STATE_INVALID")
		}
		if last.After(policy.Now) {
			policy.Now = last
		}
	}
	policy.Previous = previous
	release, verifyErr := Verify(document, policy)
	next := previous
	if verifyErr == nil {
		next = release.Checkpoint()
	}
	_, err = tx.ExecContext(ctx, `UPDATE update_trust SET sequence=?,digest=?,observed_at=? WHERE id=1`, strconv.FormatUint(next.Sequence, 10), next.Digest, policy.Now.UTC().Format(time.RFC3339Nano))
	if err != nil || tx.Commit() != nil {
		return Release{}, fail("TRUST_STATE_WRITE_FAILED")
	}
	return release, verifyErr
}
