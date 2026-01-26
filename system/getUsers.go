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
