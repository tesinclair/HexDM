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
	Err         string   `json:error`
	Success     bool     `json:success`
}

func Setup() *Pam {
	return &Pam{
		pamh:           nil,
		sessionRunning: false,
	}
}

func (p *Pam) Teardown() {
	if p.sessionRunning {
		C.teardown(p.pamh)
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

	ret := int(C.authenticate(&p.pamh, cUser, cPass, cTTY))
	switch ret {
	case C.PAM_SUCCESS:
		p.sessionRunning = true
		if err := p.getPamEnv(); err != nil {
			p.Err = fmt.Errorf("Environment error: %w", err)
			p.Success = true
		}
		p.Success = true
		p.Err = nil
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
	delim := C.CString("\xFF")
	defer C.free(unsafe.Pointer(delim))

	cEnv := C.get_pam_env_as_string(p.pamh, delim)
	if cEnv == nil {
		return ErrNoEnv
	}

	defer C.free(unsafe.Pointer(cEnv))

	env := C.GoString(cEnv)

	log.Printf("Got PAM Environment: \n\t%v", strings.ReplaceAll(env, "\xFF", "\n\t"))
	p.Environment = strings.Split(env, "\xFF")
	if len(p.Environment) > 0 {
		p.Environment = p.Environment[:len(p.Environment)-1]
	}

	return nil
}
