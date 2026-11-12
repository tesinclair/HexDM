package desktop

import (
	"slices"
	"testing"
)

func TestSplitExec(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"sway", []string{"sway"}},
		{"Hyprland", []string{"Hyprland"}},
		{"uwsm start hyprland.desktop", []string{"uwsm", "start", "hyprland.desktop"}},
		{"sway   --debug ", []string{"sway", "--debug"}},
		{`echo "Hello from Fake test desktop file"`, []string{"echo", "Hello from Fake test desktop file"}},
		{`env A="b c" Hyprland`, []string{"env", "A=b c", "Hyprland"}},
		{`sh -c "echo \"hi\" \\ \$HOME"`, []string{"sh", "-c", `echo "hi" \ $HOME`}},
		{`prog ""`, []string{"prog", ""}},
		{"gnome-session %U", []string{"gnome-session"}},
		{"prog --rate=100%% %f", []string{"prog", "--rate=100%"}},
	}
	for _, tt := range tests {
		got, err := SplitExec(tt.in)
		if err != nil {
			t.Errorf("SplitExec(%q): unexpected error %v", tt.in, err)
			continue
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("SplitExec(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSplitExecErrors(t *testing.T) {
	for _, in := range []string{"", "   ", `sway "unterminated`, `sway "bad\`, "sway %", "%f", `"" arg`} {
		if got, err := SplitExec(in); err == nil {
			t.Errorf("SplitExec(%q) = %q, want an error", in, got)
		}
	}
}
