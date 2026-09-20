// Offline end-to-end verification using real signed packages and isolated data.
package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/autoupdate"
	"github.com/cottman99/pf-remote/internal/desktopcredential"
	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/internal/releaseauth"
	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/internal/windowsinstaller"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func run(path string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	hide(cmd)
	if err := cmd.Run(); err != nil {
		panic("isolated setup failed")
	}
}

func profile(root string) ([32]byte, []string, int) {
	installed, err := windowsinstaller.Current(root)
	must(err)
	cmd := exec.Command(filepath.Join(filepath.Dir(installed.AppPath), "pfremoted.exe"))
	hide(cmd)
	must(cmd.Start())
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	var catalog contracts.CatalogResponse
	ready := false
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		catalog, err = localapi.NewClient().List(ctx)
		cancel()
		if err == nil {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready || len(catalog.Targets) == 0 {
		panic("isolated packaged daemon did not load a catalog")
	}
	config, _ := os.UserConfigDir()
	b, err := os.ReadFile(filepath.Join(config, "PF Remote", "identity", "device-identity-v1.json"))
	must(err)
	targets := []string{}
	for _, target := range catalog.Targets {
		targets = append(targets, target.Canonical)
	}
	sort.Strings(targets)
	return sha256.Sum256(b), targets, len(catalog.RecentSessions)
}

func main() {
	old := flag.String("previous", "", "old package directory")
	next := flag.String("release", "", "signed new package directory")
	pub := flag.String("public-key-file", "", "publisher public key file")
	flag.Parse()
	oldAbs, err := filepath.Abs(*old)
	must(err)
	nextAbs, err := filepath.Abs(*next)
	must(err)
	b, err := os.ReadFile(*pub)
	must(err)
	autoupdate.PublisherKey = strings.TrimSpace(string(b))
	root, err := os.MkdirTemp(".codex_tmp", "signed-upgrade-")
	must(err)
	root, err = filepath.Abs(root)
	must(err)
	defer os.RemoveAll(root)
	for _, name := range []string{"APPDATA", "LOCALAPPDATA", "ProgramData"} {
		path := filepath.Join(root, name)
		must(os.MkdirAll(path, 0700))
		must(os.Setenv(name, path))
	}
	for _, name := range []string{"PFREMOTE_GATEWAY_URL", "PFREMOTE_GATEWAY_OWNER_SYNC", "PFREMOTE_LEGACY_CENTER_CATALOG"} {
		must(os.Unsetenv(name))
	}
	must(os.Setenv("PFREMOTE_SETUP_TESTING", "true"))
	must(os.Setenv("PFREMOTE_LOCAL_ENDPOINT", fmt.Sprintf(`\\.\pipe\pfremote-signed-update-%d`, os.Getpid())))
	installation := filepath.Join(root, "program")
	run(filepath.Join(oldAbs, "PFRemoteSetup.exe"), "install", "--root", installation, "--no-launch")
	identity, targets, _ := profile(installation)
	statePath, err := state.DefaultPath()
	must(err)
	db, err := state.Open(statePath)
	must(err)
	must(db.RecordRecentSession(context.Background(), contracts.RecentSession{SessionID: "session-update-fixture", CanonicalTarget: targets[0], Action: "open", Status: "completed", StartedAt: time.Now()}, 50))
	must(db.Close())
	credentialRoot, err := desktopcredential.DefaultRoot()
	must(err)
	credentials, err := desktopcredential.New(credentialRoot)
	must(err)
	must(credentials.Save(targets[0], "synthetic-fixture-password"))
	must(autoupdate.Bootstrap())
	trust, cache, err := autoupdate.Paths()
	must(err)
	store, err := releaseauth.OpenStore(trust)
	must(err)
	doc, err := os.ReadFile(filepath.Join(nextAbs, "windows-x64.release.json"))
	must(err)
	policy, err := autoupdate.Policy()
	must(err)
	release, err := store.Accept(context.Background(), doc, policy)
	must(err)
	must(store.Close())
	server := httptest.NewTLSServer(http.FileServer(http.Dir(nextAbs)))
	defer server.Close()
	stage, err := autoupdate.Stage(context.Background(), release, server.Client(), server.URL+"/", cache)
	must(err)
	run(filepath.Join(stage, "PFRemoteSetup.exe"), "apply-update", "--root", installation, "--no-launch")
	current, err := windowsinstaller.Current(installation)
	must(err)
	if current.Current != release.Metadata().Version {
		panic("wrong installed version")
	}
	after, afterTargets, records := profile(installation)
	if after != identity || !reflect.DeepEqual(targets, afterTargets) || records != 1 {
		panic("profile did not survive signed upgrade")
	}
	password, err := credentials.Load(targets[0])
	must(err)
	if string(password) != "synthetic-fixture-password" {
		panic("credential did not survive")
	}
	clear(password)
	status, err := autoupdate.ReadStatus(cache)
	must(err)
	if status.Phase != "complete" {
		panic("update did not record completion")
	}
	run(filepath.Join(stage, "PFRemoteSetup.exe"), "rollback", "--root", installation, "--no-launch")
	restored, restoredTargets, records := profile(installation)
	if restored != identity || !reflect.DeepEqual(targets, restoredTargets) || records != 1 {
		panic("profile did not survive rollback")
	}
	fmt.Println(`{"status":"passed","signed_download":true,"installer_handoff":true,"identity_catalog_history_credentials_retained":true,"rollback":true}`)
}
