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
	"unsafe"
	"log"
	"strings"
)

var (
	ErrAuthFailed  = errors.New("Authentication Failed.")
	ErrNoEnv = errors.New("No environment found.")
)

type Pam struct{
	pamh *C.pam_handle_t
	sessionRunning bool
	Environment []string
}

func Setup() *Pam{
	return &Pam{
		pamh: nil,
		sessionRunning: false,
	}
}

func (p *Pam) Teardown(){
	if p.sessionRunning{
		C.teardown(p.pamh);
	}
}

func (p *Pam) Authenticate(user, pass string, fprintd bool) (bool, error) {
	cUser := C.CString(user)
	defer C.free(unsafe.Pointer(cUser))

	if fprintd {
		pass = "" // pass is empty for fingerprint
	}
	cPass := C.CString(pass)
	defer C.free(unsafe.Pointer(cPass))

	ret := int(C.authenticate(&p.pamh, cUser, cPass))
	switch ret {
	case C.PAM_SUCCESS:
		p.sessionRunning = true
		if err := p.getPamEnv(); err != nil{
			return false, fmt.Errorf("Environment error: %w", err)
		}
		return true, nil
	case C.PAM_AUTH_ERR:
		return false, nil
	default:
		cErrCode := C.get_err_code(C.int(ret));
		defer C.free(unsafe.Pointer(cErrCode));
		return false, fmt.Errorf("Authentication Error: %w", C.GoString(cErrCode))
	}
}

func (p *Pam) getPamEnv() error{
	delim := C.CString("\xFF");
	defer C.free(unsafe.Pointer(delim))

	cEnv := C.get_pam_env_as_string(p.pamh, delim)
	if cEnv == nil{
		return ErrNoEnv
	}

	defer C.free(unsafe.Pointer(cEnv))

	env := C.GoString(cEnv)

	log.Printf("Got PAM Environment: \n\t%v", strings.ReplaceAll(env, "\xFF", "\n\t"))
	p.Environment = strings.Split(env, "\xFF")
	if len(p.Environment) > 0{
		p.Environment = p.Environment[:len(p.Environment)-1]
	}

	return nil
}
