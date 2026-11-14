package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMissingFileGivesDefaults(t *testing.T) {
	cfg, problems := Load(filepath.Join(t.TempDir(), "nope.conf"))
	if len(problems) != 0 {
		t.Errorf("problems: %v", problems)
	}
	if cfg != Default() {
		t.Error("expected the defaults")
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hexdm.conf")
	body := `
# a comment
background = Starfield
accent = "#f80"
base=#102030
bloom = 0
clock = 12h
secondary_displays = off

accent_typo = #fff
error = red
scanlines = lots
just some words
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, problems := Load(path)

	if cfg.Background != "starfield" {
		t.Errorf("background = %q", cfg.Background)
	}
	if cfg.Accent != (Color{1, 136.0 / 255, 0}) {
		t.Errorf("accent = %v", cfg.Accent)
	}
	if cfg.Base != (Color{16.0 / 255, 32.0 / 255, 48.0 / 255}) {
		t.Errorf("base = %v", cfg.Base)
	}
	if cfg.Bloom != 0 || cfg.Clock24 || cfg.SecondaryDisplays {
		t.Errorf("bloom %v clock24 %v secondary %v", cfg.Bloom, cfg.Clock24, cfg.SecondaryDisplays)
	}

	// The four bad lines are reported and leave the defaults alone.
	if len(problems) != 4 {
		t.Errorf("got %d problems, want 4: %v", len(problems), problems)
	}
	def := Default()
	if cfg.Error != def.Error || cfg.Scanlines != def.Scanlines {
		t.Error("a bad value replaced a default")
	}
}

func TestExampleConfigIsValid(t *testing.T) {
	cfg, problems := Load("../config/hexdm.conf")
	if len(problems) != 0 {
		t.Errorf("problems: %v", problems)
	}
	if cfg != Default() {
		t.Error("the shipped config should match the built-in defaults")
	}
}
