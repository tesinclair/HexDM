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
	content, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil, ErrReadPasswd
	}

	allUsers := strings.Fields(string(content))
	var users []string
	for _, user := range allUsers {
		if !(len(strings.Split(user, ":")) > 2) {
			continue
		}
		name, uidS := strings.Split(user, ":")[0], strings.Split(user, ":")[2]
		uid, err := strconv.Atoi(uidS)
		if err != nil {
			continue // probably a different formatted passwd entry
		}
		if uid >= 1000 && uid < 65534 {
			log.Printf("Found human user: %v", name)
			users = append(users, name)
		}
	}

	return users, nil
}

// readPasswd returns the fields of every well-formed line in /etc/passwd.
func readPasswd() ([][]string, error) {
	content, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil, ErrReadPasswd
	}

	var entries [][]string
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 7 || strings.HasPrefix(line, "#") {
			continue
		}
		entries = append(entries, fields)
	}
	return entries, nil
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
