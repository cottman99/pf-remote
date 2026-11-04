package openssh

import (
	"context"
	"errors"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/cottman99/pf-remote/internal/route"
	"github.com/cottman99/pf-remote/internal/session"
	"github.com/cottman99/pf-remote/internal/shellbinding"
)

// ForwardExecutor uses the installed OpenSSH client for an authorized loopback
// forward. The coordinator owns authorization and the Device-signed binding.
// No remote command, agent forwarding or Gateway-side decryption is involved.
type ForwardExecutor struct {
	Executor Executor
	Port     uint16
	Ready    chan<- route.Candidate
}

func (f ForwardExecutor) Execute(ctx context.Context, execution session.Execution) (int, error) {
	if f.Port == 0 || f.Ready == nil || !remoteUserPattern.MatchString(execution.RemoteUser) || execution.Route.Validate() != nil || execution.Binding.Signature == "" || execution.Session.CanonicalTarget == "" {
		return -1, errors.New("SSH Desktop forward is not configured")
	}
	alias := HostKeyAlias(execution.Session.CanonicalTarget)
	knownHosts, err := shellbinding.KnownHosts(alias, &execution.Binding)
	if err != nil {
		return -1, errors.New("SSH Desktop target binding is unavailable")
	}
	knownHostsPath, removeKnownHosts, err := f.Executor.publishKnownHosts(knownHosts)
	if err != nil {
		return -1, err
	}
	defer removeKnownHosts()
	binary := f.Executor.Binary
	if binary == "" {
		binary = "ssh"
	}
	if filepath.Base(binary) == binary {
		binary, err = exec.LookPath(binary)
		if err != nil {
			return -1, errors.New("OpenSSH client is unavailable")
		}
	}
	execution.Command = nil
	args := buildArgs(execution, alias, knownHostsPath)
	host := args[len(args)-1]
	args = append(args[:len(args)-1], "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-W", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(f.Port))), host)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return -1, errors.New("SSH Desktop listener is unavailable")
	}
	defer listener.Close()
	var connections sync.Map
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
		case <-stop:
		}
		_ = listener.Close()
		connections.Range(func(key, _ any) bool { _ = key.(net.Conn).Close(); return true })
	}()
	runner := f.Executor.Runner
	if runner == nil {
		runner = forwardProcessRunner{}
	}
	candidate := route.Candidate{ID: "ssh-desktop-forward", Adapter: execution.Route.Adapter, Network: "tcp", Address: "127.0.0.1", Port: uint16(listener.Addr().(*net.TCPAddr).Port)}
	select {
	case f.Ready <- candidate:
	case <-ctx.Done():
		return -1, ctx.Err()
	}
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return -1, ctx.Err()
		}
		connections.Store(connection, true)
		go func() {
			defer connection.Close()
			defer connections.Delete(connection)
			_, _ = runner.Run(ctx, binary, args, connection, connection, io.Discard)
		}()
	}
}

type forwardProcessRunner struct{}

func (forwardProcessRunner) Run(ctx context.Context, binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	command := exec.CommandContext(ctx, binary, args...)
	command.WaitDelay = 2 * time.Second
	configureForwardProcess(command)
	command.Stdin, command.Stdout, command.Stderr = stdin, stdout, stderr
	if err := command.Run(); err != nil {
		return -1, errors.New("SSH Desktop transport ended")
	}
	return 0, nil
}
