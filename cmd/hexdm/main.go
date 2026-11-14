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
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"slices"
	"time"

	"github.com/tesinclair/hexdm/desktop"
	"github.com/tesinclair/hexdm/gfx"
	"github.com/tesinclair/hexdm/greeter"
	"github.com/tesinclair/hexdm/pam"
	"github.com/tesinclair/hexdm/settings"
	"github.com/tesinclair/hexdm/system"
	"github.com/tesinclair/hexdm/tui"
)

var (
	ErrNotRoot = errors.New("HexDM must be run as root.")
	ErrNoUsers = errors.New("Cannot find any users in /etc/passwd.")

	ErrNoDesktops = errors.New("No Window Manager Desktop Files found. Will be sent to TTY in future versions.")
)

// How long a "session failed" message stays on screen before HexDM exits.
const sessionErrorPause = 4 * time.Second

// How long the session gets to exit after SIGTERM before it is killed.
const sessionStopTimeout = 10 * time.Second

// How long the greeter gets to let go of the screen after a login is accepted.
// The session is started regardless once this has passed.
const releaseTimeout = 5 * time.Second

// Failed fingerprint attempts a user gets before falling back to a password.
const maxFprintTries = 3

func main() {
	tty, err := system.GetTTY()
	if err != nil || tty == "" {
		log.Println(err)
	}
	os.Setenv("HEX_TTY", tty)

	if os.Getenv("HEX_DEBUG") != "1" {
		// The logs name users and record login activity, so keep them to
		// root. Chmod as well, to tighten files from older versions.
		_ = os.MkdirAll("/var/log/hexdm", 0750)
		_ = os.Chmod("/var/log/hexdm", 0750)
		file, err := os.OpenFile("/var/log/hexdm/hexdm.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
		if err == nil {
			_ = file.Chmod(0640)
		}
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

	// In a dry run the only session is the fake one from testdata.
	var desktops []string
	if os.Getenv("HEX_DRYRUN") == "1" {
		desktops = []string{system.FakeDesktop}
	} else {
		desktops = desktop.GetAll()
	}
	log.Printf("Found desktop files: %v", desktops)

	var desktopNames []string
	var desktopExecs []string

	for _, dp := range desktops{
		name, exec, err := desktop.Parse(dp)
		if err != nil{
			log.Printf("Failed to parse desktop: %s (%v). Skipping.", dp, err)
			continue
		}
		if _, err := desktop.SplitExec(exec); err != nil {
			log.Printf("Desktop %s has an Exec line that can't be run (%q). Skipping.", dp, exec)
			continue
		}
		desktopNames = append(desktopNames, name)
		desktopExecs = append(desktopExecs, exec)
	}

	if len(desktopNames) < 1{
		// TODO: IMPLMENET THIS
		log.Printf("No Desktops. Will send to TTY")
		return ErrNoDesktops
	}

	desktopIdx := 0

	log.Printf("Got Desktops: %v, %v", desktopNames, desktopExecs)


	users, err := system.GetUsers()
	if err != nil {
		return err
	}
	if len(users) < 1 {
		return ErrNoUsers
	}

	currUser := users[0]

	// rememberedSession returns the index of the session the user picked
	// last time. If there is none, or it is no longer installed, the current
	// selection is kept, so the result is always a valid index.
	rememberedSession := func(user string, current int) int {
		name, err := desktop.GetSessionState(user)
		if err != nil {
			log.Printf("Failed to get user session state: %s", err)
			return current
		}
		idx := slices.Index(desktopNames, name)
		if idx < 0 {
			log.Printf("Remembered session %q for %v is not available. Options: %v", name, user, desktopNames)
			return current
		}
		return idx
	}
	desktopIdx = rememberedSession(currUser, desktopIdx)

	var lockedFprintUsers []string
	fprintd, err := system.ProbeFprintd(currUser)
	if err != nil {
		log.Printf("Failed to probe fprintd: %v. Continuing with Password Auth.", err)
		fprintd = false
	}
	// Fingerprint tries left for each user who has been offered the scanner.
	// Kept per user so switching away and back does not hand out a fresh set.
	fprintTriesLeft := make(map[string]int)
	fprintTries := -1
	if fprintd {
		log.Printf("Found fprintd records for user %v on system.", currUser)
		fprintTries = maxFprintTries
		fprintTriesLeft[currUser] = fprintTries
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
	pass := make(chan []byte, 1)
	powerReq := make(chan string, 1)
	dpName := make(chan string, 1)

	link := &greeter.Link{
		Fprintd:       fprintChan,
		FprintdTries:  fprintTriesChan,
		ScannerActive: scannerActiveChan,
		Auth:          auth,
		SessionIdx:    dpIdxChan,
		Kill:          kill,
		User:          user,
		Password:      pass,
		Session:       dpName,
		Power:         powerReq,
		Released:      make(chan struct{}, 1),
	}
	state, err := greeter.New(users, desktopNames, link)
	if err != nil {
		return err
	}
	go func() {
		tuiDone <- runGreeter(state, link)
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

	// If anything below panics, give the screen back before crashing: the
	// greeter holds the display and the keyboard (or, as the TUI, has the
	// terminal in raw mode on the alternate screen).
	defer func() {
		if r := recover(); r != nil {
			select {
			case kill <- true:
			default:
			}
			select {
			case <-tuiDone:
			case <-time.After(2 * time.Second):
			}
			panic(r)
		}
	}()

	// The handler only ever exits when it is killed, and these are the only
	// two places that talk to it, so neither can block on a handler that is
	// no longer there.
	fprintAlive := true
	killFprint := func() {
		if !fprintAlive {
			return
		}
		fprintAlive = false
		fprintKill <- true
		<-fprintDone
	}
	defer killFprint()
	stopFprint := func() {
		if !fprintAlive {
			return
		}
		fprintStop <- true
		<-fprintStopped
		// The helper is gone. Throw away anything it reported before it
		// was stopped, so it cannot be applied to a later selection.
		select {
		case stale := <-fprintAuth:
			log.Printf("Discarding fingerprint result for %v from a stopped helper.", stale.user)
		default:
		}
	}

	chooseSession := func(dpChange string) {
		prevIdx := desktopIdx
		desktopIdx = slices.Index(desktopNames, dpChange)
		log.Printf("User changed desktop session to: %s", dpChange)
		if desktopIdx < 0 {
			log.Printf("User chose desktop that doesn't exist. Options: %v. Using Previous", desktopNames)
			desktopIdx = prevIdx
		}
	}

	switchUser := func(username string) {
		if username == currUser {
			return
		}
		currUser = username
		log.Printf("Detected user change to: %v", currUser)

		stopFprint()
		fprintd, err := system.ProbeFprintd(currUser)
		if err != nil {
			log.Printf("Failed to probe fprintd: %v. Continuing with password auth.", err)
		}

		if slices.Contains(lockedFprintUsers, currUser) {
			log.Printf("User %v locked out of Fprintd.", currUser)
			fprintd = false
		}
		if fprintd && !fprintAlive {
			fprintd = false
		}
		if fprintd {
			log.Printf("Found fprintd records for user %v on system.", currUser)
			changeUser <- currUser
			<-userChanged
			fprintStart <- true
			if _, seen := fprintTriesLeft[currUser]; !seen {
				fprintTriesLeft[currUser] = maxFprintTries
			}
			fprintTries = fprintTriesLeft[currUser]
			fprintTriesChan <- fprintTries
		} else {
			log.Printf("Found no fprintd records for %v on system. Using default auth.", currUser)
		}
		fprintChan <- fprintd

		desktopIdx = rememberedSession(currUser, desktopIdx)
		dpIdxChan <- desktopIdx
	}

	// runSession starts the session for an authenticated user and waits for
	// it to end. HexDM handles one login per run: whatever happens here, we
	// return afterwards and the service manager starts a fresh instance.
	runSession := func(username string, env []string) error {
		log.Println("Saving session state.")
		if err := desktop.SaveSessionState(username, desktopNames[desktopIdx]); err != nil {
			log.Printf("Failed to save user session state: %v", err)
		} else {
			log.Printf("Saved user session state to: %v", system.SessionStateFilePath)
		}

		// The greeter owns the display and the keyboard. The session can't
		// have them until it has let go.
		auth <- greeter.LoginAccepted
		select {
		case <-link.Released:
		case <-time.After(releaseTimeout):
			log.Println("The greeter did not release the screen in time. Starting the session anyway.")
		}
		err := handleUserLoginSession(ctx, desktopExecs[desktopIdx], username, env)

		// On the fingerprint path the helper has been holding the PAM
		// session open for the user's session. Stopping it closes that.
		killFprint()

		if err != nil {
			log.Printf("Error running session: %v", err)
			// Leave the message up long enough to read before we exit.
			auth <- greeter.ErrSessionFailed
			time.Sleep(sessionErrorPause)
			kill <- true
			<-tuiDone
			return fmt.Errorf("session for %v: %w", username, err)
		}

		// session over, close.
		auth <- nil
		<-tuiDone
		log.Println("Session ended.")
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("Cleaning up...")
			kill <- true
			killFprint()
			<-tuiDone
			return ctx.Err()
		case err := <-tuiDone:
			if err == nil {
				log.Println("Logged in user.")
				return nil
			} else if errors.Is(err, greeter.UserQuit) {
				log.Println("User quit session.")
				killFprint()
				return nil
			}
			return err
		case power := <-powerReq:
			log.Printf("Got Power Request: %s from the greeter.", power)
			var verb string
			switch power {
			case greeter.PowerShutdown:
				verb = "poweroff"
			case greeter.PowerReboot:
				verb = "reboot"
			default:
				log.Printf("Unknown power request: %s", power)
				continue
			}
			if os.Getenv("HEX_DRYRUN") == "1" {
				log.Printf("DRYRUN: would run systemctl %s", verb)
				continue
			}
			if err := exec.Command("systemctl", verb).Run(); err != nil {
				log.Printf("Failed to %s: %v", verb, err)
			}
		case dpChange := <-dpName:
			chooseSession(dpChange)
		case username := <-user:
			switchUser(username)
		case password := <-pass:
			// The greeter sends its events in order, but this select does not
			// pick in order. Apply any selection made before the password
			// was submitted, so it is checked against the user (and starts
			// the session) that was on screen.
			select {
			case username := <-user:
				switchUser(username)
			default:
			}
			select {
			case dpChange := <-dpName:
				chooseSession(dpChange)
			default:
			}
			username := currUser
			pamSession.Authenticate(username, password, false)
			clear(password) // don't keep the password around
			if pamSession.Err != nil {
				log.Printf("Failed to authenticate: %v", pamSession.Err)
				auth <- pam.ErrAuthFailed
			} else if pamSession.Success == true {
				log.Printf("Authentication Successful. Logging in %v.", username)
				killFprint()
				return runSession(username, pamSession.Environment)
			} else {
				log.Printf("User %v failed Authentication. Bad Password.", username)
				auth <- errors.New("Invalid Password.")
			}
		case err := <-fprintDone:
			// Not expected: the handler should only exit when killed.
			fprintAlive = false
			if err != nil {
				log.Printf("HEXDM_AUTH ERR: %v", err)
			}
		case res := <-fprintAuth:
			if res.err != nil {
				log.Printf("Fingerprint helper failed for %v: %v. Using password auth.", res.user, res.err)
				lockedFprintUsers = append(lockedFprintUsers, res.user)
				if res.user == currUser {
					fprintChan <- false
				}
				continue
			}
			if res.user != currUser {
				log.Printf("Ignoring fingerprint result for %v: selected user is %v.", res.user, currUser)
				continue
			}
			session := res.session
			log.Printf("Authentication Returned. Success: %v", session.Success)
			if session.Success {
				log.Printf("Authentication successful. Logging in %v.", currUser)

				return runSession(res.user, session.Environment)
			} else {
				stopFprint()
				log.Printf("User %v failed Authentication. Bad Fingerprint.", currUser)
				auth <- greeter.ErrFingerprintNoMatch

				fprintTries -= 1
				fprintTriesLeft[currUser] = fprintTries
				log.Printf("Tries Remaining: %v.", fprintTries)
				if fprintTries < 1{
					lockedFprintUsers = append(lockedFprintUsers, currUser)
					log.Printf("User %v is now locked out of Fprint.", currUser)
				}else{
					fprintStart <- true
				}

				fprintTriesChan <- fprintTries
			}

		}
	}
}

// runGreeter shows the login screen until it is told to stop. The rendered
// greeter is used when it can be. If the graphics won't start, the terminal
// one takes over, so a broken driver never locks anyone out.
func runGreeter(state *greeter.State, link *greeter.Link) error {
	events := link.Events()
	debug := os.Getenv("HEX_DEBUG") == "1"

	// When debugging, a window is only possible from inside a desktop.
	// Without one, don't take the real display away from whoever has it.
	windowed := debug && (os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") != "")
	if os.Getenv("HEX_TUI") == "1" || (debug && !windowed) {
		return tui.Run(state, link, events)
	}

	path := settings.DefaultPath
	if custom := os.Getenv("HEX_CONFIG"); custom != "" && debug {
		path = custom
	}
	cfg, problems := settings.Load(path)
	for _, problem := range problems {
		log.Printf("Config: %v", problem)
	}

	err := gfx.Run(state, link, events, cfg, gfx.Options{Windowed: windowed})
	if errors.Is(err, gfx.ErrUnavailable) {
		log.Printf("%v. Falling back to the terminal greeter.", err)
		return tui.Run(state, link, events)
	}
	return err
}

func testRoot() bool {
	return os.Geteuid() == 0
}

// sessionEnv builds the environment for the user's session: the basics every
// login provides, then whatever PAM supplied, which wins on a conflict. The
// result is never nil, because a nil Env would make exec pass on our own
// (root's) environment.
func sessionEnv(home, username, shell, path string, pamEnv []string) []string {
	base := []string{
		"HOME=" + home,
		"USER=" + username,
		"LOGNAME=" + username,
		"SHELL=" + shell,
		"PATH=" + path,
	}

	fromPam := make(map[string]bool)
	for _, kv := range pamEnv {
		name, _, _ := strings.Cut(kv, "=")
		fromPam[name] = true
	}

	env := []string{}
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if !fromPam[name] {
			env = append(env, kv)
		}
	}
	return append(env, pamEnv...)
}

func handleUserLoginSession(ctx context.Context, desktopBin, username string, env []string) error {
	user, err := user.Lookup(username)
	if err != nil {
		return err
	}

	if desktopBin == ""{
		return ErrNoDesktops
	}

	log.Printf("Starting user session for user: %v. With UID: %v, and GID: %v", user.Username, user.Uid, user.Gid)

	log.Printf("Running desktop session command: %s", desktopBin)

	desktopBinCmd, err := desktop.SplitExec(desktopBin)
	if err != nil {
		return fmt.Errorf("%w: %q", err, desktopBin)
	}

	// A dry run takes the same path as a real login, privilege drop
	// included. Only the command differs (it comes from testdata).
	if os.Getenv("HEX_DRYRUN") == "1" {
		log.Printf("DRYRUN: running command: %s", desktopBin)
	}

	uid, err1 := strconv.ParseUint(user.Uid, 10, 32)
	gid, err2 := strconv.ParseUint(user.Gid, 10, 32)

	if err1 != nil || err2 != nil {
		return errors.New(fmt.Sprintf("Unable to intify Uid(%v) or Gid(%v)", user.Uid, user.Gid))
	}

	// Without an explicit list Go clears the supplementary groups, and the
	// session would run without video, input, audio, wheel and so on.
	groupIDs, err := user.GroupIds()
	if err != nil {
		return fmt.Errorf("looking up groups for %v: %w", user.Username, err)
	}
	groups := make([]uint32, 0, len(groupIDs))
	for _, id := range groupIDs {
		g, err := strconv.ParseUint(id, 10, 32)
		if err != nil {
			return fmt.Errorf("bad group id %q for %v", id, user.Username)
		}
		groups = append(groups, uint32(g))
	}

	// The child applies these as setgroups, then setgid, then setuid.
	userCred := syscall.Credential{
		Uid:    uint32(uid),
		Gid:    uint32(gid),
		Groups: groups,
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
	path := os.Getenv("PATH")
	if path == "" {
		path = system.DefaultPath
	}
	cmd.Env = sessionEnv(user.HomeDir, user.Username, system.UserShell(user.Username), path, env)
	cmd.Dir = user.HomeDir

	// The session writes to a real pipe. Waiting for the session then only
	// waits for the session process itself, not for every program it left
	// behind that still holds the pipe open, and the reader below keeps
	// draining it however long a line is, so the session can never block
	// on its own output.
	outR, outW, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdout = outW
	cmd.Stderr = outW

	go func() {
		defer outR.Close()
		forEachLine(outR, func(line string) {
			if strings.Contains(strings.ToUpper(line), "ERR") {
				log.Printf("[WINDOW MANAGER LOG]: %s", line)
			}
		})
	}()

	// Pdeathsig fires when the thread that forked the child exits, not when
	// the process does. Pin this goroutine to its thread until the session
	// is over so the Go runtime can't retire that thread underneath it.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	err = cmd.Start()
	outW.Close() // the session has its own copy
	if err != nil {
		log.Println("Failed to start command.")
		return err
	}

	// The main loop is parked here for the whole session, so this is where
	// a SIGTERM to hexdm has to be passed on.
	sessionDone := make(chan struct{})
	defer close(sessionDone)
	go func() {
		select {
		case <-sessionDone:
			return
		case <-ctx.Done():
		}
		log.Println("Asked to stop. Sending SIGTERM to the session.")
		cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-sessionDone:
		case <-time.After(sessionStopTimeout):
			log.Println("Session ignored SIGTERM. Killing it.")
			cmd.Process.Kill()
		}
	}()

	err = cmd.Wait()

	if ctx.Err() != nil {
		// We were told to stop, so however the session exited is fine.
		return nil
	}

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
	err     error // the helper could not be run or gave unusable output
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

// forEachLine calls fn for every line read from r until r is closed. It
// never stops reading early, so whoever is writing to the other end cannot
// block on a full pipe. Lines longer than the buffer are cut short.
func forEachLine(r io.Reader, fn func(line string)) {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, isPrefix, err := br.ReadLine()
		if err != nil {
			return
		}
		fn(string(line))
		for isPrefix {
			if _, isPrefix, err = br.ReadLine(); err != nil {
				return
			}
		}
	}
}

// readHelper reads everything a helper prints until the helper exits, and
// tells the handler about its result or a timeout. done is closed when the
// handler has gone away.
func readHelper(r *os.File, gen int, events chan<- helperEvent, scannerActiveChan chan<- bool, done <-chan struct{}) {
	defer r.Close()

	send := func(ev helperEvent) {
		select {
		case events <- ev:
		case <-done:
		}
	}
	scanning := func(active bool) {
		select {
		case scannerActiveChan <- active:
		case <-done:
		}
	}

	reported := false // a helper gives one result; anything after is only logged
	forEachLine(r, func(data string) {
		data = strings.TrimSpace(data)
		if text, _, ok := strings.Cut(data, "\xFE"); ok {
			log.Printf("[HEXDM_AUTH (Conversation)] %v", text)
			return
		}
		if _, jsonRaw, ok := strings.Cut(data, "\xFF"); ok {
			if reported {
				return
			}
			reported = true
			var ret pam.PamSessionSafe
			if err := json.Unmarshal([]byte(jsonRaw), &ret); err != nil {
				send(helperEvent{gen: gen, err: err})
				return
			}
			scanning(false)
			send(helperEvent{gen: gen, session: &ret})
			return
		}
		if _, msg, ok := strings.Cut(data, "PAM_ERROR_MSG:"); ok {
			if !reported && strings.Contains(msg, "Failed to match fingerprint") {
				scanning(true)
			}
			log.Printf("[HEXDM_AUTH (Pam Error)] %v", msg)
		} else if _, msg, ok := strings.Cut(data, "PAM_TEXT_INFO:"); ok {
			if !reported && strings.Contains(msg, "Verification timed out") {
				reported = true
				scanning(false)
				send(helperEvent{gen: gen, timeout: true})
				return
			}
			log.Printf("[HEXDM_AUTH (Pam Info)] %v", msg)
		} else {
			log.Printf("[HEXDM_AUTH (Debug)] %v", data)
		}
	})
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
	done := make(chan struct{})
	defer close(done)

	var (
		cmd           *exec.Cmd
		helperUser    string // the account the running helper is verifying
		gen           int    // bumped on every launch and every stop
		fprintRunning bool
	)

	launch := func() error {
		log.Println("Fprint Starting...")
		gen++
		cmd = exec.Command(fprintBin, currUser)

		// A real pipe, so that waiting for the helper to exit does not also
		// wait for its output to be read.
		r, w, err := os.Pipe()
		if err != nil {
			return err
		}
		cmd.Stdout = w
		cmd.Stderr = w

		// The helper watches its stdin: we never write to it, and it is
		// closed when we stop the helper or when hexdm itself dies, so a
		// helper can never be left behind holding a session open.
		if _, err := cmd.StdinPipe(); err != nil {
			r.Close()
			w.Close()
			return err
		}

		err = cmd.Start()
		w.Close() // the helper has its own copy
		if err != nil {
			r.Close()
			return err
		}
		go readHelper(r, gen, events, scannerActiveChan, done)

		helperUser = currUser
		fprintRunning = true
		log.Println("Fprint Started.")
		return nil
	}

	// halt stops the running helper and invalidates anything it has not yet
	// reported.
	halt := func() {
		// On SIGTERM a helper that holds a PAM session closes it before
		// exiting. Give it a moment to do that, then insist.
		cmd.Process.Signal(syscall.SIGTERM)
		exited := make(chan struct{})
		go func() {
			cmd.Wait()
			close(exited)
		}()
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			log.Println("Fprint helper ignored SIGTERM. Killing it.")
			cmd.Process.Kill()
			<-exited
		}
		fprintRunning = false
		gen++
	}

	// report hands a result to the main loop without ever blocking: this
	// goroutine is the only sender, so if an older result is still unread
	// it is replaced.
	report := func(res fprintResult) {
		for {
			select {
			case fprintAuth <- res:
				return
			default:
			}
			select {
			case <-fprintAuth:
			default:
			}
		}
	}

	// start launches a helper for the current user. A failure is reported
	// to the main loop rather than ending the handler, because the main
	// loop keeps talking to this goroutine until it kills it.
	start := func() {
		if err := launch(); err != nil {
			log.Printf("Failed to start Fprint helper: %v", err)
			report(fprintResult{user: currUser, err: err})
		}
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
				report(fprintResult{user: helperUser, err: ev.err})
			case ev.timeout:
				log.Printf("Fprintd Timedout. Restarting...")
				halt()
				start()
			case ev.session != nil:
				log.Println("Auth Returned.")
				if ev.session.User != helperUser {
					halt()
					report(fprintResult{user: helperUser, err: fmt.Errorf("fprint helper answered for user %q, expected %q", ev.session.User, helperUser)})
					continue
				}
				report(fprintResult{user: helperUser, session: *ev.session})
			}
		case doStart := <-fprintStart:
			if !doStart || fprintRunning {
				continue
			}
			start()
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
