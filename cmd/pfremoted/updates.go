package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/cottman99/pf-remote/internal/autoupdate"
	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/internal/releaseauth"
	"github.com/cottman99/pf-remote/internal/updatecheck"
)

// Provisioned by the trusted release build, never a Gateway or downloaded feed.
var publisherPublicKey = ""
var releaseVersion = "development"
var updateChannel = "preview"

func backgroundUpdates(statePath string) *updatecheck.Monitor {
	publicKey := publisherPublicKey
	if publicKey == "" {
		publicKey = autoupdate.PublisherKey
	}
	key, err := base64.StdEncoding.Strict().DecodeString(publicKey)
	if publicKey == "" {
		return updatecheck.New(nil, releaseVersion, 1, 1)
	}
	discover := func(ctx context.Context) (releaseauth.Release, error) {
		if err != nil || len(key) != ed25519.PublicKeySize || runtime.GOARCH != "amd64" {
			return releaseauth.Release{}, errors.New("update trust unavailable")
		}
		store, err := releaseauth.OpenStore(filepath.Join(filepath.Dir(statePath), "update-trust-v1.db"))
		if err != nil {
			return releaseauth.Release{}, err
		}
		defer store.Close()
		return store.Discover(ctx, http.DefaultClient, releaseauth.Policy{PublicKey: ed25519.PublicKey(key), Channel: updateChannel, Platform: runtime.GOOS + "-x64", Now: time.Now()})
	}
	return updatecheck.New(discover, releaseVersion, 1, 1)
}

func applyBackgroundUpdate(ctx context.Context, r releaseauth.Release, gate *localapi.ActionGate) string {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		return "incompatible"
	}
	_, root, err := autoupdate.Paths()
	if err != nil {
		return "retry"
	}
	m := r.Metadata()
	previous, statusErr := autoupdate.ReadStatus(root)
	if statusErr == nil && previous.Sequence == m.Sequence {
		if previous.Phase == "failed" || previous.Phase == "complete" {
			return "held"
		}
		if previous.Phase == "installing" && time.Since(previous.At) < 10*time.Minute {
			return "installing"
		}
	} else if statusErr != nil && !os.IsNotExist(statusErr) {
		return "held"
	}
	directory := filepath.Join(root, "release-"+strconv.FormatUint(m.Sequence, 10))
	if _, err = os.Stat(directory); os.IsNotExist(err) {
		base, err := releaseauth.GitHubFeed(m.Channel, m.Platform)
		if err != nil {
			return "retry"
		}
		staged, err := autoupdate.Stage(ctx, r, http.DefaultClient, base, root)
		if err != nil {
			return "retry"
		}
		if err = os.Rename(staged, directory); err != nil {
			os.RemoveAll(staged)
			return "retry"
		}
	} else if err != nil {
		return "retry"
	}
	checked, verifyErr := autoupdate.VerifyPackage(directory, m.Version)
	if verifyErr != nil || checked.Checkpoint() != r.Checkpoint() {
		return "held"
	}
	if !gate.BeginUpdate() {
		return "busy"
	}
	if !autoupdate.Idle() {
		gate.CancelUpdate()
		return "busy"
	}
	if err = autoupdate.WriteStatus(root, autoupdate.Status{Phase: "installing", Version: m.Version, Sequence: m.Sequence}); err != nil {
		gate.CancelUpdate()
		return "retry"
	}
	if err = autoupdate.StartSetup(filepath.Join(directory, autoupdate.InstallerName())); err != nil {
		gate.CancelUpdate()
		_ = autoupdate.WriteStatus(root, autoupdate.Status{Phase: "failed", Version: m.Version, Sequence: m.Sequence})
		return "held"
	}
	return "installing"
}
