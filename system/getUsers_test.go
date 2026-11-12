package system

import (
	"slices"
	"testing"
)

const passwd = `root:x:0:0::/root:/usr/bin/bash
# a comment
bin:x:1:1::/:/usr/bin/nologin
nobody:x:65534:65534:Kernel Overflow User:/:/usr/bin/nologin
ada:x:1000:1000:Ada Lovelace,Room 1:/home/ada:/usr/bin/zsh
bob:x:1001:1001::/home/bob:/usr/bin/bash
broken:line
`

func TestParsePasswd(t *testing.T) {
	var names []string
	for _, fields := range parsePasswd(passwd) {
		names = append(names, fields[0])
	}
	// The comment and the short line are dropped.
	if want := []string{"root", "bin", "nobody", "ada", "bob"}; !slices.Equal(names, want) {
		t.Errorf("got %q, want %q", names, want)
	}
}

// ada's comment field has spaces in it, which used to split her entry in two
// because the file was split on whitespace instead of on lines.
func TestHumanUsers(t *testing.T) {
	got := humanUsers(parsePasswd(passwd))
	if want := []string{"ada", "bob"}; !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}
