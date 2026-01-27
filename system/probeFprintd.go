package system

import (
	"errors"
	"log"
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

	cmd := exec.Command("fprintd-list", user)
	if cmd.Err != nil {
		return false, cmd.Err
	}

	var ret strings.Builder
	cmd.Stdout = &ret
	err := cmd.Run()
	if err != nil {
		return false, err
	}

	log.Printf("Got response: %v from Fprintd about user %v", ret.String(), user)

	if strings.Contains(ret.String(), "#0:") && !strings.Contains(ret.String(), "has no fingers enrolled") {
		return true, nil
	}
	return false, nil
}
