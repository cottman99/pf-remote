package main

import (
	"os"

	"github.com/cottman99/pf-remote/internal/cli"
)

func main() { os.Exit(cli.RunWithInput(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
