package desktop

import (
	"errors"
	"strings"
)

var ErrBadExec = errors.New("Exec line could not be split into a command")

// SplitExec turns the Exec value of a desktop file into an argument list,
// following the Desktop Entry Specification: arguments are separated by
// spaces, an argument may be wrapped in double quotes, and inside quotes a
// backslash escapes the next character.
//
// Field codes such as %f or %U have nothing to stand for when starting a
// session, so they are dropped. %% is a literal percent sign.
func SplitExec(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg, inQuote := false, false

	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case inQuote:
			switch c {
			case '"':
				inQuote = false
			case '\\':
				if i+1 >= len(rs) {
					return nil, ErrBadExec
				}
				i++
				cur.WriteRune(rs[i])
			default:
				cur.WriteRune(c)
			}
		case c == '"':
			inQuote, inArg = true, true
		case c == ' ' || c == '\t':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		case c == '%':
			if i+1 >= len(rs) {
				return nil, ErrBadExec
			}
			i++
			if rs[i] == '%' {
				cur.WriteRune('%')
				inArg = true
			}
		default:
			cur.WriteRune(c)
			inArg = true
		}
	}
	if inQuote {
		return nil, ErrBadExec
	}
	if inArg {
		args = append(args, cur.String())
	}
	if len(args) == 0 || args[0] == "" {
		return nil, ErrBadExec
	}

	return args, nil
}
