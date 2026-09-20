package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadRoadmapFindsFirstUncheckedItemAndEvidence(t *testing.T) {
	path := writeTestFile(t, `# Roadmap

## M0 — done

- [x] First.

Evidence: [Report](../status/M0.md).

## M1 — active

- [x] Earlier.

Evidence M1.1: [Report](../status/M1.1.md).

- [ ] Current.
- [ ] Later.
`)
	state, err := readRoadmap(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.first == nil || state.first.key != "M1.2" || state.first.milestone != "M1" || state.first.text != "Current." {
		t.Fatalf("unexpected first item: %#v", state.first)
	}
	if len(state.milestones) != 2 || state.milestones[0].evidencePath != "../status/M0.md" || state.milestones[1].itemEvidence[1] != "../status/M1.1.md" {
		t.Fatalf("unexpected milestones: %#v", state.milestones)
	}
}

func TestReadRoadmapRejectsCheckedWorkAfterUnchecked(t *testing.T) {
	path := writeTestFile(t, `## M0 — invalid

- [ ] Current.
- [x] Drifted completion.
`)
	if _, err := readRoadmap(path); err == nil {
		t.Fatal("expected ordering error")
	}
}

func TestReadRoadmapAllowsTerminalState(t *testing.T) {
	path := writeTestFile(t, `## M0 — complete

- [x] Done.

Evidence: [Report](../status/M0.md).
`)
	state, err := readRoadmap(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.first != nil {
		t.Fatalf("expected no active item, got %#v", state.first)
	}
}

func TestReadRoadmapRejectsEmptyAndTasklessMilestones(t *testing.T) {
	empty := writeTestFile(t, "# Empty roadmap\n")
	if _, err := readRoadmap(empty); err == nil {
		t.Fatal("expected missing-milestone error")
	}

	taskless := writeTestFile(t, "## M0 — empty\n")
	if _, err := readRoadmap(taskless); err == nil {
		t.Fatal("expected taskless-milestone error")
	}

	duplicate := writeTestFile(t, "## M0 — first\n\n- [x] Done.\n\n## M0 — duplicate\n\n- [x] Done again.\n")
	if _, err := readRoadmap(duplicate); err == nil {
		t.Fatal("expected duplicate-milestone error")
	}
}

func TestReadActiveWorkRequiresSectionsAndRejectsDuplicateKeys(t *testing.T) {
	valid := writeTestFile(t, activeWorkFixture("status: active\n"))
	work, err := readActiveWork(valid)
	if err != nil {
		t.Fatal(err)
	}
	if work.metadata["status"] != "active" || work.sections["Objective"] == "" {
		t.Fatalf("unexpected active work: %#v", work)
	}

	duplicate := writeTestFile(t, activeWorkFixture("status: active\nstatus: complete\n"))
	if _, err := readActiveWork(duplicate); err == nil {
		t.Fatal("expected duplicate-key error")
	}

	empty := writeTestFile(t, "---\nstatus: active\n---\n")
	if _, err := readActiveWork(empty); err == nil {
		t.Fatal("expected missing-section error")
	}
}

func TestReadBriefStateRequiresExactMarkers(t *testing.T) {
	valid := writeTestFile(t, "Current milestone: M3\nCurrent roadmap item: M3.1\n")
	milestone, item, err := readBriefState(valid)
	if err != nil || milestone != "M3" || item != "M3.1" {
		t.Fatalf("unexpected result %q/%q, %v", milestone, item, err)
	}

	invalid := writeTestFile(t, "Current milestone: M3 stale\nCurrent roadmap item: M3.1\n")
	if _, _, err := readBriefState(invalid); err == nil {
		t.Fatal("expected invalid-milestone error")
	}
}

func TestValidateEvidenceRequiresRegularFileInsideStatus(t *testing.T) {
	root := t.TempDir()
	status := filepath.Join(root, "docs", "status")
	plans := filepath.Join(root, "docs", "plans")
	if err := os.MkdirAll(status, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(plans, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(status, "report.md"), []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateEvidence(root, "../status/report.md", "test evidence"); err != nil {
		t.Fatal(err)
	}
	if err := validateEvidence(root, "../../outside.md", "outside evidence"); err == nil {
		t.Fatal("expected outside-path error")
	}
	if err := validateEvidence(root, "../status", "directory evidence"); err == nil {
		t.Fatal("expected directory error")
	}
}

func TestValidateRoadmapEvidenceRequiresPartialItemEvidence(t *testing.T) {
	root := t.TempDir()
	status := filepath.Join(root, "docs", "status")
	if err := os.MkdirAll(status, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(status, "M3.1.md"), []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}

	milestones := []milestone{{
		key:          "M3",
		tasks:        2,
		completed:    1,
		itemEvidence: map[int]string{},
	}}
	if err := validateRoadmapEvidence(root, milestones); err == nil {
		t.Fatal("expected missing item-evidence error")
	}
	milestones[0].itemEvidence[1] = "../status/M3.1.md"
	if err := validateRoadmapEvidence(root, milestones); err != nil {
		t.Fatal(err)
	}

	milestones[0].completed = 0
	if err := validateRoadmapEvidence(root, milestones); err == nil {
		t.Fatal("expected evidence-on-unchecked-item error")
	}
}

func TestValidateActiveWorkBlockedAndTerminalStates(t *testing.T) {
	root, commit := validationRepo(t)
	if err := os.WriteFile(filepath.Join(root, "docs", "status", "blocker.md"), []byte("blocked evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "PROJECT_BRIEF.md"), []byte("Current milestone: M3\nCurrent roadmap item: M3.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := &roadmapItem{key: "M3.1", milestone: "M3", text: "Current."}
	blocked := activeWork{
		metadata: map[string]string{
			"schema_version":    "1",
			"roadmap_item":      "M3.1",
			"roadmap_milestone": "M3",
			"roadmap_text":      "Current.",
			"status":            "blocked",
			"base_commit":       commit,
			"blocker_id":        "B1",
			"blocked_attempts":  "3",
			"blocker_evidence":  "../status/blocker.md",
			"resume_condition":  "Dependency restored.",
			"return_point":      "M3.1 contract.",
		},
		sections: map[string]string{"Blocking side task": "Dependency outage."},
	}
	if err := validateActiveWork(root, blocked, roadmapState{first: first}); err != nil {
		t.Fatal(err)
	}
	delete(blocked.metadata, "resume_condition")
	if err := validateActiveWork(root, blocked, roadmapState{first: first}); err == nil {
		t.Fatal("expected missing blocked-field error")
	}

	if err := os.WriteFile(filepath.Join(root, "PROJECT_BRIEF.md"), []byte("Current milestone: complete\nCurrent roadmap item: complete\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	complete := activeWork{
		metadata: map[string]string{
			"schema_version":    "1",
			"roadmap_item":      "complete",
			"roadmap_milestone": "complete",
			"roadmap_text":      "All roadmap items complete.",
			"status":            "complete",
			"base_commit":       commit,
		},
		sections: map[string]string{"Blocking side task": "None."},
	}
	if err := validateActiveWork(root, complete, roadmapState{}); err != nil {
		t.Fatal(err)
	}
	complete.metadata["status"] = "active"
	if err := validateActiveWork(root, complete, roadmapState{}); err == nil {
		t.Fatal("expected invalid terminal-state error")
	}
}

func TestValidateBaseCommitRejectsMissingObject(t *testing.T) {
	root, _ := validationRepo(t)
	if err := validateBaseCommit(root, "deadbee"); err == nil {
		t.Fatal("expected missing-commit error")
	}
}

func TestValidateEvidenceRejectsStatusSymlinkOutsideRepo(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs", "plans"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "report.md"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "docs", "status")); err != nil {
		t.Skipf("creating a directory symlink is unavailable: %v", err)
	}
	if err := validateEvidence(root, "../status/report.md", "outside status"); err == nil {
		t.Fatal("expected repository-boundary error")
	}
}

func activeWorkFixture(extraMetadata string) string {
	return `---
schema_version: 1
roadmap_item: M3.1
roadmap_milestone: M3
roadmap_text: Current.
` + extraMetadata + `base_commit: deadbee
---

## Objective
x
## Acceptance criteria
x
## Out of scope
x
## Current step
x
## Blocking side task
x
## Review gates
x
## Deferred findings
x
## Evidence
x
`
}

func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func validationRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "status"), 0o700); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Governance Test")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "seed.txt")
	runGit(t, root, "commit", "--quiet", "-m", "seed")
	commit := runGit(t, root, "rev-parse", "HEAD")
	return root, commit
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", root}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
