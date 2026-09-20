package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/internal/mcpstdio"
)

func main() {
	server := mcpstdio.Server{Client: localapi.NewClient()}
	if err := server.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "PF Remote MCP stopped:", err)
		os.Exit(1)
	}
}
