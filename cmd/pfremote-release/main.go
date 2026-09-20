// pfremote-release is an offline publisher tool, not a client payload.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/releaseauth"
	"github.com/cottman99/pf-remote/internal/windowsinstaller"
)

func main() {
	keyPath := flag.String("key", "", "external publisher key")
	release := flag.String("release", "", "release directory (omit to generate/read publisher key)")
	sequence := flag.Uint64("sequence", 0, "monotonic release sequence")
	channel := flag.String("channel", "preview", "release channel")
	platform := flag.String("platform", "windows-x64", "signed platform")
	flag.Parse()
	if *keyPath == "" {
		panic("external key path required")
	}
	key, err := os.ReadFile(*keyPath)
	if os.IsNotExist(err) && *release == "" {
		if err = os.MkdirAll(filepath.Dir(*keyPath), 0700); err != nil {
			panic("key directory unavailable")
		}
		_, generated, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			panic("key generation failed")
		}
		f, err := os.OpenFile(*keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			panic("key creation failed")
		}
		if _, err = f.Write(generated); err != nil {
			f.Close()
			panic("key write failed")
		}
		if err = f.Sync(); err != nil {
			f.Close()
			panic("key sync failed")
		}
		f.Close()
		key = generated
	} else if err != nil {
		panic("publisher key unavailable")
	}
	if len(key) != ed25519.PrivateKeySize {
		panic("publisher key invalid")
	}
	private := ed25519.PrivateKey(key)
	if *release == "" {
		fmt.Println(base64.StdEncoding.EncodeToString(private.Public().(ed25519.PublicKey)))
		return
	}
	b, err := os.ReadFile(filepath.Join(*release, "release-manifest.json"))
	if err != nil {
		panic("manifest unavailable")
	}
	var manifest windowsinstaller.Manifest
	if json.Unmarshal(b, &manifest) != nil {
		panic("manifest invalid")
	}
	now := time.Now().UTC()
	m := releaseauth.Metadata{SchemaVersion: releaseauth.MetadataSchemaV2, Product: "PF Remote", Version: manifest.Version, Channel: *channel, Platform: *platform, Sequence: *sequence, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(89 * 24 * time.Hour), Compatibility: &releaseauth.Compatibility{DataEpoch: 1, ProtocolMin: 1, ProtocolMax: 1}}
	artifacts := []string{"PFRemoteSetup.exe", "release-manifest.json", "PFRemote-Windows-x64-" + manifest.Version + ".zip"}
	if *platform == "linux-x64" {
		artifacts = []string{"pfremote-linux-x64", "pfremoted-linux-x64", "pfremote-update-linux-x64"}
	} else if *platform != "windows-x64" {
		panic("unsupported platform")
	}
	for _, name := range artifacts {
		path := filepath.Join(*release, name)
		info, err := os.Stat(path)
		if err != nil {
			panic("artifact unavailable")
		}
		hash, err := windowsinstaller.HashFile(path)
		if err != nil {
			panic("artifact unreadable")
		}
		m.Artifacts = append(m.Artifacts, releaseauth.Artifact{Name: name, Size: info.Size(), SHA256: hash})
	}
	document, err := releaseauth.Sign(m, func(b []byte) ([]byte, error) { return ed25519.Sign(private, b), nil })
	if err != nil {
		panic("release signing failed")
	}
	if err = os.WriteFile(filepath.Join(*release, *platform+".release.json"), document, 0600); err != nil {
		panic("signed release write failed")
	}
	names := append(append([]string{}, artifacts...), *platform+".release.json")
	checksums := "SHA256SUMS.txt"
	if *platform == "windows-x64" {
		names = append(names, "sbom.spdx.json", "licenses.json")
	} else {
		checksums = "SHA256SUMS-linux.txt"
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		hash, err := windowsinstaller.HashFile(filepath.Join(*release, name))
		if err != nil {
			panic("release checksum failed")
		}
		lines = append(lines, hash+"  "+name)
	}
	if err = os.WriteFile(filepath.Join(*release, checksums), []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		panic("checksum list write failed")
	}
	fmt.Println("Signed release metadata written.")
}
