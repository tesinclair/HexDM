package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"strconv"
	"syscall"

	"github.com/tesinclair/hexdm/pam"
	"github.com/tesinclair/hexdm/system"
	"github.com/tesinclair/hexdm/tui"
)

var (
	ErrNotRoot = errors.New("HexDM must be run as root.")
	ErrNoUsers = errors.New("Cannot find any users in /etc/passwd.")
)

func main() {
	_ = os.MkdirAll("/var/log/hexdm", 0755)
	file, err := os.OpenFile("/var/log/hexdm/hexdm.log", os.O_APPEND | os.O_CREATE | os.O_WRONLY, 0644)
	if err != nil{
		log.Println("Failed to open log file. Defaulting to Stdout.")
	}else{
		defer file.Close()
		log.SetOutput(file)
	}


	log.Println("Starting Hex...")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Printf("Got signal SIGINT. Exiting.")
			os.Exit(0)
		}
		log.Printf("HEX ERR: %v", err)
		os.Exit(1)
	}

	log.Println("Stopped Hex.")
	os.Exit(0)
}

func run(ctx context.Context) error {
	defer func() { log.Println("Cleaning Up.") }()

	if !testRoot() {
		return ErrNotRoot
	}

	users, err := system.GetUsers()
	if err != nil {
		return err
	}
	if len(users) < 1 {
		return ErrNoUsers
	}

	fprintd, err := system.ProbeFprintd(users[0])
	if err != nil {
		log.Printf("Failed to pribe fprintd: %w. Continuing with Password Auth.", err)
		fprintd = false
	}
	if fprintd {

	}

	pamSession := pam.Setup()
	defer pamSession.Teardown()

	// Sending Channels
	kill := make(chan bool, 1)
	auth := make(chan error, 1)

	// Receiving Channels
	done := make(chan error, 1)
	user := make(chan string, 1)
	pass := make(chan string, 1)

	go func() {
		done <- tui.Render(users, kill, auth, user, pass)
	}()

	for {
		select {
		case <-ctx.Done():
			kill <- true
			<-done
			return ctx.Err()
		case err := <-done:
			if err == nil {
				log.Println("Logged in user.")
				return nil
			} else if errors.Is(err, tui.UserQuit) {
				log.Println("User quit session.")
				return nil
			}
			return err
		case password := <-pass:
			username := <-user
			ok, err := pamSession.Authenticate(username, password, false)
			if err != nil {
				log.Printf("Failed to authenticate: %v", err)
				auth <- pam.ErrAuthFailed
			} else if ok == true {
				log.Printf("Authentication Successful. Logging in %v.", username)

				if err := handleUserLoginSession(username, pamSession.Environment); err != nil {
					log.Printf("Error running session: %v", err)
					auth <- tui.ErrSessionFailed
				}
				// session over, close.
				auth <- nil

			} else {
				log.Printf("User %v failed Authentication. Bad Password.", username)
				auth <- errors.New("Invalid Password.")
			}
		}
	}
	return nil
}

func testRoot() bool {
	currUser, err := user.Current()
	if err != nil {
		log.Printf("Failed to get current user: %v", err)
	}
	if currUser.Uid == "0" {
		return true
	}
	return false

}

func handleUserLoginSession(username string, env []string) error {
	user, err := user.Lookup(username)
	if err != nil {
		return err
	}

	log.Printf("Starting user session for user: %v. With UID: %v, and GUI: %v", user.Username, user.Uid, user.Gid)

	uid, err1 := strconv.Atoi(user.Uid)
	gid, err2 := strconv.Atoi(user.Gid)

	if err1 != nil || err2 != nil {
		return errors.New(fmt.Sprintf("Unable to intify Uid(%v) or Gid(%v)", user.Uid, user.Gid))
	}

	userCred := syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}

	cmd := exec.Command("qtile", "start", "-b", "wayland")
	if cmd.Err != nil {
		return cmd.Err
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &userCred,
		Setsid:     true,
		Pdeathsig:  syscall.SIGTERM,
	}
	cmd.Env = env
	cmd.Dir = user.HomeDir	

	err = cmd.Run()

	if err != nil {
		return err
	}

	return nil
}
