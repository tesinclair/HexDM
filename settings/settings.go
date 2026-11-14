// Package settings reads the greeter's look from a config file.
//
// The file is plain "key = value" lines. Blank lines and lines starting with
// '#' are ignored. Anything missing or unreadable keeps its default, so a
// broken config can never stop a login.
package settings

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const DefaultPath = "/etc/hexdm/hexdm.conf"

// Backgrounds the rendered greeter can draw. "random" picks one per start.
var Backgrounds = []string{"hexgrid", "starfield", "horizon", "aurora", "rain", "plain"}

// Color is red, green and blue, each from 0 to 1.
type Color [3]float32

type Config struct {
	Background string

	Base    Color // behind everything
	Primary Color // the background's main tint
	Accent  Color // focus, the ring, anything lit
	Text    Color
	Dim     Color // labels and hints
	Error   Color
	Success Color
	Warning Color

	Bloom     float32 // glow around bright things, 0 turns it off
	Scanlines float32 // 0 turns them off
	Vignette  float32 // darkening towards the corners, 0 turns it off

	Clock24 bool
	// Draw the background on every other connected display.
	SecondaryDisplays bool
}

func Default() Config {
	return Config{
		Background: "hexgrid",

		Base:    hex("#07050D"),
		Primary: hex("#6B2FB3"),
		Accent:  hex("#22E4FF"),
		Text:    hex("#ECE8F7"),
		Dim:     hex("#7D7799"),
		Error:   hex("#FF3B5C"),
		Success: hex("#3DFFA8"),
		Warning: hex("#FFB454"),

		Bloom:     1,
		Scanlines: 0.35,
		Vignette:  0.8,

		Clock24:           true,
		SecondaryDisplays: true,
	}
}

func hex(s string) Color {
	c, err := ParseColor(s)
	if err != nil {
		panic(err)
	}
	return c
}

// ParseColor reads "#RRGGBB" or "#RGB". The '#' is optional.
func ParseColor(s string) (Color, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return Color{}, fmt.Errorf("%q is not a colour like #22E4FF", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return Color{}, fmt.Errorf("%q is not a colour like #22E4FF", s)
	}
	return Color{
		float32(v>>16&0xFF) / 255,
		float32(v>>8&0xFF) / 255,
		float32(v&0xFF) / 255,
	}, nil
}

// Load reads the config at path on top of the defaults. A missing file is
// not a problem. Every line that could not be used is reported in problems,
// and its setting keeps the default.
func Load(path string) (cfg Config, problems []error) {
	cfg = Default()
	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			problems = append(problems, err)
		}
		return cfg, problems
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			problems = append(problems, fmt.Errorf("%s:%d: expected key = value", path, n))
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if err := cfg.set(key, value); err != nil {
			problems = append(problems, fmt.Errorf("%s:%d: %w", path, n, err))
		}
	}
	if err := scanner.Err(); err != nil {
		problems = append(problems, err)
	}
	return cfg, problems
}

func (c *Config) set(key, value string) error {
	colors := map[string]*Color{
		"base": &c.Base, "primary": &c.Primary, "accent": &c.Accent,
		"text": &c.Text, "dim": &c.Dim,
		"error": &c.Error, "success": &c.Success, "warning": &c.Warning,
	}
	amounts := map[string]*float32{
		"bloom": &c.Bloom, "scanlines": &c.Scanlines, "vignette": &c.Vignette,
	}

	if target, ok := colors[key]; ok {
		col, err := ParseColor(value)
		if err != nil {
			return err
		}
		*target = col
		return nil
	}
	if target, ok := amounts[key]; ok {
		v, err := strconv.ParseFloat(value, 32)
		if err != nil || v < 0 || v > 4 {
			return fmt.Errorf("%s: %q is not a number from 0 to 4", key, value)
		}
		*target = float32(v)
		return nil
	}

	switch key {
	case "background":
		value = strings.ToLower(value)
		if value != "random" && !contains(Backgrounds, value) {
			return fmt.Errorf("background: %q is not one of %s or random", value, strings.Join(Backgrounds, ", "))
		}
		c.Background = value
	case "clock":
		switch value {
		case "24", "24h":
			c.Clock24 = true
		case "12", "12h":
			c.Clock24 = false
		default:
			return fmt.Errorf("clock: %q is not 24h or 12h", value)
		}
	case "secondary_displays":
		switch strings.ToLower(value) {
		case "background", "on", "true", "yes":
			c.SecondaryDisplays = true
		case "off", "false", "no":
			c.SecondaryDisplays = false
		default:
			return fmt.Errorf("secondary_displays: %q is not background or off", value)
		}
	default:
		return fmt.Errorf("unknown setting %q", key)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
