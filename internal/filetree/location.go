package filetree

import (
	"fmt"
	"net/netip"
	"net/url"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"zeus/internal/zeus"
)

// Location is a local path or an SSH path. Its implementations are sealed
// within this package so local and remote paths cannot be confused.
type Location interface{ isLocation() }

type LocalLocation struct{ Path string }

func (LocalLocation) isLocation() {}

type SSHLocation struct {
	Target SSHTarget
	Path   string
}

func (SSHLocation) isLocation() {}

type SSHLocationUnknown struct{ Target SSHTarget }

func (SSHLocationUnknown) isLocation() {}

// SSHTarget describes a direct OpenSSH destination and the options needed
// to reconnect from the same local directory as the running ssh process.
type SSHTarget struct {
	Destination        string
	DestinationLiteral bool
	Options            []string
	WorkingDir         string
}

// ResolveLocation turns session metadata into a typed filesystem location.
// A remote shell's OSC 7 path is authoritative; the ssh process's local
// working directory is never used as a remote path.
func ResolveLocation(info zeus.SessionInfo) (Location, error) {
	if strings.EqualFold(info.Program, "ssh") {
		target, err := ParseSSHTarget(info.Args)
		if err != nil {
			return nil, err
		}
		target.WorkingDir = info.Dir
		if !path.IsAbs(info.ShellDir) {
			return SSHLocationUnknown{Target: target}, nil
		}
		return SSHLocation{Target: target, Path: info.ShellDir}, nil
	}
	if info.ShellDir != "" {
		localPath := info.ShellDir
		if runtime.GOOS == "windows" {
			localPath = windowsFileURLPath(localPath)
		}
		if filepath.IsAbs(localPath) && (runtime.GOOS != "windows" || filepath.VolumeName(localPath) != "") {
			return LocalLocation{Path: filepath.Clean(localPath)}, nil
		}
	}
	if info.Dir == "" {
		return nil, fmt.Errorf("local working directory is unavailable")
	}
	return LocalLocation{Path: info.Dir}, nil
}

func windowsFileURLPath(value string) string {
	if len(value) >= 3 && value[0] == '/' && isDriveLetter(value[1]) && value[2] == ':' {
		value = value[1:]
	}
	if len(value) >= 3 && isDriveLetter(value[0]) && value[1] == ':' && value[2] == '/' {
		return strings.ReplaceAll(value, "/", `\`)
	}
	return value
}

func isDriveLetter(r byte) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

// ParseSSHTarget parses the connection options of an ssh argv and converts
// them to options accepted by sftp. A command after the destination is
// rejected because it is not an interactive remote shell location.
func ParseSSHTarget(args []string) (SSHTarget, error) {
	var target SSHTarget
	if len(args) == 0 {
		return target, fmt.Errorf("ssh destination is missing")
	}

	i := 0
	destinationLiteral := false
	if isSSHExecutable(args[0]) {
		i++
	}
	login := ""
	userOptionSet, portOptionSet, portFlagSet := false, false, false
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			destinationLiteral = true
			i++
			if i >= len(args) {
				return target, fmt.Errorf("ssh destination is missing after --")
			}
			target.Destination = args[i]
			i++
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			target.Destination = arg
			i++
			break
		}
		if len(arg) < 2 {
			return target, fmt.Errorf("invalid ssh option %q", arg)
		}

		flags := arg[1:]
		for len(flags) > 0 {
			flag := flags[0]
			flags = flags[1:]
			switch flag {
			case '4', '6', 'C', 'q', 'v':
				target.Options = append(target.Options, "-"+string(flag))
			case 'F', 'i', 'J', 'o', 'p', 'l':
				value := flags
				flags = ""
				if value == "" {
					i++
					if i >= len(args) {
						return target, fmt.Errorf("ssh option -%c requires a value", flag)
					}
					value = args[i]
				}
				if value == "" || strings.HasPrefix(value, "=") {
					return target, fmt.Errorf("ssh option -%c has an invalid value", flag)
				}
				switch flag {
				case 'p':
					if portFlagSet || portOptionSet {
						return target, fmt.Errorf("ssh port was specified more than once")
					}
					port, err := strconv.Atoi(value)
					if err != nil || port < 1 || port > 65535 {
						return target, fmt.Errorf("ssh port %q is invalid", value)
					}
					portFlagSet = true
					target.Options = append(target.Options, "-P", value)
				case 'l':
					if userOptionSet {
						return target, fmt.Errorf("ssh login name was specified more than once")
					}
					if strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
						return target, fmt.Errorf("ssh login name %q is invalid", value)
					}
					if login != "" {
						return target, fmt.Errorf("ssh login name was specified more than once")
					}
					login = value
					target.Options = append(target.Options, "-o", "User="+value)
				case 'o':
					key, _, hasValue := strings.Cut(value, "=")
					switch {
					case hasValue && strings.EqualFold(key, "user"):
						if login != "" || userOptionSet {
							return target, fmt.Errorf("ssh login name was specified more than once")
						}
						userOptionSet = true
					case hasValue && strings.EqualFold(key, "port"):
						if portFlagSet || portOptionSet {
							return target, fmt.Errorf("ssh port was specified more than once")
						}
						portOptionSet = true
					}
					target.Options = append(target.Options, "-o", value)
				default:
					target.Options = append(target.Options, "-"+string(flag), value)
				}
			default:
				return target, fmt.Errorf("ssh option -%c is unsupported for file browsing", flag)
			}
		}
		i++
	}

	if target.Destination == "" {
		return target, fmt.Errorf("ssh destination is missing")
	}
	if strings.ContainsAny(target.Destination, " \t\r\n\x00") {
		return target, fmt.Errorf("ssh destination %q is invalid", target.Destination)
	}
	var destinationHasLogin bool
	var uriPort string
	normalizedDestination, destinationHasLogin, uriPort, err := normalizeSSHDestination(target.Destination)
	if err != nil {
		return target, err
	}
	target.Destination = normalizedDestination
	if destinationHasLogin && (login != "" || userOptionSet) {
		return target, fmt.Errorf("ssh destination and login options both specify a login name")
	}
	if uriPort != "" {
		if portFlagSet || portOptionSet {
			return target, fmt.Errorf("ssh destination URI and another option both specify a port")
		}
		target.Options = append(target.Options, "-P", uriPort)
	}
	target.DestinationLiteral = destinationLiteral && strings.HasPrefix(target.Destination, "-")
	if i < len(args) {
		return target, fmt.Errorf("remote commands are unsupported for file browsing")
	}
	return target, nil
}

// normalizeSSHDestination converts ssh's destination grammar to the host
// operand accepted by sftp. In particular, sftp needs URI ports split out
// into -P and IPv6 literals bracketed to avoid its host/path delimiter.
func normalizeSSHDestination(destination string) (host string, hasUser bool, port string, err error) {
	if strings.Contains(destination, "://") {
		u, parseErr := url.Parse(destination)
		if parseErr != nil || !strings.EqualFold(u.Scheme, "ssh") || u.Opaque != "" || u.Host == "" ||
			u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return "", false, "", fmt.Errorf("SSH destination URI %q is invalid", destination)
		}
		if u.User != nil {
			if _, hasPassword := u.User.Password(); hasPassword {
				return "", false, "", fmt.Errorf("SSH destination URI must not contain a password")
			}
			user := u.User.Username()
			if user == "" || strings.ContainsAny(user, "@:\x00\r\n\t ") {
				return "", false, "", fmt.Errorf("SSH destination URI username is invalid")
			}
			hasUser = true
			host = user + "@"
		}
		uriHost := u.Hostname()
		if uriHost == "" || strings.ContainsAny(uriHost, "\x00\r\n\t ") {
			return "", false, "", fmt.Errorf("SSH destination URI host is invalid")
		}
		if strings.Contains(uriHost, ":") {
			ip, parseErr := netip.ParseAddr(uriHost)
			if parseErr != nil || !ip.Is6() {
				return "", false, "", fmt.Errorf("SSH destination URI IPv6 host %q is invalid", uriHost)
			}
			uriHost = "[" + ip.String() + "]"
		}
		port = u.Port()
		if port != "" {
			n, convErr := strconv.Atoi(port)
			if convErr != nil || n < 1 || n > 65535 {
				return "", false, "", fmt.Errorf("SSH destination URI port %q is invalid", port)
			}
		}
		return host + uriHost, hasUser, port, nil
	}
	user, hostname := "", destination
	if at := strings.LastIndexByte(destination, '@'); at >= 0 {
		if at == 0 || strings.Contains(destination[:at], "@") {
			return "", false, "", fmt.Errorf("ssh destination %q has an invalid login name", destination)
		}
		user, hostname = destination[:at], destination[at+1:]
		if strings.ContainsAny(user, " \t\r\n\x00") {
			return "", false, "", fmt.Errorf("ssh destination %q has an invalid login name", destination)
		}
		hasUser = true
	}
	if hostname == "" {
		return "", false, "", fmt.Errorf("ssh destination host is missing")
	}
	if strings.HasPrefix(hostname, "[") && strings.HasSuffix(hostname, "]") {
		ip, parseErr := netip.ParseAddr(hostname[1 : len(hostname)-1])
		if parseErr != nil || !ip.Is6() {
			return "", false, "", fmt.Errorf("ssh destination IPv6 host %q is invalid", hostname)
		}
		hostname = "[" + ip.String() + "]"
	} else if strings.Contains(hostname, ":") {
		ip, parseErr := netip.ParseAddr(hostname)
		if parseErr != nil || !ip.Is6() {
			return "", false, "", fmt.Errorf("ssh destination %q contains an ambiguous colon; use an SSH URI for a port", destination)
		}
		hostname = "[" + ip.String() + "]"
	}
	if hasUser {
		return user + "@" + hostname, true, "", nil
	}
	return hostname, false, "", nil
}

func isSSHExecutable(arg string) bool {
	base := strings.ToLower(filepath.Base(arg))
	base = strings.TrimSuffix(base, ".exe")
	return base == "ssh"
}
