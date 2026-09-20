package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

const usage = `PF Remote command line

Usage:
  pfremote list [--json]
  pfremote inspect <target-or-alias> [--json]
  pfremote context <target-or-alias> [--task <text>] [--constraint <text>] [--json]
  pfremote doctor [--updates|--check-updates|--notify-updates] [--json]
  pfremote connect|exec|open ...
`

type actionClient interface {
	List(context.Context) (contracts.CatalogResponse, error)
	Inspect(context.Context, string) (contracts.InspectResponse, error)
	Context(context.Context, string, string, []string) (contracts.ContextResponse, error)
	Doctor(context.Context) (contracts.DoctorResponse, error)
	Connect(context.Context, string) (contracts.ShellActionResponse, error)
	Exec(context.Context, string, []string) (contracts.ShellActionResponse, error)
	Open(context.Context, string) (contracts.DesktopActionResponse, error)
	OpenVia(context.Context, string, string) (contracts.DesktopActionResponse, error)
	SaveDesktopCredentialAndOpen(context.Context, string, string) (contracts.DesktopActionResponse, error)
	SaveDesktopCredentialAndOpenVia(context.Context, string, string, string) (contracts.DesktopActionResponse, error)
}

func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), stdout, stderr)
}

func RunWithInput(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return run(args, stdin, stdout, stderr, localapi.NewClient())
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, client actionClient) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, usage)
		return 0
	}

	jsonOutput, cleanArgs := takeFlag(args, "--json")
	if len(cleanArgs) == 0 {
		return writeError(stderr, jsonOutput, "INVALID_ARGUMENT", "cli", "A PF Remote command is required.", "Run pfremote --help.")
	}
	command := cleanArgs[0]
	rest := cleanArgs[1:]
	timeout := 5 * time.Second
	if command == "connect" || command == "exec" || command == "open" || command == "doctor" {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var value any
	var err error
	switch command {
	case "list":
		value, err = client.List(ctx)
	case "inspect":
		if len(rest) != 1 {
			return writeError(stderr, jsonOutput, "INVALID_ARGUMENT", "cli", "inspect requires one target or alias.", "Run pfremote inspect <target-or-alias> --json.")
		}
		value, err = client.Inspect(ctx, rest[0])
	case "context":
		if len(rest) < 1 {
			return writeError(stderr, jsonOutput, "INVALID_ARGUMENT", "cli", "context requires one target or alias.", "Run pfremote context <target-or-alias> --json.")
		}
		task, constraints, parseErr := parseContextArgs(rest[1:])
		if parseErr != nil {
			return writeError(stderr, jsonOutput, "INVALID_ARGUMENT", "cli", parseErr.Error(), "Run pfremote context --help.")
		}
		value, err = client.Context(ctx, rest[0], task, constraints)
	case "doctor":
		if len(rest) == 0 {
			value, err = client.Doctor(ctx)
		} else {
			actions := map[string]string{"--updates": "update-status", "--check-updates": "check-updates", "--notify-updates": "notify-updates"}
			if len(rest) != 1 || actions[rest[0]] == "" {
				return writeError(stderr, jsonOutput, "INVALID_ARGUMENT", "cli", "Unknown update action.", "Run pfremote --help.")
			}
			updater, ok := client.(interface {
				Updates(context.Context, string) (json.RawMessage, error)
			})
			if !ok {
				return writeError(stderr, jsonOutput, "UPDATE_UNAVAILABLE", "updates", "Update actions are unavailable.", "Update the client.")
			}
			value, err = updater.Updates(ctx, actions[rest[0]])
		}
	case "connect":
		if len(rest) != 1 {
			return writeError(stderr, jsonOutput, "INVALID_ARGUMENT", "connect", "connect requires one computer target.", "Choose one visible Shell target.")
		}
		value, err = client.Connect(ctx, rest[0])
	case "exec":
		if len(rest) < 2 {
			return writeError(stderr, jsonOutput, "INVALID_ARGUMENT", "exec", "exec requires one computer target and a task.", "Choose one visible Shell target and provide a command.")
		}
		value, err = client.Exec(ctx, rest[0], rest[1:])
	case "open":
		credentialStdin, openArgs := takeFlag(rest, "--credential-stdin")
		target, routeAdapter, parseErr := parseOpenArgs(openArgs)
		if parseErr != nil {
			return writeError(stderr, jsonOutput, "INVALID_ARGUMENT", "open", "open requires one computer target.", "Choose one visible Desktop target.")
		}
		if credentialStdin {
			credentialBytes, readErr := io.ReadAll(io.LimitReader(stdin, 4097))
			if readErr != nil || len(credentialBytes) > 4096 {
				clear(credentialBytes)
				return writeError(stderr, jsonOutput, "INVALID_CREDENTIAL_INPUT", "setup", "The desktop password could not be read.", "Retry one-time desktop setup from PF Remote Center.")
			}
			credential := strings.TrimRight(string(credentialBytes), "\r\n")
			clear(credentialBytes)
			value, err = client.SaveDesktopCredentialAndOpenVia(ctx, target, credential, routeAdapter)
			credential = ""
		} else {
			value, err = client.OpenVia(ctx, target, routeAdapter)
		}
	default:
		return writeError(stderr, jsonOutput, "UNKNOWN_COMMAND", "cli", "Unknown command: "+command, "Run pfremote --help.")
	}

	if err != nil {
		var remote *localapi.RemoteError
		if errors.As(err, &remote) {
			return writeFailure(stderr, jsonOutput, remote.Failure)
		}
		return writeError(stderr, jsonOutput, "DAEMON_UNAVAILABLE", "local-api", "PF Remote could not reach its local daemon.", "Start or repair pfremoted, then retry.")
	}
	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if encodeErr := encoder.Encode(value); encodeErr != nil {
			fmt.Fprintln(stderr, encodeErr)
			return 1
		}
		return 0
	}
	writeHuman(stdout, value)
	return 0
}

func parseOpenArgs(args []string) (target, routeAdapter string, err error) {
	for index := 0; index < len(args); index++ {
		if args[index] == "--route" {
			if index+1 >= len(args) || routeAdapter != "" {
				return "", "", errors.New("open route requires one value")
			}
			routeAdapter = strings.TrimSpace(args[index+1])
			index++
			continue
		}
		if strings.HasPrefix(args[index], "--") || target != "" {
			return "", "", errors.New("open requires one target")
		}
		target = args[index]
	}
	if target == "" || (routeAdapter != "" && routeAdapter != "lan" && routeAdapter != "tailscale" && routeAdapter != "frp" && routeAdapter != "legacy-external") {
		return "", "", errors.New("open route is invalid")
	}
	return target, routeAdapter, nil
}

func takeFlag(args []string, flag string) (bool, []string) {
	found := false
	clean := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == flag {
			found = true
			continue
		}
		clean = append(clean, arg)
	}
	return found, clean
}

func parseContextArgs(args []string) (string, []string, error) {
	var task string
	var constraints []string
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			return "", nil, fmt.Errorf("%s requires a value", args[i])
		}
		switch args[i] {
		case "--task":
			task = args[i+1]
		case "--constraint":
			constraints = append(constraints, args[i+1])
		default:
			return "", nil, fmt.Errorf("unknown context option: %s", args[i])
		}
		i++
	}
	return task, constraints, nil
}

func writeHuman(out io.Writer, value any) {
	switch v := value.(type) {
	case contracts.CatalogResponse:
		for _, target := range v.Targets {
			fmt.Fprintf(out, "%-28s %-8s %s\n", target.Alias, target.Capability.Kind, target.Canonical)
		}
	case contracts.InspectResponse:
		fmt.Fprintf(out, "%s\n  device: %s (%s)\n  capability: %s (%s)\n  granted: %t\n", v.Target.Alias, v.Target.Device.DisplayName, v.Target.Device.ID, v.Target.Capability.DisplayName, v.Target.Capability.ID, v.Target.Granted)
	case contracts.ContextResponse:
		fmt.Fprintln(out, v.Envelope)
	case contracts.DoctorResponse:
		fmt.Fprintln(out, "PF Remote doctor:", v.Overall)
		for _, check := range v.Checks {
			fmt.Fprintf(out, "  [%s] %s: %s\n", strings.ToUpper(check.Status), check.Name, check.Summary)
		}
	case contracts.ShellActionResponse:
		fmt.Fprintf(out, "%s: %s\n", v.Target.Alias, v.Status)
		if v.Output != "" {
			fmt.Fprint(out, v.Output)
		}
	case contracts.DesktopActionResponse:
		fmt.Fprintf(out, "%s: desktop %s\n", v.Target.Alias, v.Status)
	}
}

func writeError(out io.Writer, jsonOutput bool, code, stage, summary, remediation string) int {
	failure := contracts.Error{
		SchemaVersion: contracts.ErrorSchema, Code: code, Stage: stage,
		CorrelationID: correlationID(), Summary: summary, Remediation: remediation,
	}
	return writeFailure(out, jsonOutput, failure)
}

func writeFailure(out io.Writer, jsonOutput bool, failure contracts.Error) int {
	if jsonOutput {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		_ = encoder.Encode(failure)
	} else {
		fmt.Fprintf(out, "%s [%s]\n%s\nCorrelation: %s\n", failure.Summary, failure.Code, failure.Remediation, failure.CorrelationID)
	}
	return 2
}

func correlationID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(b)
}
