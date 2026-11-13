//go:build !windows

package openssh

import (
	"context"
	"errors"
	"io"
)

type consoleRunner struct{ started func() }

func (consoleRunner) Run(context.Context, string, []string, io.Reader, io.Writer, io.Writer) (int, error) {
	return -1, errors.New("native terminal launch is available on Windows")
}
