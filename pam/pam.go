package pam

/*
#cgo LDFLAGS: -lpam
#include "pam_raw.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"unsafe"
)

var (
	ErrAuthFailed    = errors.New("Authentication Failed.")
	ErrNoEnv         = errors.New("No environment found.")
	ErrFprintTimeout = errors.New("Fprint Timedout without input.")
)

type Pam struct {
	pamh           *C.pam_handle_t
	sessionRunning bool
	Environment    []string
	User           string // the user the last Authenticate call was for
	Err            error
	Success        bool
}

type PamSessionSafe struct {
	User        string   `json:"user"`
	Environment []string `json:"environment"`
	Err         string   `json:"error"`
	Success     bool     `json:"success"`
}

func Setup() *Pam {
	return &Pam{
		pamh:           nil,
		sessionRunning: false,
	}
}

func (p *Pam) Teardown() {
	if p.sessionRunning {
		if ret := C.teardown(p.pamh); ret != C.PAM_SUCCESS {
			log.Printf("PAM teardown reported: %s", C.GoString(C.get_err_code(ret)))
		}
		// The handle is freed. Make a second Teardown a no-op.
		p.pamh = nil
		p.sessionRunning = false
	}
}

func (p *Pam) ToSafe() PamSessionSafe {
	return PamSessionSafe{
		User:        p.User,
		Success:     p.Success,
		Err:         fmt.Sprintf("%v", p.Err),
		Environment: p.Environment,
	}
}

func (p *Pam) Authenticate(user, pass string, fprintd bool) {
	if p.sessionRunning {
		// One handle, one session: starting again would leak the open one.
		p.Success = false
		p.Err = errors.New("Authentication Error: a session is already open")
		return
	}
	p.User = user
	cUser := C.CString(user)
	defer C.free(unsafe.Pointer(cUser))

	if fprintd {
		pass = "" // pass is empty for fingerprint
	}
	cPass := C.CString(pass)
	defer C.free(unsafe.Pointer(cPass))

	cTTY := C.CString(os.Getenv("HEX_TTY"))
	defer C.free(unsafe.Pointer(cTTY))

	cFprint := C.int(0)
	if fprintd {
		cFprint = 1
	}

	ret := int(C.authenticate(&p.pamh, cUser, cPass, cTTY, cFprint))
	switch ret {
	case C.PAM_SUCCESS:
		p.sessionRunning = true
		p.Success = true
		p.Err = nil
		if err := p.getPamEnv(); err != nil {
			// The user is authenticated and the session is open, so this
			// is not a reason to fail the login. Leave an empty, non-nil
			// environment: a nil one would make exec hand the session
			// our own (root) environment.
			log.Printf("Could not read the PAM environment: %v", err)
			p.Environment = []string{}
		}
	case C.PAM_AUTH_ERR:
		p.Success = false
		p.Err = nil
	default:
		cErrCode := C.get_err_code(C.int(ret))
		p.Success = false
		p.Err = fmt.Errorf("Authentication Error: %s", C.GoString(cErrCode))
	}
}

func (p *Pam) getPamEnv() error {
	list := C.pam_getenvlist(p.pamh)
	if list == nil {
		return ErrNoEnv
	}
	defer C.free(unsafe.Pointer(list))

	// list is a NULL-terminated array of malloc'd "NAME=value" strings. Copy
	// each one into Go and free it as we go.
	env := []string{}
	for i := 0; ; i++ {
		entry := *(**C.char)(unsafe.Add(unsafe.Pointer(list), uintptr(i)*unsafe.Sizeof(*list)))
		if entry == nil {
			break
		}
		env = append(env, C.GoString(entry))
		C.free(unsafe.Pointer(entry))
	}

	log.Printf("Got PAM Environment: \n\t%v", strings.Join(env, "\n\t"))
	p.Environment = env

	return nil
}
