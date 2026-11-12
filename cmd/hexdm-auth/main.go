package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/tesinclair/hexdm/pam"
	"github.com/tesinclair/hexdm/system"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(system.ERRNO_NoArgs)
	}
	user := os.Args[1]

	if os.Getenv("HEX_DEBUG") != "1" {
		// The logs name users and record login activity, so keep them to
		// root. Chmod as well, to tighten files from older versions.
		_ = os.MkdirAll("/var/log/hexdm", 0750)
		_ = os.Chmod("/var/log/hexdm", 0750)
		file, err := os.OpenFile("/var/log/hexdm/hexdm-auth.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
		if err == nil {
			_ = file.Chmod(0640)
		}
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

	// We are asked to stop either with a signal or by hexdm going away,
	// which shows up as end-of-file on the stdin pipe it holds open.
	//
	// While PAM is still waiting for a finger there is nothing to clean up,
	// so we just exit. Once a session is open we must return from run so
	// the deferred Teardown closes it.
	var (
		mu       sync.Mutex
		authDone bool
		stopOnce sync.Once
	)
	stop := make(chan struct{})
	requestStop := func(why string) {
		mu.Lock()
		defer mu.Unlock()
		if !authDone {
			log.Printf("%s before authentication finished. Exiting. \xFE", why)
			os.Exit(1)
		}
		stopOnce.Do(func() { close(stop) })
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		requestStop("Got signal")
	}()
	go func() {
		io.Copy(io.Discard, os.Stdin)
		requestStop("Parent went away")
	}()

	log.Printf("Starting Fprintd attempt, for user %v. \xFE\n", user)

	pamSession.Authenticate(user, nil, true)
	if pamSession.Err != nil {
		log.Printf("Got error: %v from pam authenticate. \xFE\n", pamSession.Err)
	}

	mu.Lock()
	authDone = true
	mu.Unlock()

	ret := pamSession.ToSafe()
	retJson, err := json.Marshal(ret)
	if err != nil {
		return err
	}

	fmt.Printf("\xFF%s\n", retJson) // reader looks for \xFF to find json

	if !ret.Success {
		log.Printf("Authentication failed, result sent. \xFE")
		return nil
	}

	// The PAM session belongs to this process, so stay alive (asleep) for as
	// long as the user's session runs.
	log.Printf("Authentication successful, Session data sent. Holding the PAM session open... \xFE")
	<-stop
	log.Printf("Closing the PAM session. \xFE")

	return nil
}
