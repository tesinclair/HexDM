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
	"slices"

	"github.com/tesinclair/hexdm/desktop"
	"github.com/tesinclair/hexdm/pam"
	"github.com/tesinclair/hexdm/system"
	"github.com/tesinclair/hexdm/tui"
)

var (
	ErrNotRoot = errors.New("HexDM must be run as root.")
	ErrNoUsers = errors.New("Cannot find any users in /etc/passwd.")

	ErrNoDesktops = errors.New("No Window Manager Desktop Files found. Will be sent to TTY in future versions.")
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
	// MUST BE FIRST
	if !testRoot() {
		return ErrNotRoot
	}

	desktops := desktop.GetAll()
	log.Printf("Found desktop files: %v", desktops)

	var desktopNames []string
	var desktopExecs []string

	for _, dp := range desktops{
		name, exec, err := desktop.Parse(dp)
		if err != nil{
			log.Printf("Failed to parse desktop: %s. Skipping.", dp)
			continue
		}
		desktopNames = append(desktopNames, name)
		desktopExecs = append(desktopExecs, exec)
	}

	if len(desktopNames) < 1{
		// TODO: IMPLMENET THIS
		log.Printf("No Desktops. Will send to TTY")
		desktopNames = append(desktopNames, "TTY")
		return ErrNoDesktops
	}

	if os.Getenv("HEX_DRYRUN") == "1"{
		desktopNames = []string{}[:]
		desktopExecs = []string{}[:]
		name, exec, err := desktop.Parse(system.FakeDesktop)
		if err != nil{
			log.Printf("Failed to parse Fake Desktop. Got Err: %s.", err)
		}else{
			desktopNames = append(desktopNames, name)
			desktopExecs = append(desktopExecs, exec)
		}
	}

	log.Printf("Got Desktops: %v, %v", desktopNames, desktopExecs)

	desktopIdx := 0

	users, err := system.GetUsers()
	if err != nil {
		return err
	}
	if len(users) < 1 {
		return ErrNoUsers
	}

	currUser := users[0]

	var lockedFprintUsers = make([]string, 1)
	fprintd, err := system.ProbeFprintd(currUser)
	if err != nil {
		log.Printf("Failed to pribe fprintd: %w. Continuing with Password Auth.", err)
		fprintd = false
	}
	fprintTries := -1
	if fprintd {
		log.Printf("Found fprintd records for user %v on system.", currUser)
		fprintTries = 3
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
	fprintTriesChan := make(chan int, 1)
	fprintTriesChan <- fprintTries

	// Receiving Channels
	tuiDone := make(chan error, 1)
	user := make(chan string, 1)
	pass := make(chan string, 1)
	powerReq := make(chan string, 1)
	dpName := make(chan string, 1)

	go func() {
		tuiDone <- tui.Render(desktopNames, users, fprintChan, kill, auth, user, pass, dpName, powerReq, fprintTriesChan)
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
			log.Println("Cleaning up...")
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
				fprintKill <- true
				<-fprintDone
				return nil
			}
			return err
		case power := <-powerReq:
			log.Printf("Got Power Request: %s from TUI.", power)
			if power == tui.PowerShutdown{
				cmd := exec.Command("systemctl", "poweroff")
				if cmd.Err != nil{
					log.Printf("Failed to shutdown: %v", cmd.Err)
				}else{
					if err := cmd.Run(); err != nil{
						log.Printf("Failed to shutdown: %v", err)
					}
				}
			}
			if power == tui.PowerReboot{
				cmd := exec.Command("systemctl", "reboot")
				if cmd.Err != nil{
					log.Printf("Failed to reboot: %v", cmd.Err)
				}else{
					if err := cmd.Run(); err != nil{
						log.Printf("Failed to reboot: %v", err)
					}
				}			
			}
		case dpChange := <- dpName:
			prevIdx := desktopIdx
			desktopIdx = slices.Index(desktopNames, dpChange)
			log.Printf("User changed desktop session to: %s", dpChange)
			if desktopIdx < 0{
				log.Printf("User chose desktop that doesn't exist. Options: %v. Using Previous", desktopNames)
				desktopIdx = prevIdx
			}
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

			if slices.Contains(lockedFprintUsers, currUser){
				log.Printf("User %v locked out of Fprintd.", currUser)
				fprintd = false
			}
			if fprintd {
				log.Printf("Found fprintd records for user %v on system.", currUser)
				changeUser <- currUser
				<-userChanged
				fprintStart <- true
				fprintTries = 3
				fprintTriesChan <- fprintTries
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

				if err := handleUserLoginSession(desktopExecs[desktopIdx], username, pamSession.Environment); err != nil {
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
			log.Printf("Authentication Returned: %v", session)
			if session.Success {
				log.Printf("Authentication successful. Logging in %v.", currUser)
				if err := handleUserLoginSession(desktopExecs[desktopIdx], currUser, session.Environment); err != nil {
					log.Printf("Error running session: %v", err)
					auth <- tui.ErrSessionFailed
				}
				auth <- nil
			} else {
				fprintStop <- true
				<-fprintStopped
				log.Printf("User %v failed Authentication. Bad Fingerprint.", currUser)
				auth <- errors.New("Couldn't Match Fingerprint")

				fprintTries -= 1
				log.Printf("Tries Remaining: %v.", fprintTries)
				if fprintTries < 1{
					fprintKill <- true
					fprintChan <- false
					lockedFprintUsers = append(lockedFprintUsers, currUser)
					log.Printf("User %v is now locked out of Fprint.", currUser)
				}else{
					fprintStart <- true
				}

				fprintTriesChan <- fprintTries
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

	if desktopBin == ""{
		return ErrNoDesktops
	}

	log.Printf("Starting user session for user: %v. With UID: %v, and GUI: %v", user.Username, user.Uid, user.Gid)

	log.Printf("Running desktop session command: %s", desktopBin)

	desktopBinCmd := strings.Split(desktopBin, " ")
	if len(desktopBinCmd) < 2{
		desktopBinCmd = append(desktopBinCmd, "") // Bit jank?
	}

	if os.Getenv("HEX_DRYRUN") == "1" {
		log.Printf("DRYRUN: running command: %s", desktopBin)
		cmd := exec.Command(desktopBinCmd[0], desktopBinCmd[1:]...)
		if cmd.Err != nil{
			return cmd.Err
		}
		err := cmd.Run()
		if err != nil{
			return err
		}
		log.Println("DRYRYN: Completed dryrun. Returning.")
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

	cmd := exec.Command(desktopBinCmd[0], desktopBinCmd[1:]...)
	if cmd.Err != nil {
		log.Println("Failed to create command.")
		return cmd.Err
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &userCred,
		Setsid:     true,
		Pdeathsig:  syscall.SIGTERM,
	}
	cmd.Env = env
	cmd.Dir = user.HomeDir

	pipereader, pipewriter := io.Pipe()
	defer pipewriter.Close()
	cmd.Stdout = pipewriter
	cmd.Stderr = pipewriter

	go func() {
		scanner := bufio.NewScanner(pipereader)
		for scanner.Scan(){
			data := scanner.Text()
			if strings.Contains(strings.ToUpper(data), "ERR"){
				log.Printf("[WINDOW MANAGER LOG]: %s", data)
			}
		}
		if scanner.Err() != nil{
			log.Printf("[WINDOW MANAGER PERR]: %s", scanner.Err())
		}
	}()

	err = cmd.Run()

	if err != nil {
		log.Println("Command exited with errors.")
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
			log.Println("Auth Returned.")
			fprintAuth <- s
		}
	}

	return system.ErrFprintKilled
}
