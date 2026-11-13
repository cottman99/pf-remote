package actions

import (
	"context"
	"github.com/cottman99/pf-remote/internal/session"
)

type TerminalRunner interface {
	RunTerminal(context.Context, string, func(session.Session)) (ShellRunResult, error)
}
