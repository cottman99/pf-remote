package windowsinstaller

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func installedFixture(t *testing.T) (string, State) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "program")
	for _, v := range []string{"0.1.0", "0.2.0"} {
		a, m, ah, mh := releaseFixture(t, v, v)
		if _, err := Install(root, a, m, ah, mh); err != nil {
			t.Fatal(err)
		}
	}
	s, err := loadState(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, s
}

func TestHealthFailureRestoresExactStateAndPreservesUserFiles(t *testing.T) {
	root, before := installedFixture(t)
	// Synthetic runtime lives outside the versioned program, as in deployment.
	data := filepath.Join(filepath.Dir(root), "private-runtime")
	if err := os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"identity.json", "credentials.json", "history.db", "gateway.json", "tls.key", "legacy.json"} {
		if err := os.WriteFile(filepath.Join(data, name), []byte("synthetic retained "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a, m, ah, mh := releaseFixture(t, "0.3.0", "candidate")
	var started []string
	result, err := Upgrade(root, a, m, ah, mh, func(r Result) error {
		started = append(started, r.Current)
		if r.Current == "0.3.0" {
			return errors.New("candidate failed")
		}
		return nil
	})
	if err == nil || result.Current != before.Current || !reflect.DeepEqual(started, []string{"0.3.0", "0.2.0"}) {
		t.Fatalf("result=%+v err=%v started=%v", result, err, started)
	}
	state, err := loadState(root)
	if err != nil || state != before {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	for _, version := range []string{"0.1.0", "0.2.0"} {
		if _, err := os.Stat(filepath.Join(root, "versions", version)); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(data)
	if len(entries) != 6 {
		t.Fatal("user state files missing")
	}
	for _, entry := range entries {
		b, err := os.ReadFile(filepath.Join(data, entry.Name()))
		if err != nil || string(b) != "synthetic retained "+entry.Name() {
			t.Fatal("user state changed")
		}
	}
	if err := noPending(root); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedUpgradeRecoveryRetriesAndBlocksOtherMaintenance(t *testing.T) {
	root, before := installedFixture(t)
	if err := writeJSONAtomic(filepath.Join(root, pendingName), pendingUpgrade{"pfremote.pending-upgrade/v1", before}); err != nil {
		t.Fatal(err)
	}
	a, m, ah, mh := releaseFixture(t, "0.3.0", "candidate")
	if _, err := install(root, a, m, ah, mh, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(root, a, m, ah, mh); err == nil {
		t.Fatal("pending upgrade overwritten")
	}
	if _, err := Rollback(root); err == nil {
		t.Fatal("rollback ignored pending upgrade")
	}
	if err := Uninstall(root); err == nil {
		t.Fatal("uninstall removed recovery files")
	}
	if _, err := RecoverUpgrade(root, func(Result) error { return errors.New("not ready") }); err == nil {
		t.Fatal("failed recovery reported success")
	}
	if err := noPending(root); err == nil {
		t.Fatal("failed recovery lost its journal")
	}
	got, err := RecoverUpgrade(root, func(r Result) error {
		if r.Current != before.Current {
			t.Fatal("wrong recovery version")
		}
		return nil
	})
	if err != nil || got.Current != before.Current {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := RecoverUpgrade(root, func(Result) error { t.Fatal("completed recovery repeated activation"); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestHealthyUpgradeCommitsAndSerializesMaintenance(t *testing.T) {
	root, _ := installedFixture(t)
	a, m, ah, mh := releaseFixture(t, "0.3.0", "candidate")
	got, err := Upgrade(root, a, m, ah, mh, func(r Result) error {
		if _, err := Install(root, a, m, ah, mh); err == nil {
			t.Fatal("concurrent maintenance allowed")
		}
		return nil
	})
	if err != nil || got.Current != "0.3.0" || got.Previous != "0.2.0" {
		t.Fatalf("%+v %v", got, err)
	}
	if err := noPending(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "versions", "0.1.0")); !os.IsNotExist(err) {
		t.Fatal("obsolete version not pruned after health success")
	}
}

func TestRecoveryRejectsUnsafeVersion(t *testing.T) {
	root, before := installedFixture(t)
	before.Current = "../../outside"
	if err := writeJSONAtomic(filepath.Join(root, pendingName), pendingUpgrade{"pfremote.pending-upgrade/v1", before}); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverUpgrade(root, func(Result) error { t.Fatal("unsafe recovery activated"); return nil }); err == nil {
		t.Fatal("unsafe recovery accepted")
	}
}

func TestInstallRejectsCorruptExistingState(t *testing.T) {
	root, _ := installedFixture(t)
	if err := os.WriteFile(filepath.Join(root, "current.json"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	a, m, ah, mh := releaseFixture(t, "0.3.0", "candidate")
	if _, err := Install(root, a, m, ah, mh); err == nil {
		t.Fatal("corrupt state overwritten")
	}
	got, err := os.ReadFile(filepath.Join(root, "current.json"))
	if err != nil || string(got) != "corrupt" {
		t.Fatal("corrupt state was silently replaced")
	}
}
