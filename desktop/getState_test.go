package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func useTempState(t *testing.T) string {
	t.Helper()
	t.Setenv("HEX_DRYRUN", "")
	old := stateFilePath
	stateFilePath = filepath.Join(t.TempDir(), "hexdm", "state.json")
	t.Cleanup(func() { stateFilePath = old })
	return stateFilePath
}

func TestSessionStateRoundTrip(t *testing.T) {
	path := useTempState(t)

	if _, err := GetSessionState("ada"); err == nil {
		t.Error("expected an error before anything is saved")
	}
	if err := SaveSessionState("ada", "Sway"); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionState("bob", "Hyprland"); err != nil {
		t.Fatal(err)
	}
	if err := SaveSessionState("ada", "Qtile"); err != nil {
		t.Fatal(err)
	}

	for user, want := range map[string]string{"ada": "Qtile", "bob": "Hyprland"} {
		got, err := GetSessionState(user)
		if err != nil || got != want {
			t.Errorf("GetSessionState(%q) = %q, %v; want %q", user, got, err, want)
		}
	}
	if _, err := GetSessionState("eve"); !errors.Is(err, ErrNoStateForUser) {
		t.Errorf("unknown user: got %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("state file mode = %o, want 600", perm)
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".state-*"))
	if len(left) != 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
}

// A damaged file used to make every later save fail.
func TestSaveSessionStateRecoversFromCorruptFile(t *testing.T) {
	path := useTempState(t)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"state":{"ada":"Sw`), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := GetSessionState("ada"); err == nil {
		t.Error("expected an error reading a corrupt file")
	}
	if err := SaveSessionState("ada", "Sway"); err != nil {
		t.Fatalf("save over a corrupt file: %v", err)
	}
	if got, err := GetSessionState("ada"); err != nil || got != "Sway" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestSessionStateRejectsEmptyArguments(t *testing.T) {
	useTempState(t)
	if err := SaveSessionState("", "Sway"); !errors.Is(err, ErrBadArgs) {
		t.Errorf("empty user: got %v", err)
	}
	if err := SaveSessionState("ada", ""); !errors.Is(err, ErrBadArgs) {
		t.Errorf("empty session: got %v", err)
	}
}
