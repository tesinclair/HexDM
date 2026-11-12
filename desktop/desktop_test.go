package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func writeDesktop(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.desktop")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParse(t *testing.T) {
	name, exec, err := Parse(writeDesktop(t, "[Desktop Entry]\nName=Sway\nComment=A WM\nExec=sway --debug\nType=Application\n"))
	if err != nil {
		t.Fatal(err)
	}
	if name != "Sway" || exec != "sway --debug" {
		t.Errorf("got name=%q exec=%q", name, exec)
	}
}

func TestParseFakeDesktop(t *testing.T) {
	name, exec, err := Parse("../testdata/fake.desktop")
	if err != nil {
		t.Fatal(err)
	}
	if name != "FakeWM" || exec != `echo "Hello from Fake test desktop file"` {
		t.Errorf("got name=%q exec=%q", name, exec)
	}
}

// An empty value used to index past the end of the token list and panic.
func TestParseEmptyValue(t *testing.T) {
	for _, body := range []string{
		"[Desktop Entry]\nName=\nExec=sway\n",
		"[Desktop Entry]\nName=Sway\nExec=\n",
	} {
		if _, _, err := Parse(writeDesktop(t, body)); err == nil {
			t.Errorf("Parse(%q): expected an error", body)
		}
	}
}
