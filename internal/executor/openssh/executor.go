// Package openssh adapts the platform OpenSSH client to a verified Session.
package openssh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/cottman99/pf-remote/internal/session"
	"github.com/cottman99/pf-remote/internal/shellbinding"
)

var remoteUserPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type Runner interface {
	Run(context.Context, string, []string, io.Reader, io.Writer, io.Writer) (int, error)
}

type Executor struct {
	Binary   string
	StateDir string
	Runner   Runner
}

func (e Executor) Execute(ctx context.Context, execution session.Execution) (int, error) {
	if !remoteUserPattern.MatchString(execution.RemoteUser) {
		return -1, errors.New("OpenSSH remote user is invalid")
	}
	if err := execution.Route.Validate(); err != nil {
		return -1, err
	}
	if execution.Session.CanonicalTarget == "" || execution.Binding.Signature == "" {
		return -1, errors.New("OpenSSH execution has no verified target binding")
	}
	alias := HostKeyAlias(execution.Session.CanonicalTarget)
	knownHosts, err := shellbinding.KnownHosts(alias, &execution.Binding)
	if err != nil {
		return -1, errors.New("prepare strict OpenSSH host-key input")
	}
	knownHostsPath, cleanup, err := e.publishKnownHosts(knownHosts)
	if err != nil {
		return -1, err
	}
	defer cleanup()

	binary := e.Binary
	if binary == "" {
		binary = "ssh"
	}
	if filepath.Base(binary) == binary {
		resolved, lookErr := exec.LookPath(binary)
		if lookErr != nil {
			return -1, errors.New("OpenSSH client is unavailable")
		}
		binary = resolved
	}
	args := buildArgs(execution, alias, knownHostsPath)
	runner := e.Runner
	if runner == nil {
		runner = processRunner{}
	}
	exitCode, err := runner.Run(ctx, binary, args, execution.Stdin, execution.Stdout, execution.Stderr)
	if err != nil {
		return exitCode, err
	}
	return exitCode, nil
}

func HostKeyAlias(canonicalTarget string) string {
	digest := sha256.Sum256([]byte(canonicalTarget))
	return "pfremote-" + hex.EncodeToString(digest[:16])
}

func buildArgs(execution session.Execution, alias, knownHostsPath string) []string {
	options := []string{
		"BatchMode=yes",
		"StrictHostKeyChecking=yes",
		"UserKnownHostsFile=" + openSSHOptionPath(knownHostsPath),
		"GlobalKnownHostsFile=none",
		"HostKeyAlias=" + alias,
		"CheckHostIP=no",
		"UpdateHostKeys=no",
		"VerifyHostKeyDNS=no",
		"CanonicalizeHostname=no",
		"ControlMaster=no",
		"ControlPath=none",
		"ProxyCommand=none",
		"ClearAllForwardings=yes",
		"ForwardAgent=no",
		"ForwardX11=no",
		"PermitLocalCommand=no",
		"RequestTTY=no",
		"EscapeChar=none",
		"LogLevel=ERROR",
	}
	args := []string{"-F", "none"}
	for _, option := range options {
		args = append(args, "-o", option)
	}
	if execution.IdentityFile != "" {
		args = append(args, "-o", "IdentitiesOnly=yes", "-i", execution.IdentityFile)
	}
	args = append(args,
		"-p", strconv.Itoa(int(execution.Route.Port)),
		"-l", execution.RemoteUser,
		execution.Route.Address,
	)
	args = append(args, execution.Command...)
	return args
}

func openSSHOptionPath(path string) string {
	if runtime.GOOS == "windows" {
		path = filepath.ToSlash(path)
	}
	if strings.ContainsAny(path, " \t") {
		return `"` + strings.ReplaceAll(path, `"`, `\"`) + `"`
	}
	return path
}

func (e Executor) publishKnownHosts(content []byte) (string, func(), error) {
	dir := e.StateDir
	if dir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", nil, errors.New("locate protected OpenSSH session state")
		}
		dir = filepath.Join(configDir, "PF Remote", "session")
	}
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, errors.New("create protected OpenSSH session state")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", nil, errors.New("OpenSSH session state directory is unsafe")
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", nil, errors.New("protect OpenSSH session state directory")
	}
	file, err := os.CreateTemp(dir, ".known-hosts-*.tmp")
	if err != nil {
		return "", nil, errors.New("create OpenSSH host-key input")
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	failed := true
	defer func() {
		if failed {
			_ = file.Close()
			cleanup()
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return "", nil, errors.New("protect OpenSSH host-key input")
	}
	if _, err := file.Write(content); err != nil {
		return "", nil, errors.New("write OpenSSH host-key input")
	}
	if err := file.Sync(); err != nil {
		return "", nil, errors.New("sync OpenSSH host-key input")
	}
	if err := file.Close(); err != nil {
		return "", nil, errors.New("close OpenSSH host-key input")
	}
	failed = false
	return path, cleanup, nil
}

type processRunner struct{}

func (processRunner) Run(ctx context.Context, binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	command := exec.CommandContext(ctx, binary, args...)
	command.Stdin, command.Stdout, command.Stderr = stdin, stdout, stderr
	err := command.Run()
	if err == nil {
		return 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		code := exitError.ExitCode()
		if code == 255 {
			return code, errors.New("OpenSSH connection or authentication failed")
		}
		return code, nil
	}
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	return -1, fmt.Errorf("start OpenSSH executor: %w", safeProcessError(err))
}

func safeProcessError(err error) error {
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return errors.New(strings.TrimSpace(pathError.Op))
	}
	return errors.New("process unavailable")
}
