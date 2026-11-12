package desktop

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/tesinclair/hexdm/system"
)

var (
	ErrBadArgs = errors.New("A bad argument was given.")
	ErrNoStateForUser = errors.New("No session state found for given user.")
)

// Where the state lives. A variable only so tests can point it elsewhere.
var stateFilePath = system.SessionStateFilePath

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

	stateData, err := os.ReadFile(stateFilePath)
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

	stateData, err := os.ReadFile(stateFilePath)

	if err != nil && !errors.Is(err, os.ErrNotExist){
		return err
	}else if err == nil{
		if err := json.Unmarshal(stateData, &prevState); err != nil{
			// A damaged file must not stop choices being saved for good.
			// Start again from empty; it only holds remembered sessions.
			log.Printf("Session state file is not valid JSON (%v). Starting a new one.", err)
			prevState = sessionState{}
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
	
	// Write a temporary file next to the real one and rename it into place,
	// so a crash part-way through can never leave a half-written state file.
	dir := filepath.Dir(stateFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil{
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.json") // created with mode 0600
	if err != nil{
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once the rename has happened

	if _, err := tmp.Write(newStateData); err != nil{
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil{
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil{
		return err
	}

	return os.Rename(tmp.Name(), stateFilePath)
}
