package desktop

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrNoDesktop = errors.New("No desktop file given to parse")
	ErrParseFailed = errors.New("Parser failed to find name or exec")
)

// returns (Desktop name, Desktop exec) in that order
//
// Only the [Desktop Entry] group is read. Per the Desktop Entry
// Specification a comment is a whole line starting with '#', spaces around
// the '=' are ignored, and the first occurrence of a key wins.
func Parse(dName string) (string, string, error) {
	if dName == "" {
		return "", "", ErrNoDesktop
	}

	var name string
	var exec string

	f, err := os.Open(dName)
	if err != nil {
		return "", "", err // probably shouldn't just send that error up the chain for long
	}
	defer f.Close()

	desktopEntry := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if desktopEntry {
				break // end of the [Desktop Entry] group
			}
			desktopEntry = line == "[Desktop Entry]"
			continue
		}
		if !desktopEntry {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = unescape(strings.TrimSpace(value))

		switch key {
		case "Name":
			if name == "" {
				name = value
			}
		case "Exec":
			if exec == "" {
				exec = value
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}
	if name == "" || exec == "" {
		return "", "", ErrParseFailed
	}

	return name, exec, nil

}

// unescape resolves the escape sequences the specification allows in a
// string value: \s, \n, \t, \r and \\.
func unescape(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 >= len(value) {
			b.WriteByte(value[i])
			continue
		}
		i++
		switch value[i] {
		case 's':
			b.WriteByte(' ')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(value[i])
		}
	}
	return b.String()
}

// returns a slice of found desktop files
func GetAll() []string {
	// Just doing systemwide for now
	wayland := filepath.Join("/usr", "share", "wayland-sessions")

	// Wayland only. HexDM does not start an X server, so an entry from
	// /usr/share/xsessions could be picked but would never come up.
	desktopDirs := []string{wayland}

	desktops := make([]string, 0)
	for _, desktopDir := range desktopDirs {
		desktopDirFiles, err := os.ReadDir(desktopDir)
		if err != nil{
			continue // this directory may not exist; still look in the others
		}
		for _, d := range desktopDirFiles {
			if len(d.Name()) < 8{
				continue
			}
			if d.Name()[len(d.Name()) - 8:] == ".desktop" {
				desktops = append(desktops, filepath.Join(desktopDir, d.Name()))
			}
		}
	}

	return desktops
}
