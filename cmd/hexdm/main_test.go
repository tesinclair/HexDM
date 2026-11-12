package main

import (
	"slices"
	"strings"
	"testing"
)

func TestSessionEnv(t *testing.T) {
	got := sessionEnv("/home/ada", "ada", "/bin/zsh", "/usr/bin", []string{"XDG_RUNTIME_DIR=/run/user/1000", "PATH=/from/pam"})
	want := []string{
		"HOME=/home/ada",
		"USER=ada",
		"LOGNAME=ada",
		"SHELL=/bin/zsh",
		"XDG_RUNTIME_DIR=/run/user/1000",
		"PATH=/from/pam",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A nil environment would make os/exec hand the session root's environment.
func TestSessionEnvNeverNil(t *testing.T) {
	got := sessionEnv("/home/ada", "ada", "/bin/sh", "/usr/bin", nil)
	if got == nil || len(got) != 5 {
		t.Fatalf("got %q", got)
	}
}

func TestForEachLine(t *testing.T) {
	long := strings.Repeat("x", 200*1024)
	in := "first\n" + long + "\nlast\nno newline"
	var got []string
	forEachLine(strings.NewReader(in), func(line string) { got = append(got, line) })
	if len(got) != 4 || got[0] != "first" || got[2] != "last" || got[3] != "no newline" {
		t.Fatalf("got %d lines: first=%q", len(got), got[0])
	}
	if len(got[1]) == 0 || len(got[1]) > 64*1024 {
		t.Errorf("long line should be cut to the buffer size, got %d bytes", len(got[1]))
	}
}
