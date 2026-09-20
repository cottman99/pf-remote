// Package autoupdate binds verified releases to the existing Windows installer.
package autoupdate

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cottman99/pf-remote/internal/releaseauth"
)

// These public values are embedded by the trusted build. Private signing keys
// never enter a client, Gateway, repository, or deployment package.
var PublisherKey string
var Channel = "preview"

type Status struct {
	Schema   string    `json:"schema_version"`
	Phase    string    `json:"phase"`
	Version  string    `json:"version"`
	Sequence uint64    `json:"sequence"`
	At       time.Time `json:"at"`
}

func Paths() (string, string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", "", err
	}
	root = filepath.Join(root, "PF Remote", "state")
	return filepath.Join(root, "update-trust-v1.db"), filepath.Join(root, "updates"), nil
}

func Policy() (releaseauth.Policy, error) {
	key, err := base64.StdEncoding.Strict().DecodeString(PublisherKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return releaseauth.Policy{}, errors.New("publisher trust is not configured")
	}
	return releaseauth.Policy{PublicKey: ed25519.PublicKey(key), Channel: Channel, Platform: "windows-x64", Now: time.Now()}, nil
}

// Bootstrap is called only by an explicitly installed, trusted package. A marker
// prevents a later missing trust database from silently resetting replay state.
func Bootstrap() error {
	if PublisherKey == "" {
		return nil
	}
	p, err := Policy()
	if err != nil {
		return err
	}
	trust, root, err := Paths()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return err
	}
	marker := filepath.Join(root, "bootstrap-v1")
	if _, err = os.Stat(trust); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("update trust requires repair")
	}
	if _, err = f.WriteString("pfremote.update-bootstrap/v1\n"); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return releaseauth.Bootstrap(trust, p.PublicKey, p.Channel, p.Platform)
}

func WriteStatus(root string, s Status) error {
	s.Schema = "pfremote.update-status/v1"
	s.At = time.Now().UTC()
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(root, ".status-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(root, "status-v1.json"))
}

func ReadStatus(root string) (Status, error) {
	f, err := os.Open(filepath.Join(root, "status-v1.json"))
	if err != nil {
		return Status{}, err
	}
	defer f.Close()
	var s Status
	err = json.NewDecoder(io.LimitReader(f, 4096)).Decode(&s)
	if err != nil || s.Schema != "pfremote.update-status/v1" {
		return Status{}, errors.New("update status invalid")
	}
	return s, nil
}

// Stage creates a unique private directory containing exactly the three signed
// installer artifacts. It never accepts package names or endpoints from a hint.
func Stage(ctx context.Context, r releaseauth.Release, client *http.Client, base, root string) (string, error) {
	if !r.Compatible(1, 1) {
		return "", errors.New("update is not data compatible")
	}
	m := r.Metadata()
	names := []string{"PFRemoteSetup.exe", "release-manifest.json", "PFRemote-Windows-x64-" + m.Version + ".zip"}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, "package-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()
	for _, name := range names {
		f, err := r.DownloadArtifact(ctx, client, base, name, dir)
		if err != nil {
			return "", err
		}
		path := f.Name()
		if err = f.Close(); err != nil {
			return "", err
		}
		if err = os.Rename(path, filepath.Join(dir, name)); err != nil {
			return "", err
		}
	}
	if err = os.WriteFile(filepath.Join(dir, "windows-x64.release.json"), r.Document(), 0600); err != nil {
		return "", err
	}
	ok = true
	return dir, nil
}

func VerifyPackage(directory, version string) (releaseauth.Release, error) {
	p, err := Policy()
	if err != nil {
		return releaseauth.Release{}, err
	}
	trust, _, err := Paths()
	if err != nil {
		return releaseauth.Release{}, err
	}
	s, err := releaseauth.OpenStore(trust)
	if err != nil {
		return releaseauth.Release{}, err
	}
	defer s.Close()
	f, err := os.Open(filepath.Join(directory, "windows-x64.release.json"))
	if err != nil {
		return releaseauth.Release{}, err
	}
	doc, err := io.ReadAll(io.LimitReader(f, releaseauth.MaxEnvelope+1))
	f.Close()
	if err != nil {
		return releaseauth.Release{}, err
	}
	r, err := s.Accept(context.Background(), doc, p)
	if err != nil {
		return releaseauth.Release{}, err
	}
	if !r.Compatible(1, 1) || r.Metadata().Version != version {
		return releaseauth.Release{}, errors.New("update compatibility mismatch")
	}
	for _, name := range []string{"PFRemoteSetup.exe", "release-manifest.json", "PFRemote-Windows-x64-" + version + ".zip"} {
		f, err := os.Open(filepath.Join(directory, name))
		if err != nil {
			return releaseauth.Release{}, err
		}
		err = r.VerifyArtifact(name, f)
		f.Close()
		if err != nil {
			return releaseauth.Release{}, err
		}
	}
	return r, nil
}
