package filetree

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"strings"
	"time"
)

const defaultSFTPTimeout = 12 * time.Second

type commandRunner func(context.Context, string, string, []string, string) ([]byte, []byte, error)

type SFTPSource struct {
	Executable string
	Timeout    time.Duration
	Runner     commandRunner
}

func (s SFTPSource) ReadDir(ctx context.Context, location Location) (Listing, error) {
	var target SSHTarget
	remotePath := ""
	pathKnown := false
	switch loc := location.(type) {
	case SSHLocation:
		target, remotePath, pathKnown = loc.Target, loc.Path, true
	case SSHLocationUnknown:
		target = loc.Target
	default:
		return Listing{}, fmt.Errorf("SFTP source cannot read %T", location)
	}
	if target.Destination == "" {
		return Listing{}, fmt.Errorf("SFTP destination is missing")
	}
	if pathKnown && !path.IsAbs(remotePath) {
		return Listing{}, fmt.Errorf("remote directory %q is not an absolute POSIX path", remotePath)
	}

	input, err := sftpBatch(remotePath, pathKnown)
	if err != nil {
		return Listing{}, err
	}
	args := append([]string(nil), target.Options...)
	args = append(args, "-q", "-b", "-")
	if target.DestinationLiteral {
		args = append(args, "--")
	}
	args = append(args, target.Destination)
	executable := s.Executable
	if executable == "" {
		executable = "sftp"
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultSFTPTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	runner := s.Runner
	if runner == nil {
		runner = runCommand
	}
	stdout, stderr, err := runner(commandCtx, executable, target.WorkingDir, args, input)
	if err != nil {
		message := strings.TrimSpace(string(stderr))
		if message != "" {
			return Listing{}, fmt.Errorf("SFTP %s: %s: %w", target.Destination, message, err)
		}
		return Listing{}, fmt.Errorf("SFTP %s: %w", target.Destination, err)
	}
	resolvedPath, entries, err := parseSFTPOutput(stdout)
	if err != nil {
		return Listing{}, fmt.Errorf("parse SFTP listing for %s: %w", target.Destination, err)
	}
	resolved := SSHLocation{Target: target, Path: resolvedPath}
	return Listing{Location: resolved, Entries: entries}, nil
}

func sftpBatch(remotePath string, pathKnown bool) (string, error) {
	var b strings.Builder
	if pathKnown {
		quoted, err := quoteSFTPPath(remotePath)
		if err != nil {
			return "", err
		}
		b.WriteString("@cd ")
		b.WriteString(quoted)
		b.WriteByte('\n')
	}
	b.WriteString("@pwd\n@ls -la\n")
	return b.String(), nil
}

func quoteSFTPPath(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("remote path contains a control character")
	}
	var b strings.Builder
	b.Grow(len(value) + 2)
	b.WriteByte('"')
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("remote path contains a control character")
		}
		if r == '\\' || r == '"' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String(), nil
}

func parseSFTPOutput(output []byte) (string, []Entry, error) {
	remoteDir := ""
	entries := make([]Entry, 0)
	const pwdPrefix = "Remote working directory: "
	for _, raw := range strings.Split(strings.ReplaceAll(string(output), "\r\n", "\n"), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}
		if strings.HasPrefix(line, pwdPrefix) {
			if remoteDir != "" {
				return "", nil, fmt.Errorf("SFTP returned more than one working directory")
			}
			remoteDir = strings.TrimSpace(strings.TrimPrefix(line, pwdPrefix))
			continue
		}
		entry, skip, err := parseSFTPEntry(line)
		if err != nil {
			return "", nil, err
		}
		if !skip {
			entries = append(entries, entry)
		}
	}
	if remoteDir == "" || !path.IsAbs(remoteDir) {
		return "", nil, fmt.Errorf("SFTP did not report an absolute working directory")
	}
	sortEntries(entries)
	return remoteDir, entries, nil
}

func parseSFTPEntry(line string) (Entry, bool, error) {
	fields := strings.Fields(line)
	if len(fields) < 9 {
		return Entry{}, false, fmt.Errorf("malformed SFTP listing line %q", line)
	}
	mode := fields[0]
	if len(mode) < 10 {
		return Entry{}, false, fmt.Errorf("malformed SFTP permissions in line %q", line)
	}
	name := lineAfterFields(line, 8)
	if len(name) == 0 || (name[0] != ' ' && name[0] != '\t') {
		return Entry{}, false, fmt.Errorf("SFTP listing has no name separator in line %q", line)
	}
	// SFTP's long listing uses one delimiter before the filename. Preserve
	// any additional spaces because they can be part of the filename.
	name = name[1:]
	if name == "" {
		return Entry{}, false, fmt.Errorf("SFTP listing has an empty name in line %q", line)
	}
	if name == "." || name == ".." {
		return Entry{}, true, nil
	}
	kind := EntryOther
	switch mode[0] {
	case 'd':
		kind = EntryDirectory
	case 'l':
		kind = EntrySymlink
	case '-':
		kind = EntryFile
	case 'b', 'c', 'p', 's':
	default:
		return Entry{}, false, fmt.Errorf("unknown SFTP entry type %q in line %q", mode[0], line)
	}
	return Entry{Name: name, Kind: kind}, false, nil
}

// lineAfterFields returns the unparsed suffix after the first n whitespace-
// separated fields, preserving spaces inside the filename.
func lineAfterFields(line string, n int) string {
	i := 0
	for field := 0; field < n; field++ {
		for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		for i < len(line) && line[i] != ' ' && line[i] != '\t' {
			i++
		}
	}
	return line[i:]
}

func runCommand(ctx context.Context, executable, directory string, args []string, input string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	if directory != "" {
		cmd.Dir = directory
	}
	cmd.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	return stdout, stderr.Bytes(), err
}
