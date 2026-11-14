package filetree

import (
	"strings"
	"testing"
)

func TestQuoteSFTPPath(t *testing.T) {
	tests := []struct {
		value string
		want  string
		fails bool
	}{
		{value: "docs", want: `"docs"`},
		{value: "/home/user/my files", want: `"/home/user/my files"`},
		{value: `say "hi"`, want: `"say \"hi\""`},
		{value: `back\slash`, want: `"back\\slash"`},
		{value: `both \" quoted`, want: `"both \\\" quoted"`},
		{value: "", want: `""`},
		{value: "日本語", want: `"日本語"`},
		{value: "new\nline", fails: true},
		{value: "carriage\rreturn", fails: true},
		{value: "nul\x00byte", fails: true},
		{value: "tab\tinside", fails: true},
		{value: "delete\x7fbyte", fails: true},
	}
	for _, test := range tests {
		got, err := quoteSFTPPath(test.value)
		if test.fails {
			if err == nil {
				t.Errorf("quoteSFTPPath(%q) = %q, want an error", test.value, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("quoteSFTPPath(%q): %v", test.value, err)
		} else if got != test.want {
			t.Errorf("quoteSFTPPath(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestParseSFTPOutput(t *testing.T) {
	listing := strings.Join([]string{
		"Remote working directory: /home/sam",
		"total 24",
		"drwxr-xr-x 7 sam sam 4096 Nov 13 10:00 .",
		"drwxr-xr-x 3 root root 4096 Nov 1 09:00 ..",
		"-rw-r--r-- 1 sam sam 1234 Nov 13 09:59 notes.txt",
		"drwxr-xr-x 2 sam sam 4096 Nov 12 18:30 my dir",
		"lrwxrwxrwx 1 sam sam 5 Nov 10 08:00 latest",
		"",
	}, "\n")
	dir, entries, err := parseSFTPOutput([]byte(listing))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if dir != "/home/sam" {
		t.Errorf("the working directory is %q", dir)
	}
	// The dot entries are skipped; directories come first, and what is
	// left of them sorts by name, ignoring its case. A symlink's name is
	// its own: sftp's long listing does not name the target after it.
	want := []Entry{
		{Name: "my dir", Kind: EntryDirectory},
		{Name: "latest", Kind: EntrySymlink},
		{Name: "notes.txt", Kind: EntryFile},
	}
	if len(entries) != len(want) {
		t.Fatalf("the listing has %v, want %v", entries, want)
	}
	for i, e := range entries {
		if e != want[i] {
			t.Errorf("entry %d is %v, want %v", i, e, want[i])
		}
	}
}

func TestParseSFTPEntryNamesWithSpaces(t *testing.T) {
	line := "drwxr-xr-x 2 sam sam 4096 Nov 12 18:30 a  name  with   spaces"
	e, skip, err := parseSFTPEntry(line)
	if err != nil || skip {
		t.Fatalf("parse %q: %v, skip %v", line, err, skip)
	}
	if e.Name != "a  name  with   spaces" {
		t.Errorf("the name is %q", e.Name)
	}
	if _, _, err := parseSFTPEntry("too few fields here"); err == nil {
		t.Error("a line with no room for a name parsed")
	}
}

func TestParseSFTPOutputRejectsRelativeDirectory(t *testing.T) {
	listing := "Remote working directory: relative/path\n"
	if _, _, err := parseSFTPOutput([]byte(listing)); err == nil {
		t.Error("a relative working directory parsed")
	}
}

func TestParseSSHTarget(t *testing.T) {
	target, err := ParseSSHTarget([]string{"host"})
	if err != nil || target.Destination != "host" || target.DestinationLiteral {
		t.Errorf("ParseSSHTarget(host) = %+v, %v", target, err)
	}
	target, err = ParseSSHTarget([]string{"-p", "2222", "sam@host"})
	if err != nil || target.Destination != "sam@host" {
		t.Errorf("ParseSSHTarget(-p 2222 sam@host) = %+v, %v", target, err)
	}
	if !contains(target.Options, "-P", "2222") {
		t.Errorf("the port option is missing from %v", target.Options)
	}
	// A destination that would pass for an option is marked literal, so
	// sftp takes it for a host; and a literal destination is still one
	// word: spaces never parse.
	target, err = ParseSSHTarget([]string{"--", "-weird"})
	if err != nil || target.Destination != "-weird" || !target.DestinationLiteral {
		t.Errorf("ParseSSHTarget(-- -weird) = %+v, %v", target, err)
	}
	if _, err := ParseSSHTarget([]string{"--", "host name"}); err == nil {
		t.Error("a destination with a space parsed")
	}
	if _, err := ParseSSHTarget(nil); err == nil {
		t.Error("an empty target parsed")
	}
	if _, err := ParseSSHTarget([]string{"-X", "host"}); err == nil {
		t.Error("an unsupported option parsed")
	}
	if _, err := ParseSSHTarget([]string{"-p", "0", "host"}); err == nil {
		t.Error("an invalid port parsed")
	}
}

// contains tells whether values holds every one of the wants.
func contains(values []string, wants ...string) bool {
	for _, want := range wants {
		found := false
		for _, v := range values {
			if v == want {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestDestinationIdentity(t *testing.T) {
	tests := []struct{ in, host, user string }{
		{"host", "host", ""},
		{"sam@host", "host", "sam"},
		{"sam@[2001:db8::1]", "[2001:db8::1]", "sam"},
	}
	for _, test := range tests {
		host, user := DestinationIdentity(test.in)
		if host != test.host || user != test.user {
			t.Errorf("DestinationIdentity(%q) = %q, %q; want %q, %q", test.in, host, user, test.host, test.user)
		}
	}
}

func TestWindowsFileURLPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{in: "/C:/Users/sam", want: `C:\Users\sam`},
		{in: "C:/Users/sam", want: `C:\Users\sam`},
		{in: "/home/sam", want: "/home/sam"},
		{in: "//server/share", want: "//server/share"},
		{in: "relative/path", want: "relative/path"},
	}
	for _, test := range tests {
		if got := windowsFileURLPath(test.in); got != test.want {
			t.Errorf("windowsFileURLPath(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}
