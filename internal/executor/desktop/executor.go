// Package desktop launches existing operating-system RDP and VNC clients.
package desktop

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	session "github.com/cottman99/pf-remote/internal/desktop"
	"github.com/cottman99/pf-remote/internal/desktopcredential"
)

type Command interface {
	Run(context.Context, Launch) error
}

type Launch struct {
	Name        string
	Arguments   []string
	Environment map[string]string
	RDP         *RDPOptions
}

type OSCommand struct {
	SignRDP    func(context.Context, string) error
	PrepareRDP func(RDPOptions) error
}

func (runner OSCommand) Run(ctx context.Context, launch Launch) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	arguments := append([]string(nil), launch.Arguments...)
	cleanup := func() {}
	if launch.RDP != nil {
		path, remove, err := writeRDPProfile(*launch.RDP)
		if err != nil {
			return err
		}
		cleanup = remove
		sign := runner.SignRDP
		if sign == nil {
			sign = signRDPProfile
		}
		if err := sign(ctx, path); err != nil {
			cleanup()
			return err
		}
		prepare := runner.PrepareRDP
		if prepare == nil {
			prepare = prepareRDPCredentials
		}
		if err := prepare(*launch.RDP); err != nil {
			cleanup()
			return err
		}
		arguments = append([]string{path}, arguments...)
	}
	command := exec.Command(launch.Name, arguments...)
	if len(launch.Environment) > 0 {
		command.Env = os.Environ()
		for name, value := range launch.Environment {
			command.Env = append(command.Env, name+"="+value)
		}
	}
	if err := command.Start(); err != nil {
		cleanup()
		return err
	}
	// A Desktop client is the user's session, not a request-scoped helper.
	// Reap it asynchronously so the local API can confirm a successful launch
	// without killing a healthy remote desktop when the request timeout ends.
	go func() { _ = command.Wait(); cleanup() }()
	return nil
}

type TrustProvider interface {
	VNCTrustFile(canonicalTarget string) (string, error)
}

type CredentialProvider interface {
	Load(string) ([]byte, error)
}

type Executor struct {
	Command     Command
	Trust       TrustProvider
	Credentials CredentialProvider
	Lookup      func(string) (string, error)
	GOOS        string
}

func (e Executor) Open(ctx context.Context, execution session.Execution) error {
	command := e.Command
	if command == nil {
		command = OSCommand{}
	}
	goos := e.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos != "windows" {
		return errors.New("desktop client launcher is not available on this operating system")
	}
	lookup := e.Lookup
	if lookup == nil {
		lookup = lookupWindowsClient
	}
	endpoint := net.JoinHostPort(execution.Route.Address, portString(execution.Route.Port))
	vncEndpoint := tigerVNCEndpoint(execution.Route.Address, execution.Route.Port)
	switch {
	case execution.Session.Protocol == "rdp" && execution.Session.Authentication == "windows-sso":
		client, err := lookup("mstsc.exe")
		if err != nil {
			return &session.ExecutorFault{Code: "DESKTOP_CLIENT_UNAVAILABLE", Summary: "Windows Remote Desktop is unavailable on this computer.", Remediation: "Repair the Windows Remote Desktop client and try again."}
		}
		// PF Remote can present an authorized RDP target through LAN or an
		// encrypted loopback relay. Remote Credential Guard requires a direct
		// Kerberos/domain path and closes these workgroup or relay sockets before
		// sign-in, so the system client must use its normal credential flow.
		// This is the owner's personal controller. Public mode prevents Windows
		// from retaining credentials the owner explicitly chooses to save.
		options, err := e.rdpOptions(execution, endpoint)
		if err != nil {
			return err
		}
		if err := command.Run(ctx, Launch{Name: client, RDP: &options}); err != nil {
			return &session.ExecutorFault{Code: "DESKTOP_CLIENT_START_FAILED", Summary: "Windows Remote Desktop could not start.", Remediation: "Retry from the signed-in Windows desktop."}
		}
		return nil
	case execution.Session.Protocol == "rdp" && execution.Session.Authentication == "tailscale-device":
		if execution.Route.Adapter != "tailscale" {
			return &session.ExecutorFault{Code: "DESKTOP_ROUTE_AUTH_MISMATCH", Summary: "This desktop identity is limited to its Tailscale path.", Remediation: "Use Tailscale or update the desktop identity for the selected path."}
		}
		client, err := lookup("mstsc.exe")
		if err != nil {
			return &session.ExecutorFault{Code: "DESKTOP_CLIENT_UNAVAILABLE", Summary: "Windows Remote Desktop is unavailable on this computer.", Remediation: "Repair the Windows Remote Desktop client and try again."}
		}
		options, err := e.rdpOptions(execution, endpoint)
		if err != nil {
			return err
		}
		if err := command.Run(ctx, Launch{Name: client, RDP: &options}); err != nil {
			return &session.ExecutorFault{Code: "DESKTOP_CLIENT_START_FAILED", Summary: "Windows Remote Desktop could not start.", Remediation: "Retry from the signed-in Windows desktop."}
		}
		return nil
	case execution.Session.Protocol == "vnc" && execution.Session.Authentication == "x509-route-grant":
		if e.Trust == nil {
			return errors.New("VNC target trust is not configured")
		}
		trustFile, err := e.Trust.VNCTrustFile(execution.Session.CanonicalTarget)
		if err != nil || !filepath.IsAbs(trustFile) {
			return errors.New("VNC target trust is unavailable")
		}
		info, err := os.Stat(trustFile)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("VNC target trust is unavailable")
		}
		client, err := lookup("vncviewer.exe")
		if err != nil {
			return errors.New("VNC client is unavailable")
		}
		return command.Run(ctx, Launch{Name: client, Arguments: []string{"-SecurityTypes=X509None", "-X509CA=" + trustFile, vncEndpoint}})
	case execution.Session.Protocol == "vnc" && execution.Session.Authentication == "tailscale-device":
		if execution.Route.Adapter != "tailscale" {
			return errors.New("VNC target identity requires a Tailscale route")
		}
		client, err := lookup("vncviewer.exe")
		if err != nil {
			return errors.New("VNC client is unavailable")
		}
		return command.Run(ctx, Launch{Name: client, Arguments: []string{vncEndpoint}})
	case execution.Session.Protocol == "vnc" && execution.Session.Authentication == "legacy-vnc-password":
		client, err := lookup("vncviewer.exe")
		if err != nil {
			return errors.New("VNC client is unavailable")
		}
		if e.Credentials == nil {
			return &session.ExecutorFault{Code: "DESKTOP_CREDENTIAL_REQUIRED", Summary: "This desktop needs one-time setup.", Remediation: "Save the desktop password in PF Remote, then retry."}
		}
		credential, err := e.Credentials.Load(execution.Session.CanonicalTarget)
		if err != nil || len(credential) == 0 {
			clear(credential)
			return &session.ExecutorFault{Code: "DESKTOP_CREDENTIAL_REQUIRED", Summary: "This desktop needs one-time setup.", Remediation: "Save the desktop password in PF Remote, then retry."}
		}
		defer clear(credential)
		return command.Run(ctx, Launch{Name: client, Arguments: []string{vncEndpoint}, Environment: map[string]string{"VNC_PASSWORD": string(credential)}})
	default:
		return &session.ExecutorFault{Code: "DESKTOP_PROFILE_UNSUPPORTED", Summary: "This desktop connection profile is incomplete.", Remediation: "Repair this computer connection and try again."}
	}
}

func (e Executor) rdpOptions(execution session.Execution, endpoint string) (RDPOptions, error) {
	o := RDPOptions{Endpoint: endpoint, SessionSelector: execution.Session.SessionSelector}
	if store, ok := e.Credentials.(interface {
		LoadRDP(string, string, string) (desktopcredential.RDPPreset, error)
	}); ok {
		p, err := store.LoadRDP(execution.Session.CanonicalTarget, execution.Session.TargetIdentity, execution.Session.SessionSelector)
		if errors.Is(err, desktopcredential.ErrNotFound) {
			return o, nil
		}
		if err != nil {
			return o, &session.ExecutorFault{Code: "DESKTOP_PRESET_INVALID", Summary: "Saved connection settings no longer match this desktop.", Remediation: "Open connection settings and save this account again."}
		}
		o.Username = p.Username
		o.Password = p.Password
		o.Windowed = !p.FullScreen
		o.CredentialOwner = execution.Session.CanonicalTarget
	}
	return o, nil
}

func prepareRDPCredentials(o RDPOptions) error {
	return desktopcredential.PrepareRDP(o.Endpoint, o.Username, o.Password, o.CredentialOwner)
}

func tigerVNCEndpoint(address string, port uint16) string {
	host := strings.TrimSpace(address)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return host + "::" + portString(port)
}

func lookupWindowsClient(name string) (string, error) {
	if name == "mstsc.exe" {
		if windowsDirectory := strings.TrimSpace(os.Getenv("WINDIR")); windowsDirectory != "" {
			candidate := filepath.Join(windowsDirectory, "System32", name)
			if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
				return candidate, nil
			}
		}
	}
	if resolved, err := exec.LookPath(name); err == nil {
		return resolved, nil
	}
	if name == "vncviewer.exe" {
		if executable, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(executable), "protocol-executors", "tigervnc", name)
			if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
				return candidate, nil
			}
		}
	}
	productDirectory := ""
	switch name {
	case "vncviewer.exe":
		productDirectory = "TigerVNC"
	default:
		return "", errors.New("desktop client is unavailable")
	}
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if root == "" {
			continue
		}
		candidate := filepath.Join(root, productDirectory, name)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", errors.New("desktop client is unavailable")
}

func portString(port uint16) string {
	const digits = "0123456789"
	if port == 0 {
		return "0"
	}
	var buffer [5]byte
	position := len(buffer)
	for port > 0 {
		position--
		buffer[position] = digits[port%10]
		port /= 10
	}
	return string(buffer[position:])
}

var _ session.Executor = Executor{}
