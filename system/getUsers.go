package system

import (
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
)

var (
	ErrReadPasswd = errors.New("Failed to read /etc/passwd")
)

func GetUsers() ([]string, error) {
	entries, err := readPasswd()
	if err != nil {
		return nil, err
	}
	return humanUsers(entries), nil
}

// humanUsers picks the login names of ordinary accounts out of passwd
// entries.
func humanUsers(entries [][]string) []string {
	var users []string
	for _, fields := range entries {
		name := fields[0]
		uid, err := strconv.Atoi(fields[2])
		if err != nil {
			continue // probably a different formatted passwd entry
		}
		if name != "" && uid >= 1000 && uid < 65534 {
			log.Printf("Found human user: %v", name)
			users = append(users, name)
		}
	}
	return users
}

// readPasswd returns the fields of every well-formed line in /etc/passwd.
func readPasswd() ([][]string, error) {
	content, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil, ErrReadPasswd
	}
	return parsePasswd(string(content)), nil
}

// parsePasswd splits passwd content into one set of fields per line. It
// works line by line: the comment (GECOS) field may contain spaces.
func parsePasswd(content string) [][]string {
	var entries [][]string
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 7 || strings.HasPrefix(line, "#") {
			continue
		}
		entries = append(entries, fields)
	}
	return entries
}

// UserShell returns the login shell of the given user, or /bin/sh if it
// can't be found.
func UserShell(name string) string {
	entries, err := readPasswd()
	if err != nil {
		return DefaultShell
	}
	for _, fields := range entries {
		if fields[0] == name && fields[6] != "" {
			return fields[6]
		}
	}
	return DefaultShell
}
