package session 

import (
	"errors"
	"os"
	"os/exec"
	"fmt"

	"golang.org/x/sys/unix"

	"github.com/godbus/dbus/v5"
)

var (
	//QtileNotFound error = errors.New("Qtile not found on system!")
	FIFOPollTimeout error = errors.New("Timeout reached when polling fifo!")
	FIFOPollFailed error = errors.New("Failed to poll the fifo with POLLIN!")
)

const (
     // TODO(tesinclair): These need to be fetched dynamically
	blykid int = 1000
	seat string = "seat0"
)

type DesktopSession struct{
	session_id string
	object_path dbus.ObjectPath
	runtime_path string
	fifo_fd dbus.UnixFD
	uid uint32
	seat_id string
	vtnr uint32
	existing bool
}

func CreateSession() (DesktopSession, error){
	conn, err := dbus.ConnectSystemBus()
	if err != nil{
		return DesktopSession{}, err // pass it up the chain
	}
	defer conn.Close()

	unix_pidfd, err := unix.PidfdOpen(unix.Getpid(), 0) // Getpid() doesn't error
	if err != nil{
		return DesktopSession{}, err
	}
	defer unix.Close(unix_pidfd)

	pidfd := dbus.UnixFD(unix_pidfd)

	var ret DesktopSession; 

	err = conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").Call("org.freedesktop.login1.Manager.CreateSessionWithPIDFD", 0,
									blykid, //  u uid
									pidfd, // h pidfd
									"blykdm", // s service
									"wayland", // s type
									"user", // s class
									"qtile", // s desktop
									seat, // s seat
									0, // u vtnr -- 0 means have logind auto assign
									"", // s tty -- again auto allocation
									"", // s display -- only used for X11
									false, // b remote -- we are not remote
									"", // s remote_user -- we are not remote
									"", // s remote_host -- we are not remote
									uint64(0), // u flags
									map[string]dbus.Variant{}, // a(sv) properties
								).Store(
									&ret.session_id,
									&ret.object_path,
									&ret.runtime_path,
									&ret.fifo_fd,
									&ret.uid,
									&ret.seat_id,
									&ret.vtnr,
									&ret.existing,
								)
	if err != nil{
		return DesktopSession{}, err
	}

	return ret, nil
}

func (d *DesktopSession) RunSession() error{
	pfd := unix.PollFd{
		Fd: int32(d.fifo_fd),
		Events: unix.POLLIN,
	}
	fds := []unix.PollFd{pfd}
	n, err := unix.Poll(fds, 5000)
	if err != nil{
		return err
	}

	if n == 0{
		return FIFOPollTimeout
	}

	if fds[0].Revents & (unix.POLLHUP | unix.POLLERR | unix.POLLINVAL) != 0{
		return FIFOPollFailed
	}

	if (fds[0].Revents & unix.POLLIN) == 0{
		return FIFOPollFailed
	}

	os.Setenv("XDG_RUNTIME_DIR", fmt.Sprintf("/run/user/%v", blykid))	
	os.Setenv("XDG_SESSION_TYPE", "wayland")
	os.Setenv("XDG_SESSION_CLASS", "user")
	os.Setenv("XDG_SESSION_DESKTOP", "qtile")
	os.Setenv("XDG_CURRENT_DESKTOP", "qtile")
	os.Setenv("XDG_SEAT", fmt.Sprintf("%v", d.seat_id))
	os.Setenv("XDG_VTNR", fmt.Sprintf("%v", d.vtnr))

	os.Setenv("QT_QPA_PLATFORM", "wayland")
	os.Setenv("SDL_VIDEODRIVER", "wayland")
	os.Setenv("MOZ_ENABLE_WAYLAND", "1")
	os.Setenv("GDK_BACKEND", "wayland")
	os.Setenv("LANG", "en_GB.UTF-8")

	cmd := exec.Command("dbus-run-session", "qtile", "start", "-b", "wayland")
	if cmd.Err != nil{
		return cmd.Err
	}

	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	return cmd.Run()

}

// should match every call to create session
// Returns nothing; panics on errors
func (d *DesktopSession) DestroySession(){
	conn, err := dbus.ConnectSystemBus()
	if err != nil{
		panic(err)
	}
	defer conn.Close()

	call := conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").Call("org.freedesktop.login1.Manager.TerminateSession", 0, d.session_id)
	if call.Err != nil{
		panic(call.Err)
	}
}
