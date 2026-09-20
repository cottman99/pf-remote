package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cottman99/pf-remote/internal/recovery"
	"github.com/cottman99/pf-remote/internal/state"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(arguments []string, input io.Reader, output, errorOutput io.Writer) int {
	if len(arguments) == 0 {
		fmt.Fprintln(errorOutput, "recovery action is required")
		return 2
	}
	password, err := readPassword(input)
	if err != nil {
		fmt.Fprintln(errorOutput, "recovery password could not be read")
		return 2
	}
	switch arguments[0] {
	case "export":
		set := flag.NewFlagSet("export", flag.ContinueOnError)
		set.SetOutput(io.Discard)
		statePath := set.String("state", "", "Gateway state path")
		if set.Parse(arguments[1:]) != nil || set.NArg() != 0 {
			fmt.Fprintln(errorOutput, "recovery export options are invalid")
			return 2
		}
		if *statePath == "" {
			*statePath, err = state.DefaultPath()
		}
		if err != nil {
			fmt.Fprintln(errorOutput, "Gateway state could not be located")
			return 1
		}
		store, err := state.Open(*statePath)
		if err != nil {
			fmt.Fprintln(errorOutput, "Gateway state could not be opened")
			return 1
		}
		encoded, _, err := recovery.Export(context.Background(), store, password)
		store.Close()
		if err != nil {
			fmt.Fprintln(errorOutput, err)
			return 1
		}
		if _, err := output.Write(encoded); err != nil {
			fmt.Fprintln(errorOutput, "recovery file could not be written")
			return 1
		}
		return 0
	case "restore":
		set := flag.NewFlagSet("restore", flag.ContinueOnError)
		set.SetOutput(io.Discard)
		filePath := set.String("file", "", "Recovery file path")
		destination := set.String("destination", "", "new Gateway state path")
		if set.Parse(arguments[1:]) != nil || set.NArg() != 0 || *filePath == "" {
			fmt.Fprintln(errorOutput, "recovery restore options are invalid")
			return 2
		}
		if *destination == "" {
			*destination, err = state.DefaultPath()
		}
		if err != nil {
			fmt.Fprintln(errorOutput, "Gateway state could not be located")
			return 1
		}
		encoded, err := os.ReadFile(*filePath)
		if err != nil {
			fmt.Fprintln(errorOutput, "recovery file could not be read")
			return 1
		}
		_, err = recovery.RestoreToNewDatabase(context.Background(), *destination, encoded, password)
		if err != nil {
			fmt.Fprintln(errorOutput, err)
			return 1
		}
		return 0
	default:
		fmt.Fprintln(errorOutput, "recovery action is unsupported")
		return 2
	}
}

func readPassword(input io.Reader) (string, error) {
	value, err := bufio.NewReader(io.LimitReader(input, 4097)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value = strings.TrimRight(value, "\r\n")
	if value == "" || len(value) > 4096 {
		return "", errors.New("invalid password")
	}
	return value, nil
}
