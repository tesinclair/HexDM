package system

import (
	"errors"
)

const (
	FprintBinPath = "/usr/libexec/hexdm/hex-auth"

	FakeDesktop = "./testdata/fake.desktop"
)

var (
	ErrFprintPipeFailed = errors.New("Failed to open pipe to fprint auth binary")
	ErrFprintKilled     = errors.New("Fprint killed. No Auth run.")
)

var (
	ERRNO_NoArgs = 3
)
