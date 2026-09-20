package migration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cottman99/pf-remote/internal/actions"
)

type legacyExternalDefinition struct {
	Executable string  `json:"executable"`
	Arguments  string  `json:"arguments"`
	URI        *string `json:"uri"`
}

type legacyExternalCommand interface {
	Run(context.Context, string) error
}

type legacyOSExternalCommand struct{}

func (legacyOSExternalCommand) Run(ctx context.Context, executable string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	command := exec.Command(executable)
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

// LegacyExternalDesktop launches only exact, privately bound, reviewed local
// applications. Its command map is never serialized or persisted outward.
type LegacyExternalDesktop struct {
	commands map[string]legacyExternalDefinition
	runner   legacyExternalCommand
	goos     string
}

func (d LegacyExternalDesktop) HasTarget(target string) bool {
	_, exists := d.commands[target]
	return exists
}

func (d LegacyExternalDesktop) rekeyTargets(targets map[string]string) LegacyExternalDesktop {
	result := d
	result.commands = make(map[string]legacyExternalDefinition, len(d.commands))
	for target, command := range d.commands {
		if replacement, exists := targets[target]; exists {
			target = replacement
		}
		result.commands[target] = command
	}
	return result
}

// WithoutTargets removes exact targets that have been promoted to a native
// routed Desktop profile, while leaving the read-only legacy source untouched.
func (d LegacyExternalDesktop) WithoutTargets(targets []string) LegacyExternalDesktop {
	if len(targets) == 0 {
		return d
	}
	removed := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		removed[target] = struct{}{}
	}
	result := d
	result.commands = make(map[string]legacyExternalDefinition, len(d.commands))
	for target, command := range d.commands {
		if _, exists := removed[target]; !exists {
			result.commands[target] = command
		}
	}
	return result
}

func (d LegacyExternalDesktop) Run(ctx context.Context, target string) (actions.DesktopRunResult, error) {
	definition, exists := d.commands[target]
	if !exists {
		return actions.DesktopRunResult{}, errors.New("external desktop target is not configured")
	}
	goos := d.goos
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos != "windows" || !validLegacyExternalDefinition(definition) {
		return actions.DesktopRunResult{}, errors.New("external desktop application is unavailable")
	}
	runner := d.runner
	if runner == nil {
		runner = legacyOSExternalCommand{}
	}
	if err := runner.Run(ctx, definition.Executable); err != nil {
		return actions.DesktopRunResult{}, errors.New("external desktop application could not start")
	}
	sessionID, err := externalSessionID()
	if err != nil {
		return actions.DesktopRunResult{}, errors.New("external desktop session identity is unavailable")
	}
	return actions.DesktopRunResult{SessionID: sessionID, Protocol: "external", RenderingEnvironment: "physical", RouteAdapter: "legacy-external"}, nil
}

func (d *LegacyExternalDesktop) bind(target string, connection LegacyCenterConnection) bool {
	definition, ready := legacyExternalDefinitionFor(connection)
	if !ready {
		return false
	}
	if d.commands == nil {
		d.commands = make(map[string]legacyExternalDefinition)
	}
	d.commands[target] = definition
	return true
}

func legacyExternalDefinitionFor(connection LegacyCenterConnection) (legacyExternalDefinition, bool) {
	if !strings.EqualFold(strings.TrimSpace(connection.Protocol), "external") {
		return legacyExternalDefinition{}, false
	}
	for _, candidate := range connection.Routes {
		if candidate.Adapter != "legacy-external" {
			continue
		}
		var definition legacyExternalDefinition
		decoder := json.NewDecoder(bytes.NewBufferString(candidate.PrivateJSON))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&definition); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			return legacyExternalDefinition{}, false
		}
		if validLegacyExternalDefinition(definition) {
			return definition, true
		}
		return legacyExternalDefinition{}, false
	}
	return legacyExternalDefinition{}, false
}

func validLegacyExternalDefinition(definition legacyExternalDefinition) bool {
	executable := strings.TrimSpace(definition.Executable)
	if executable == "" || executable != definition.Executable || !filepath.IsAbs(executable) || strings.TrimSpace(definition.Arguments) != "" {
		return false
	}
	if definition.URI != nil && strings.TrimSpace(*definition.URI) != "" {
		return false
	}
	info, err := os.Stat(executable)
	return err == nil && info.Mode().IsRegular()
}

func externalSessionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "external-" + hex.EncodeToString(value[:]), nil
}

var _ actions.TargetedDesktopRunner = LegacyExternalDesktop{}
