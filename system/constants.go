package system

import (
	"errors"
)

const (
	FprintBinPath = "/usr/libexec/hexdm/hex-auth"

	SessionStateFilePath = "/var/lib/hexdm/state.json"

	FakeDesktop = "./testdata/fake.desktop"

	// Used for the session when we have nothing better.
	DefaultShell = "/bin/sh"
	DefaultPath  = "/usr/local/sbin:/usr/local/bin:/usr/bin:/bin"
)

var (
	ErrFprintPipeFailed = errors.New("Failed to open pipe to fprint auth binary")
	ErrFprintKilled     = errors.New("Fprint killed. No Auth run.")
)

var (
	ERRNO_NoArgs = 3
)
