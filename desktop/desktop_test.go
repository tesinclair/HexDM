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

func TestParseCases(t *testing.T) {
	tests := []struct {
		label, body, name, exec string
	}{
		{"no trailing newline", "[Desktop Entry]\nName=Sway\nExec=sway", "Sway", "sway"},
		{"hash inside a value", "[Desktop Entry]\nName=Sway #1\nExec=sway --config /etc/a#b\n", "Sway #1", "sway --config /etc/a#b"},
		{"comment lines", "# header\n[Desktop Entry]\n# note\nName=Sway\n  # indented\nExec=sway\n", "Sway", "sway"},
		{"spaces around equals", "[Desktop Entry]\nName = Sway\nExec = sway -d\n", "Sway", "sway -d"},
		{"equals inside a value", "[Desktop Entry]\nName=Sway\nExec=env A=b sway\n", "Sway", "env A=b sway"},
		{"localised name ignored", "[Desktop Entry]\nName[de]=Schwanken\nName=Sway\nExec=sway\n", "Sway", "sway"},
		{"first key wins", "[Desktop Entry]\nName=Sway\nExec=sway\nExec=other\n", "Sway", "sway"},
		{"CRLF line endings", "[Desktop Entry]\r\nName=Sway\r\nExec=sway\r\n", "Sway", "sway"},
		{"other groups first", "[Other]\nName=No\nExec=no\n[Desktop Entry]\nName=Sway\nExec=sway\n", "Sway", "sway"},
		{"escapes", "[Desktop Entry]\nName=A\\sB\nExec=sh -c \"echo \\\\\\\\\"\n", "A B", `sh -c "echo \\"`},
	}
	for _, tt := range tests {
		name, exec, err := Parse(writeDesktop(t, tt.body))
		if err != nil {
			t.Errorf("%s: unexpected error %v", tt.label, err)
			continue
		}
		if name != tt.name || exec != tt.exec {
			t.Errorf("%s: got name=%q exec=%q, want name=%q exec=%q", tt.label, name, exec, tt.name, tt.exec)
		}
	}
}

// Keys from a later group, such as a [Desktop Action], must not be used.
func TestParseIgnoresOtherGroups(t *testing.T) {
	body := "[Desktop Entry]\nName=Sway\n[Desktop Action other]\nExec=evil\n"
	if name, exec, err := Parse(writeDesktop(t, body)); err == nil {
		t.Errorf("got name=%q exec=%q, want an error", name, exec)
	}
}
