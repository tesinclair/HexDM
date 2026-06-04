package desktop

import (
	"os"
	"errors"
	"encoding/json"

	"github.com/tesinclair/hexdm/system"
)

var (
	ErrBadArgs = errors.New("A bad argument was given.")
	ErrNoStateForUser = errors.New("No session state found for given user.")
)

type sessionState struct{
	State map[string]string `json:"state"`
}

// Returns the session name for the user
func GetSessionState(user string) (string, error){
	if os.Getenv("HEX_DRYRUN") == "1"{
		return "FakeWM", nil
	}

	if user == ""{
		return "", ErrBadArgs
	}

	stateData, err := os.ReadFile(system.SessionStateFilePath)
	if err != nil{
		return "", err
	}

	fullState := sessionState{}

	if err := json.Unmarshal(stateData, &fullState); err != nil{
		return "", err
	}

	session := fullState.State[user]
	if session == ""{
		return "", ErrNoStateForUser
	}

	return session, nil

}

func SaveSessionState(user, session string) error{
	if os.Getenv("HEX_DRYRUN") == "1"{
		return nil
	}

	if user == "" || session == ""{
		return ErrBadArgs
	}

	prevState := sessionState{}

	stateData, err := os.ReadFile(system.SessionStateFilePath)

	if err != nil && !errors.Is(err, os.ErrNotExist){
		return err
	}else if err == nil{
		if err := json.Unmarshal(stateData, &prevState); err != nil{
			return err
		}
	}
	
	if prevState.State == nil{
		prevState.State = make(map[string]string)
	}
	prevState.State[user] = session

	newStateData, err := json.Marshal(prevState)
	if err != nil{
		return err
	}
	
	_ = os.MkdirAll("/var/lib/hexdm", 0755)
	if err := os.WriteFile(system.SessionStateFilePath, newStateData, 0600); err != nil{
		return err
	}

	return nil
}
