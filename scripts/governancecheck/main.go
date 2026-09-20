package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	milestonePattern    = regexp.MustCompile(`^## (M[0-9]+)\b`)
	itemPattern         = regexp.MustCompile(`^- \[([ xX])\] (.+)$`)
	evidencePattern     = regexp.MustCompile(`^Evidence: \[[^]]+\]\(([^)]+)\)\.$`)
	itemEvidencePattern = regexp.MustCompile(`^Evidence (M[0-9]+\.[0-9]+): \[[^]]+\]\(([^)]+)\)\.$`)
	commitPattern       = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	milestoneKeyPattern = regexp.MustCompile(`^(M[0-9]+|complete)$`)
	itemKeyPattern      = regexp.MustCompile(`^(M[0-9]+\.[0-9]+|complete)$`)
)

var requiredSections = []string{
	"Objective",
	"Acceptance criteria",
	"Out of scope",
	"Current step",
	"Blocking side task",
	"Review gates",
	"Deferred findings",
	"Evidence",
}

type activeWork struct {
	metadata map[string]string
	sections map[string]string
}

type roadmapItem struct {
	key       string
	milestone string
	text      string
}

type milestone struct {
	key          string
	tasks        int
	completed    int
	evidencePath string
	itemEvidence map[int]string
}

type roadmapState struct {
	first      *roadmapItem
	milestones []milestone
}

func main() {
	root, err := findRepoRoot()
	if err != nil {
		fail(err)
	}

	active, err := readActiveWork(filepath.Join(root, "docs", "plans", "ACTIVE_WORK.md"))
	if err != nil {
		fail(err)
	}
	state, err := readRoadmap(filepath.Join(root, "docs", "plans", "IMPLEMENTATION_ROADMAP.md"))
	if err != nil {
		fail(err)
	}
	if err := validateActiveWork(root, active, state); err != nil {
		fail(err)
	}
	if err := validateRoadmapEvidence(root, state.milestones); err != nil {
		fail(err)
	}

	if state.first == nil {
		fmt.Println("Execution governance check passed: the roadmap is complete and no item is active.")
		return
	}
	fmt.Printf("Execution governance check passed: %s is the single %s roadmap item.\n", state.first.key, active.metadata["status"])
}

func validateActiveWork(root string, active activeWork, state roadmapState) error {
	required := []string{"schema_version", "roadmap_item", "roadmap_milestone", "roadmap_text", "status", "base_commit"}
	for _, key := range required {
		if active.metadata[key] == "" {
			return fmt.Errorf("ACTIVE_WORK.md is missing %q", key)
		}
	}
	if active.metadata["schema_version"] != "1" {
		return fmt.Errorf("unsupported ACTIVE_WORK schema_version %q", active.metadata["schema_version"])
	}
	status := active.metadata["status"]
	if status != "active" && status != "blocked" && status != "complete" {
		return fmt.Errorf("ACTIVE_WORK status must be active, blocked, or complete, got %q", status)
	}
	if err := validateBaseCommit(root, active.metadata["base_commit"]); err != nil {
		return err
	}

	briefMilestone, briefItem, err := readBriefState(filepath.Join(root, "PROJECT_BRIEF.md"))
	if err != nil {
		return err
	}
	if state.first == nil {
		if status != "complete" || active.metadata["roadmap_item"] != "complete" ||
			active.metadata["roadmap_milestone"] != "complete" ||
			active.metadata["roadmap_text"] != "All roadmap items complete." ||
			briefMilestone != "complete" || briefItem != "complete" {
			return fmt.Errorf("a fully checked roadmap requires complete ACTIVE_WORK and PROJECT_BRIEF markers")
		}
		return nil
	}
	if status == "complete" {
		return fmt.Errorf("ACTIVE_WORK cannot be complete while roadmap item %s is unchecked", state.first.key)
	}
	if active.metadata["roadmap_item"] != state.first.key ||
		active.metadata["roadmap_milestone"] != state.first.milestone ||
		active.metadata["roadmap_text"] != state.first.text {
		return fmt.Errorf("ACTIVE_WORK %s does not match first unchecked roadmap item %s (%s)", active.metadata["roadmap_item"], state.first.key, state.first.text)
	}
	if briefMilestone != state.first.milestone || briefItem != state.first.key {
		return fmt.Errorf("PROJECT_BRIEF %s/%s does not match roadmap %s/%s", briefMilestone, briefItem, state.first.milestone, state.first.key)
	}
	if status == "blocked" {
		for _, key := range []string{"blocker_id", "blocked_attempts", "blocker_evidence", "resume_condition", "return_point"} {
			if active.metadata[key] == "" {
				return fmt.Errorf("blocked ACTIVE_WORK is missing %q", key)
			}
		}
		attempts, err := strconv.Atoi(active.metadata["blocked_attempts"])
		if err != nil || attempts < 3 {
			return fmt.Errorf("blocked ACTIVE_WORK requires blocked_attempts of at least 3")
		}
		if err := validateEvidence(root, active.metadata["blocker_evidence"], "blocker evidence"); err != nil {
			return err
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(active.sections["Blocking side task"])), "none") {
			return fmt.Errorf("blocked ACTIVE_WORK must describe its blocking side task")
		}
	} else {
		for _, key := range []string{"blocker_id", "blocked_attempts", "blocker_evidence", "resume_condition", "return_point"} {
			if active.metadata[key] != "" {
				return fmt.Errorf("%s ACTIVE_WORK must not retain blocked-only field %q", status, key)
			}
		}
	}
	return nil
}

func validateRoadmapEvidence(root string, milestones []milestone) error {
	for _, entry := range milestones {
		for ordinal := range entry.itemEvidence {
			if ordinal > entry.completed {
				return fmt.Errorf("unchecked item %s.%d must not have completion evidence", entry.key, ordinal)
			}
		}
		if entry.completed == 0 {
			continue
		}
		if entry.completed == entry.tasks {
			if entry.evidencePath == "" {
				return fmt.Errorf("completed milestone %s has no Evidence link", entry.key)
			}
			if err := validateEvidence(root, entry.evidencePath, "completed milestone "+entry.key); err != nil {
				return err
			}
			continue
		}
		for ordinal := 1; ordinal <= entry.completed; ordinal++ {
			path := entry.itemEvidence[ordinal]
			if path == "" {
				return fmt.Errorf("completed item %s.%d has no item Evidence link", entry.key, ordinal)
			}
			if err := validateEvidence(root, path, fmt.Sprintf("completed item %s.%d", entry.key, ordinal)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateEvidence(root, path, label string) error {
	plansDir := filepath.Join(root, "docs", "plans")
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve repository root for %s: %w", label, err)
	}
	allowedDir, err := filepath.EvalSymlinks(filepath.Join(root, "docs", "status"))
	if err != nil {
		return fmt.Errorf("resolve docs/status for %s: %w", label, err)
	}
	allowedRelative, err := filepath.Rel(resolvedRoot, allowedDir)
	if err != nil || filepath.IsAbs(allowedRelative) || filepath.ToSlash(allowedRelative) != "docs/status" {
		return fmt.Errorf("docs/status must resolve to the repository docs/status directory")
	}
	candidate, err := filepath.EvalSymlinks(filepath.Clean(filepath.Join(plansDir, filepath.FromSlash(path))))
	if err != nil {
		return fmt.Errorf("%s is unavailable: %w", label, err)
	}
	relative, err := filepath.Rel(allowedDir, candidate)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s must resolve inside docs/status", label)
	}
	info, err := os.Stat(candidate)
	if err != nil {
		return fmt.Errorf("stat %s: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must resolve to a regular file", label)
	}
	return nil
}

func validateBaseCommit(root, commit string) error {
	if !commitPattern.MatchString(commit) {
		return fmt.Errorf("ACTIVE_WORK base_commit must be a 7-40 character lowercase Git object ID")
	}
	if output, err := exec.Command("git", "-C", root, "cat-file", "-e", commit+"^{commit}").CombinedOutput(); err != nil {
		return fmt.Errorf("ACTIVE_WORK base_commit is not a Git commit: %s", strings.TrimSpace(string(output)))
	}
	if err := exec.Command("git", "-C", root, "merge-base", "--is-ancestor", commit, "HEAD").Run(); err != nil {
		return fmt.Errorf("ACTIVE_WORK base_commit %s is not an ancestor of HEAD", commit)
	}
	return nil
}

func findRepoRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(current, "AGENTS.md")) && fileExists(filepath.Join(current, "go.mod")) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("repository root containing AGENTS.md and go.mod was not found")
		}
		current = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func readActiveWork(path string) (activeWork, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return activeWork{}, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return activeWork{}, fmt.Errorf("%s must start with YAML-style front matter", path)
	}
	metadata := make(map[string]string)
	closing := -1
	for index := 1; index < len(lines); index++ {
		line := lines[index]
		if line == "---" {
			closing = index
			break
		}
		key, value, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return activeWork{}, fmt.Errorf("invalid ACTIVE_WORK front-matter line %q", line)
		}
		key = strings.TrimSpace(key)
		if _, duplicate := metadata[key]; duplicate {
			return activeWork{}, fmt.Errorf("duplicate ACTIVE_WORK front-matter key %q", key)
		}
		metadata[key] = strings.TrimSpace(value)
	}
	if closing < 0 {
		return activeWork{}, fmt.Errorf("%s front matter is not closed", path)
	}

	sections := make(map[string]string)
	current := ""
	for _, line := range lines[closing+1:] {
		if strings.HasPrefix(line, "## ") {
			current = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if _, duplicate := sections[current]; duplicate {
				return activeWork{}, fmt.Errorf("duplicate ACTIVE_WORK section %q", current)
			}
			sections[current] = ""
			continue
		}
		if current != "" {
			sections[current] += line + "\n"
		}
	}
	for _, name := range requiredSections {
		if strings.TrimSpace(sections[name]) == "" {
			return activeWork{}, fmt.Errorf("ACTIVE_WORK section %q is missing or empty", name)
		}
	}
	return activeWork{metadata: metadata, sections: sections}, nil
}

func readRoadmap(path string) (roadmapState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return roadmapState{}, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	state := roadmapState{}
	current := -1
	seenUnchecked := false
	seenMilestones := make(map[string]bool)
	for _, line := range lines {
		if match := milestonePattern.FindStringSubmatch(line); match != nil {
			if seenMilestones[match[1]] {
				return roadmapState{}, fmt.Errorf("duplicate roadmap milestone %s", match[1])
			}
			seenMilestones[match[1]] = true
			state.milestones = append(state.milestones, milestone{key: match[1], itemEvidence: make(map[int]string)})
			current = len(state.milestones) - 1
			continue
		}
		if current < 0 {
			continue
		}
		if match := itemPattern.FindStringSubmatch(line); match != nil {
			state.milestones[current].tasks++
			checked := strings.EqualFold(match[1], "x")
			if checked {
				state.milestones[current].completed++
				if seenUnchecked {
					return roadmapState{}, fmt.Errorf("checked roadmap item appears after the first unchecked item: %s", match[2])
				}
				continue
			}
			if !seenUnchecked {
				ordinal := state.milestones[current].tasks
				state.first = &roadmapItem{
					key:       fmt.Sprintf("%s.%d", state.milestones[current].key, ordinal),
					milestone: state.milestones[current].key,
					text:      strings.TrimSpace(match[2]),
				}
				seenUnchecked = true
			}
			continue
		}
		if match := evidencePattern.FindStringSubmatch(line); match != nil {
			state.milestones[current].evidencePath = match[1]
			continue
		}
		if match := itemEvidencePattern.FindStringSubmatch(line); match != nil {
			itemMilestone, ordinalText, _ := strings.Cut(match[1], ".")
			if itemMilestone != state.milestones[current].key {
				return roadmapState{}, fmt.Errorf("item evidence %s is under milestone %s", match[1], state.milestones[current].key)
			}
			ordinal, _ := strconv.Atoi(ordinalText)
			if ordinal < 1 || ordinal > state.milestones[current].tasks {
				return roadmapState{}, fmt.Errorf("item evidence %s does not refer to a preceding roadmap item", match[1])
			}
			if _, duplicate := state.milestones[current].itemEvidence[ordinal]; duplicate {
				return roadmapState{}, fmt.Errorf("duplicate item evidence for %s", match[1])
			}
			state.milestones[current].itemEvidence[ordinal] = match[2]
		}
	}
	if len(state.milestones) == 0 {
		return roadmapState{}, fmt.Errorf("roadmap has no milestones")
	}
	for _, entry := range state.milestones {
		if entry.tasks == 0 {
			return roadmapState{}, fmt.Errorf("roadmap milestone %s has no tasks", entry.key)
		}
	}
	return state, nil
}

func readBriefState(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var milestone, item string
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if value, found := strings.CutPrefix(line, "Current milestone:"); found {
			if milestone != "" {
				return "", "", fmt.Errorf("PROJECT_BRIEF has duplicate Current milestone markers")
			}
			milestone = strings.TrimSpace(value)
		}
		if value, found := strings.CutPrefix(line, "Current roadmap item:"); found {
			if item != "" {
				return "", "", fmt.Errorf("PROJECT_BRIEF has duplicate Current roadmap item markers")
			}
			item = strings.TrimSpace(value)
		}
	}
	if !milestoneKeyPattern.MatchString(milestone) {
		return "", "", fmt.Errorf("invalid PROJECT_BRIEF current milestone %q", milestone)
	}
	if !itemKeyPattern.MatchString(item) {
		return "", "", fmt.Errorf("invalid PROJECT_BRIEF current roadmap item %q", item)
	}
	return milestone, item, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Execution governance check failed:", err)
	os.Exit(1)
}
