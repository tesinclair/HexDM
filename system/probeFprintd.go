package system

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

var (
	ErrToxicUser = errors.New("The user contains toxic characters: <;>, <&&>, <||>.")
)

func ProbeFprintd(user string) (bool, error) {
	/*
		Defensive, but entirely uncessesary. User is internal only, and would not
		be possible to be corrupted unless some crazy shit went down
		exec is also a safe function which does not expand.
		Keep for the sake of my hairline, though.
	*/
	if strings.Contains(user, ";") || strings.Contains(user, "&&") || strings.Contains(user, "||") {
		return false, ErrToxicUser
	}

	cmd := exec.Command("fprintd-list", user, "2>&1")
	if cmd.Err != nil {
		return false, cmd.Err
	}

	ret := make([]byte, 1024)

	cmd.Stdout = bytes.NewBuffer(ret)
	err := cmd.Run()
	if err != nil {
		return false, err
	}

	if strings.Contains(string(ret), "#0:") && !strings.Contains(string(ret), "has no fingers enrolled") {
		return true, nil
	}
	return false, nil
}
