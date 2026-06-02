package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	"github.com/tesinclair/hexdm/desktop"
	"github.com/tesinclair/hexdm/pam"
	"github.com/tesinclair/hexdm/system"
	"github.com/tesinclair/hexdm/tui"
)

var (
	ErrNotRoot = errors.New("HexDM must be run as root.")
	ErrNoUsers = errors.New("Cannot find any users in /etc/passwd.")
)

func main() {
	tty, err := system.GetTTY()
	if err != nil || tty == "" {
		log.Println(err)
	}
	os.Setenv("HEX_TTY", tty)

	if os.Getenv("HEX_DEBUG") != "1" {
		_ = os.MkdirAll("/var/log/hexdm", 0755)
		file, err := os.OpenFile("/var/log/hexdm/hexdm.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println("Failed to open log file. Defaulting to Stdout.")
		} else {
			defer file.Close()
			log.SetOutput(file)
		}
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

	desktops := desktop.GetAll() // TODO(tesinclair): generalise this to use a user config
	log.Printf("Found desktop files: %v", desktops)

	desktopName, desktopBin, err := desktop.Parse(desktops[0])
	if err != nil{
		log.Printf("Failed to parse: %v", err)
	}

	log.Printf("Got Desktop: %s, %s", desktopName, desktopBin)

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

	currUser := users[0]

	fprintd, err := system.ProbeFprintd(currUser)
	if err != nil {
		log.Printf("Failed to pribe fprintd: %w. Continuing with Password Auth.", err)
		fprintd = false
	}
	if fprintd {
		log.Printf("Found fprintd records for user %v on system.", currUser)
	} else {
		log.Printf("Found no fprintd records for %v on system. Using default auth.", currUser)
	}

	pamSession := pam.Setup()
	defer pamSession.Teardown()

	// Sending Channels
	kill := make(chan bool, 1)
	auth := make(chan error, 1)
	fprintChan := make(chan bool, 1) // send whether fprint is available
	fprintChan <- fprintd            // send it initial value

	// Receiving Channels
	tuiDone := make(chan error, 1)
	user := make(chan string, 1)
	pass := make(chan string, 1)

	go func() {
		tuiDone <- tui.Render(users, fprintChan, kill, auth, user, pass)
	}()

	// SendingChannels
	fprintStop := make(chan bool, 1)
	fprintStopped := make(chan bool, 1)
	fprintKill := make(chan bool, 1)
	changeUser := make(chan string, 1)
	fprintStart := make(chan bool, 1)
	fprintStart <- fprintd

	// Receiving Channels
	fprintDone := make(chan error, 1)
	fprintAuth := make(chan pam.PamSessionSafe, 1)
	userChanged := make(chan bool, 1)
	go func() {
		fprintDone <- handleFprint(currUser, fprintStart, fprintStop, fprintStopped, fprintKill, userChanged, changeUser, fprintAuth)
	}()

	for {
		select {
		case <-ctx.Done():
			kill <- true
			fprintKill <- true
			<-tuiDone
			<-fprintDone
			return ctx.Err()
		case err := <-tuiDone:
			if err == nil {
				log.Println("Logged in user.")
				return nil
			} else if errors.Is(err, tui.UserQuit) {
				log.Println("User quit session.")
				return nil
			}
			return err
		case username := <-user:
			if username == currUser {
				continue
			}
			currUser = username
			log.Printf("Detected user change to: %v", currUser)

			fprintStop <- true
			<-fprintStopped
			fprintd, err := system.ProbeFprintd(currUser)
			if err != nil {
				log.Printf("Failed to probe fprintd: %w. Continuing with password auth.", err)
			}

			if fprintd {
				log.Printf("Found fprintd records for user %v on system.", currUser)
				changeUser <- currUser
				<-userChanged
				fprintStart <- true
			} else {
				log.Printf("Found no fprintd records for %v on system. Using default auth.", currUser)
			}
			fprintChan <- fprintd

		case password := <-pass:
			username := currUser
			pamSession.Authenticate(username, password, false)
			if pamSession.Err != nil {
				log.Printf("Failed to authenticate: %v", pamSession.Err)
				auth <- pam.ErrAuthFailed
			} else if pamSession.Success == true {
				log.Printf("Authentication Successful. Logging in %v.", username)
				fprintKill <- true // don't need to wait, just cleanup

				if err := handleUserLoginSession(desktopBin, username, pamSession.Environment); err != nil {
					log.Printf("Error running session: %v", err)
					auth <- tui.ErrSessionFailed
				}
				// session over, close.
				auth <- nil

			} else {
				log.Printf("User %v failed Authentication. Bad Password.", username)
				auth <- errors.New("Invalid Password.")
			}
		case err := <-fprintDone:
			if err != nil {
				log.Println("HEXDM_AUTH ERR: %v", err)
			}
		case session := <-fprintAuth:
			if session.Success {
				log.Printf("Authentication successful. Logging in %v.", currUser)
				if err := handleUserLoginSession(desktopBin, currUser, session.Environment); err != nil {
					log.Printf("Error running session: %v", err)
					auth <- tui.ErrSessionFailed
				}
				auth <- nil
				fprintKill <- true
			} else {
				log.Printf("User %v failed Authentication. Bad Fingerprint.", currUser)
				auth <- errors.New("Fingerprint Not Recognised.")
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

func handleUserLoginSession(desktopBin, username string, env []string) error {
	user, err := user.Lookup(username)
	if err != nil {
		return err
	}

	log.Printf("Starting user session for user: %v. With UID: %v, and GUI: %v", user.Username, user.Uid, user.Gid)

	if os.Getenv("HEX_DRYRUN") == "1" {
		return nil
	}

	uid, err1 := strconv.Atoi(user.Uid)
	gid, err2 := strconv.Atoi(user.Gid)

	if err1 != nil || err2 != nil {
		return errors.New(fmt.Sprintf("Unable to intify Uid(%v) or Gid(%v)", user.Uid, user.Gid))
	}

	userCred := syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}

	cmd := exec.Command(desktopBin)
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

func handleFprint(initialUser string, fprintStart, fprintStop, fprintStopped, fprintKill, userChanged chan bool, changeUser chan string, fprintAuth chan pam.PamSessionSafe) error {
	log.Println("Starting Fprint handler")
	currUser := initialUser

	var fprintBin string
	if bin := os.Getenv("HEX_FPRINT_PATH"); bin != "" && os.Getenv("HEX_DEBUG") == "1" {
		fprintBin = bin
	} else {
		fprintBin = system.FprintBinPath
	}

	log.Printf("Fprint path: %v", fprintBin)

	authErr := make(chan error, 1)
	scannerErr := make(chan error, 1)
	session := make(chan pam.PamSessionSafe, 1)

	var cmd *exec.Cmd

	fprintRunning := false
	for {
		select {
		case scanErr := <-scannerErr:
			return scanErr

		case start := <-fprintStart:
			if !start || fprintRunning {
				continue
			}
			log.Println("Fprint Starting...")

			cmd = exec.Command(fprintBin, currUser)
			if cmd == nil {
				return errors.New("Fprintd binary couldn't be loaded")
			}

			pipereader, pipewriter := io.Pipe()
			defer pipewriter.Close()
			cmd.Stdout = pipewriter
			cmd.Stderr = pipewriter

			go func() {
				scanner := bufio.NewScanner(pipereader)
				for scanner.Scan() {
					data := scanner.Text()
					if len(strings.Split(data, "\xFE")) > 1 {
						text := strings.Split(data, "\xFE")[0]
						log.Printf("HEXDM_AUTH: %v.", text)
						data = strings.TrimPrefix(data, fmt.Sprintf("%s\xFE", text))
					}
					if len(strings.Split(data, "\xFF")) > 1 {
						jsonRaw := strings.Split(data, "\xFF")[1]
						var ret pam.PamSessionSafe
						if err := json.Unmarshal([]byte(jsonRaw), &ret); err != nil {
							authErr <- err
							return
						}

						session <- ret
						return

					} else {
						log.Printf("HEXDM_AUTH: %v.", data)
					}

				}
				if scanner.Err() != nil {
					scannerErr <- scanner.Err()
				}
			}()

			if err := cmd.Start(); err != nil {
				return err
			}
			fprintRunning = true
			log.Println("Fprint Started.")
		case stop := <-fprintStop:
			if !stop || !fprintRunning {
				fprintStopped <- true
				continue
			}
			log.Println("Stopping Fprint...")
			cmd.Process.Signal(syscall.SIGTERM)
			cmd.Wait()
			fprintRunning = false
			fprintStopped <- true
			log.Println("Fprint Stopped.")
		case kill := <-fprintKill:
			if !kill {
				continue
			}
			log.Println("Kill Signal sent to Fprint...")
			if fprintRunning {
				cmd.Process.Signal(syscall.SIGTERM)
				cmd.Wait()
			}

			log.Println("Fprint Killed. Exiting.")
			return nil
		case newUser := <-changeUser:
			log.Printf("User Change detected in Fprint from %v to %v.", currUser, newUser)
			currUser = newUser
			userChanged <- true
			log.Println("User changed.")
		case err := <-authErr:
			if err == nil {
				continue
			}
			log.Printf("Got auth error: %v. Killing Fprint...", err)
			cmd.Process.Signal(syscall.SIGTERM)
			cmd.Wait()
			log.Println("Fprint Killed.")
			return err
		case s := <-session:
			log.Println("Auth Succeeded.")
			fprintAuth <- s
			log.Println("Waiting to die...")
			select {
			case kill := <-fprintKill:
				if !kill {
					continue
				}
				cmd.Process.Signal(syscall.SIGTERM)
				cmd.Wait()
				log.Println("Dying...")
				return nil
			}
		}
	}

	return system.ErrFprintKilled
}
