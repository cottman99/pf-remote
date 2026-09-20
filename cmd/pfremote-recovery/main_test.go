package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsUnsupportedActionWithoutEchoingPassword(t *testing.T) {
	var output bytes.Buffer
	password := "private recovery password"
	if code := run([]string{"unsupported"}, strings.NewReader(password+"\n"), &output, &output); code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if strings.Contains(output.String(), password) {
		t.Fatal("password was echoed")
	}
}

func TestReadPasswordIsBounded(t *testing.T) {
	if _, err := readPassword(strings.NewReader(strings.Repeat("x", 5000))); err == nil {
		t.Fatal("oversized password was accepted")
	}
}
