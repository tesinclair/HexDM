package pam

/*
#cgo LDFLAGS: -lpam
#include "pam_raw.h"
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"
	"errors"
	"fmt"
	//"os/exec"
)

var (
	ErrAuthFailed = errors.New("Authentication Failed.")
	ErrProbeFailed = errors.New("Failed to probe FPrintD.")

	//ErrFellthrough = errors.New("An error was uncaught.")
)

func ProbeFprintd(user string) (bool, error){
	return false, nil
}

func Authenticate(user, pass string, fprintd bool) (bool, error){
	cUser := C.CString(user)
	defer C.free(unsafe.Pointer(cUser))

	if fprintd{
		pass = "" // pass is empty for fingerprint
	}
	cPass := C.CString(pass)
	defer C.free(unsafe.Pointer(cPass))

	ret := int(C.authenticate(cUser, cPass))
	switch(ret){
	case C.SUCCESS:
		return true, nil
	case C.PAM_AUTH_ERR:
		return false, nil
	default:
		return false, fmt.Errorf("Authentication Error: %w", C.GoString(C.get_err_code(C.int(ret))))
	}
}

