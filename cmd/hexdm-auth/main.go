package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/tesinclair/hexdm/pam"
	"github.com/tesinclair/hexdm/system"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(system.ERRNO_NoArgs)
	}
	user := os.Args[1]

	if os.Getenv("HEX_DEBUG") != "1" {
		_ = os.MkdirAll("/var/log/hexdm", 0755)
		file, err := os.OpenFile("/var/log/hexdm/hexdm-auth.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println("Failed to open log file. Defaulting to Stdout. \xFE")
		} else {
			defer file.Close()
			log.SetOutput(file)
		}
	}

	log.Println("Starting HexDM Authenticator... \xFE")

	if err := run(user); err != nil {
		log.Printf("HEX_AUTH ERR: %v \xFE", err)
		os.Exit(1)
	}

	log.Println("Stopped HexDM Authenticator. \xFE")
}

func run(user string) error {
	pamSession := pam.Setup()
	defer pamSession.Teardown()

	var ret pam.PamSessionSafe
	for {
		log.Printf("Starting Fprintd attempt, for user %v. \xFE\n", user)

		pamSession.Authenticate(user, "", true)
		if pamSession.Err != nil {
			log.Printf("Got error: %v from pam authenticate. \xFE\n", pamSession.Err)
		}
		ret = pamSession.ToSafe()
		break
	}

	retJson, err := json.Marshal(ret)
	if err != nil {
		return err
	}

	fmt.Printf("\xFF%s\n", retJson) // reader looks for \xFF to find json

	log.Printf("Authentication successful, Session data sent. Waiting to die...")
	for {
	}
	log.Printf("Dying...")

	return nil
}
