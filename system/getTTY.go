package system

import "os"

func GetTTY() (string, error) {
	tty, err := os.Readlink("/proc/self/fd/0")
	if err != nil {
		return "", err
	}
	return tty, nil
}
