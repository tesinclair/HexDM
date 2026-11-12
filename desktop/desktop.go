package desktop

import (
	"bufio"
	"os"
	"slices"
	"errors"
	"path/filepath"
)

var (
	ErrNoDesktop = errors.New("No desktop file given to parse")
	ErrParseFailed = errors.New("Parser failed to find name or exec")
)

// returns (Desktop name, Desktop exec) in that order
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

	r := bufio.NewReader(f)

	desktopEntry := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			break
		}

		tokens := tokenizeLine(line)
		if !desktopEntry {
			desktopEntry = checkDesktop(tokens)
		} else {
			if ret, n := isType(tokens, "Name"); ret {
				name = n
			}
			if ret, b := isType(tokens, "Exec"); ret {
				exec = b
			}
		}

		if name != "" && exec != "" {
			break
		}

	}
	if name == "" || exec == "" {
		return "", "", ErrParseFailed
	}

	return name, exec, nil

}

// returns a slice of found desktop files
func GetAll() []string {
	// Just doing systemwide for now
	wayland := filepath.Join("/usr", "share", "wayland-sessions")
	x := filepath.Join("/usr", "share", "xsessions")

	desktopDirs := []string{wayland, x}

	desktops := make([]string, 0)
	for _, desktopDir := range desktopDirs {
		desktopDirFiles, err := os.ReadDir(desktopDir)
		if err != nil{
			return desktops
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

// =-=-=-=- Private
func tokenizeLine(line string) []string {
	/* In this case we ignore after '#', and we tokenize thus:
	 *   [Desktop Entry] => '[', 'Desktop Entry', ']'
	 *   Name=Content = here # ignore this => 'Name', '=', 'Content = here'
	 */

	tokenStream := make([]string, 0)
	token := ""
	for idx, c := range line {
		switch c {
		case '#': // Comment
			if token != "" {
				tokenStream = append(tokenStream, token)
			}
			return tokenStream
		case '=': // only track first
			if !slices.Contains(tokenStream, "=") {
				if token != "" {
					tokenStream = append(tokenStream, token)
					token = ""
				}
				tokenStream = append(tokenStream, "=")
			} else {
				token += "="
			}
		case '[':
			if token == "" && len(tokenStream) == 0 {
				tokenStream = append(tokenStream, "[")
			} else {
				token += "["
			}
		case ']':
			if slices.Contains(tokenStream, "[") && !slices.Contains(tokenStream, "]") {
				if token != "" {
					tokenStream = append(tokenStream, token)
					token = ""
				}
				tokenStream = append(tokenStream, "]")
			} else {
				token += "]"
			}
		case '\n':
			if token != ""{
				tokenStream = append(tokenStream, token)
				token = ""
			}
		case ' ':
			if idx == len(line) - 1 || idx == 0{
				continue
			}
			if line[idx + 1] != '\n'{
				token += " "
			}
		case '\t', '\r':
			continue
		default:
			token += string(c)
		}
	}
	if token != "" {
		tokenStream = append(tokenStream, token)
	}
	return tokenStream
}

func isType(tokens []string, typ string) (bool, string) {
	if len(tokens) < 1 {
		return false, ""
	}

	if tokens[0] == typ {
		equal_idx := slices.Index(tokens, "=")
		if equal_idx == -1 || len(tokens) <= equal_idx+1 {
			return false, ""
		}

		return true, tokens[equal_idx+1]
	}

	return false, ""
}

func checkDesktop(tokens []string) bool {
	if len(tokens) != 3 {
		return false
	}

	if tokens[1] == "Desktop Entry" {
		return true
	}

	return false
}
