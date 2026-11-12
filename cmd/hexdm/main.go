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

	desktopIdx := 0

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


	users, err := system.GetUsers()
	if err != nil {
		return err
	}
	if len(users) < 1 {
		return ErrNoUsers
	}

	currUser := users[0]

	userSessionState, err := desktop.GetSessionState(currUser)
	if err != nil{
		log.Printf("Failed to get user session state: %s", err)
	}else{
		desktopIdx = slices.Index(desktopNames, userSessionState)
	}

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
	scannerActiveChan := make(chan bool, 1)
	dpIdxChan := make(chan int, 1)
	dpIdxChan <- desktopIdx

	// Receiving Channels
	tuiDone := make(chan error, 1)
	user := make(chan string, 1)
	pass := make(chan string, 1)
	powerReq := make(chan string, 1)
	dpName := make(chan string, 1)

	go func() {
		tuiDone <- tui.Render(desktopNames, users, fprintChan, kill, scannerActiveChan, auth, user, pass, dpName, powerReq, fprintTriesChan, dpIdxChan)
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
	fprintAuth := make(chan fprintResult, 1)
	userChanged := make(chan bool, 1)
	go func() {
		fprintDone <- handleFprint(currUser, fprintStart, fprintStop, fprintStopped, fprintKill, userChanged, scannerActiveChan, changeUser, fprintAuth)
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
			// The helper for the previous user is gone. Throw away anything
			// it reported before it was stopped, so it cannot be applied to
			// the newly selected user.
			select {
			case stale := <-fprintAuth:
				log.Printf("Discarding fingerprint result for previous user %v.", stale.user)
			default:
			}
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

			userSessionState, err := desktop.GetSessionState(currUser)
			if err != nil{
				log.Printf("Failed to get user session state: %s", err)
			}else{
				desktopIdx = slices.Index(desktopNames, userSessionState)
			}
			dpIdxChan <- desktopIdx

		case password := <-pass:
			username := currUser
			pamSession.Authenticate(username, password, false)
			if pamSession.Err != nil {
				log.Printf("Failed to authenticate: %v", pamSession.Err)
				auth <- pam.ErrAuthFailed
			} else if pamSession.Success == true {
				log.Printf("Authentication Successful. Logging in %v.", username)
				fprintKill <- true // don't need to wait, just cleanup

				log.Println("Saving session state.")
				if err := desktop.SaveSessionState(currUser, desktopNames[desktopIdx]); err != nil{
					log.Printf("Failed to save user session state: %v", err)
				}
				log.Printf("Saved user session state to: %v", system.SessionStateFilePath)

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
		case res := <-fprintAuth:
			if res.user != currUser {
				log.Printf("Ignoring fingerprint result for %v: selected user is %v.", res.user, currUser)
				continue
			}
			session := res.session
			log.Printf("Authentication Returned: %v", session)
			if session.Success {
				log.Printf("Authentication successful. Logging in %v.", currUser)

				log.Println("Saving session state.")
				if err := desktop.SaveSessionState(currUser, desktopNames[desktopIdx]); err != nil{
					log.Printf("Failed to save user session state: %v", err)
				}
				log.Printf("Saved user session state to: %v", system.SessionStateFilePath)

				if err := handleUserLoginSession(desktopExecs[desktopIdx], res.user, session.Environment); err != nil {
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

// fprintResult is what the fingerprint handler reports to the main loop.
// user is the account the helper was launched for. It is recorded on our side
// of the pipe, so a result can never be applied to a different account than
// the one that was actually authenticated.
type fprintResult struct {
	user    string
	session pam.PamSessionSafe
}

// helperEvent is one thing a helper's output reader has to tell the handler.
// gen identifies the helper launch it came from, so anything from a helper
// that has since been stopped is ignored.
type helperEvent struct {
	gen     int
	timeout bool
	err     error
	session *pam.PamSessionSafe
}

func readHelper(r io.Reader, gen int, events chan<- helperEvent, scannerActiveChan chan<- bool) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		data := strings.TrimSpace(scanner.Text())
		if text, _, ok := strings.Cut(data, "\xFE"); ok {
			log.Printf("[HEXDM_AUTH (Conversation)] %v", text)
			continue
		}
		if _, jsonRaw, ok := strings.Cut(data, "\xFF"); ok {
			var ret pam.PamSessionSafe
			if err := json.Unmarshal([]byte(jsonRaw), &ret); err != nil {
				events <- helperEvent{gen: gen, err: err}
				return
			}
			scannerActiveChan <- false
			events <- helperEvent{gen: gen, session: &ret}
			return
		}
		if _, msg, ok := strings.Cut(data, "PAM_ERROR_MSG:"); ok {
			if strings.Contains(msg, "Failed to match fingerprint") {
				scannerActiveChan <- true
			}
			log.Printf("[HEXDM_AUTH (Pam Error)] %v", msg)
		} else if _, msg, ok := strings.Cut(data, "PAM_TEXT_INFO:"); ok {
			if strings.Contains(msg, "Verification timed out") {
				scannerActiveChan <- false
				events <- helperEvent{gen: gen, timeout: true}
				return
			}
			log.Printf("[HEXDM_AUTH (Pam Info)] %v", msg)
		} else {
			log.Printf("[HEXDM_AUTH (Debug)] %v", data)
		}
	}
	if err := scanner.Err(); err != nil {
		events <- helperEvent{gen: gen, err: err}
	}
}

func handleFprint(initialUser string, fprintStart, fprintStop, fprintStopped, fprintKill, userChanged, scannerActiveChan chan bool, changeUser chan string, fprintAuth chan fprintResult) error {
	log.Println("Starting Fprint handler")
	currUser := initialUser

	var fprintBin string
	if bin := os.Getenv("HEX_FPRINT_PATH"); bin != "" && os.Getenv("HEX_DEBUG") == "1" {
		fprintBin = bin
	} else {
		fprintBin = system.FprintBinPath
	}

	log.Printf("Fprint path: %v", fprintBin)

	events := make(chan helperEvent, 4)

	var (
		cmd           *exec.Cmd
		helperPipe    *io.PipeWriter
		helperUser    string // the account the running helper is verifying
		gen           int    // bumped on every launch and every stop
		fprintRunning bool
	)

	launch := func() error {
		log.Println("Fprint Starting...")
		gen++
		cmd = exec.Command(fprintBin, currUser)

		pipereader, pipewriter := io.Pipe()
		cmd.Stdout = pipewriter
		cmd.Stderr = pipewriter

		go readHelper(pipereader, gen, events, scannerActiveChan)

		if err := cmd.Start(); err != nil {
			pipewriter.Close()
			return err
		}
		helperPipe = pipewriter
		helperUser = currUser
		fprintRunning = true
		log.Println("Fprint Started.")
		return nil
	}

	// halt stops the running helper and invalidates anything it has not yet
	// reported.
	halt := func() {
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
		helperPipe.Close()
		fprintRunning = false
		gen++
	}

	for {
		select {
		case ev := <-events:
			if ev.gen != gen || !fprintRunning {
				continue // from a helper that has already been stopped
			}
			switch {
			case ev.err != nil:
				log.Printf("Got auth error: %v. Killing Fprint...", ev.err)
				halt()
				log.Println("Fprint Killed.")
				return ev.err
			case ev.timeout:
				log.Printf("Fprintd Timedout. Restarting...")
				halt()
				if err := launch(); err != nil {
					return err
				}
			case ev.session != nil:
				log.Println("Auth Returned.")
				if ev.session.User != helperUser {
					halt()
					return fmt.Errorf("fprint helper answered for user %q, expected %q", ev.session.User, helperUser)
				}
				fprintAuth <- fprintResult{user: helperUser, session: *ev.session}
			}
		case start := <-fprintStart:
			if !start || fprintRunning {
				continue
			}
			if err := launch(); err != nil {
				return err
			}
		case stop := <-fprintStop:
			if !stop || !fprintRunning {
				fprintStopped <- true
				continue
			}
			log.Println("Stopping Fprint...")
			halt()
			fprintStopped <- true
			log.Println("Fprint Stopped.")
		case kill := <-fprintKill:
			if !kill {
				continue
			}
			log.Println("Kill Signal sent to Fprint...")
			if fprintRunning {
				halt()
			}

			log.Println("Fprint Killed. Exiting.")
			return nil
		case newUser := <-changeUser:
			log.Printf("User Change detected in Fprint from %v to %v.", currUser, newUser)
			currUser = newUser
			userChanged <- true
			log.Println("User changed.")
		}
	}
}
