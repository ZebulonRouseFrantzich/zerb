// Package sourceurl defines the one byte-level source locator grammar used by
// every Phase 1 proof slice. It is disposable proof code, not a product API.
package sourceurl

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// ValidatePublicHTTPS accepts only an already-canonical, credential-free,
// absolute public HTTPS URI with a DNS host and a non-root repository path.
func ValidatePublicHTTPS(raw string) error {
	return validate(raw, "https", true)
}

// Resolver is the exact DNS capability used during public-source admission.
// Keeping it narrow permits deterministic private-answer and rebinding proofs.
type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

var nonPublicSpecialUse = [...]netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
}

// IsPublicAddr is the shared endpoint predicate for admission and traced
// connects. It rejects non-unicast, local/private, and IANA special-use space.
func IsPublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsPrivate() || !addr.IsGlobalUnicast() {
		return false
	}
	for _, special := range nonPublicSpecialUse {
		if special.Contains(addr) {
			return false
		}
	}
	return true
}

// PinnedPublicHTTPS validates raw before resolving its exact canonical host,
// rejects the whole answer set if any answer is non-public, and returns one
// permanent CURLOPT_RESOLVE entry per address. libcurl therefore keeps the URI
// hostname for TLS/SNI verification while never consulting DNS for the
// admitted HOST:PORT during the operation.
func PinnedPublicHTTPS(ctx context.Context, raw string, resolver Resolver) ([]string, []netip.Addr, error) {
	if err := ValidatePublicHTTPS(raw); err != nil {
		return nil, nil, err
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, nil, errors.New("validated locator could not be parsed")
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "443"
	}
	answers, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(answers) == 0 {
		return nil, nil, errors.New("canonical source DNS resolution failed")
	}
	seen := make(map[netip.Addr]bool, len(answers))
	public := make([]netip.Addr, 0, len(answers))
	for _, answer := range answers {
		answer = answer.Unmap()
		if !IsPublicAddr(answer) {
			return nil, nil, errors.New("canonical source DNS returned a non-public answer")
		}
		if !seen[answer] {
			seen[answer] = true
			public = append(public, answer)
		}
	}
	sort.Slice(public, func(i, j int) bool { return public[i].Less(public[j]) })
	entries := make([]string, len(public))
	for i, answer := range public {
		address := answer.String()
		if answer.Is6() {
			address = "[" + address + "]"
		}
		entries[i] = host + ":" + port + ":" + address
	}
	return entries, public, nil
}

// ValidateUnselectedGitlink accepts the closed static locator grammar for an
// unselected gitlink. Selection remains forbidden independently of this parse.
func ValidateUnselectedGitlink(raw string) (string, error) {
	switch {
	case strings.HasPrefix(raw, "https://"):
		if err := validate(raw, "https", true); err != nil {
			return "", err
		}
		return "https", nil
	case strings.HasPrefix(raw, "ssh://"):
		if err := validate(raw, "ssh", false); err != nil {
			return "", err
		}
		return "ssh", nil
	default:
		return "", errors.New("locator is not canonical absolute HTTPS or SSH")
	}
}

func validate(raw, scheme string, allowPort bool) error {
	if raw == "" || strings.ContainsAny(raw, "\\\x00\r\n\t") {
		return errors.New("locator contains an empty or forbidden byte")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] >= 0x7f {
			return errors.New("locator must contain printable ASCII only")
		}
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme != scheme || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("locator is not an absolute credential-free URI")
	}
	if u.Host == "" || u.Path == "" || u.Path == "/" || u.Path[0] != '/' {
		return errors.New("locator requires a host and non-root absolute path")
	}
	if strings.Contains(u.Host, "@") || strings.HasSuffix(u.Hostname(), ".") || u.Hostname() != strings.ToLower(u.Hostname()) {
		return errors.New("locator authority is not canonical")
	}
	if err := validatePort(u.Port(), allowPort); err != nil {
		return err
	}
	if err := validateDNSHost(u.Hostname()); err != nil {
		return err
	}
	if err := validatePath(u.EscapedPath()); err != nil {
		return err
	}
	if u.String() != raw {
		return errors.New("locator requires canonical rewriting")
	}
	return nil
}

func validatePort(port string, allow bool) error {
	if port == "" {
		return nil
	}
	if !allow || port == "443" || len(port) > 1 && port[0] == '0' {
		return errors.New("locator port is not canonical")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("locator port is invalid")
	}
	return nil
}

func validateDNSHost(host string) error {
	if host == "" || len(host) > 253 || net.ParseIP(host) != nil || !strings.ContainsRune(host, '.') {
		return errors.New("locator requires a public canonical DNS host")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("locator host is not canonical DNS ASCII")
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return errors.New("locator host is not canonical DNS ASCII")
			}
		}
	}
	return nil
}

func validatePath(escaped string) error {
	if escaped == "" || escaped[0] != '/' {
		return errors.New("locator path is not absolute")
	}
	for _, segment := range strings.Split(escaped[1:], "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("locator path contains an empty or dot segment")
		}
		for i := 0; i < len(segment); i++ {
			c := segment[i]
			if isPChar(c) {
				continue
			}
			if c != '%' || i+2 >= len(segment) || !isUpperHex(segment[i+1]) || !isUpperHex(segment[i+2]) {
				return errors.New("locator path contains a non-canonical byte")
			}
			decoded := fromHex(segment[i+1])<<4 | fromHex(segment[i+2])
			if decoded < 0x20 || decoded == 0x7f || decoded == '/' || decoded == '\\' || isUnreserved(decoded) {
				return errors.New("locator path contains an encoded control, separator, or non-canonical escape")
			}
			i += 2
		}
	}
	return nil
}

func isAlphaNum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

func isUpperHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'F'
}

func fromHex(c byte) byte {
	if c <= '9' {
		return c - '0'
	}
	return c - 'A' + 10
}

func isUnreserved(c byte) bool {
	return isAlphaNum(c) || strings.ContainsRune("-._~", rune(c))
}

func isPChar(c byte) bool {
	return isUnreserved(c) || strings.ContainsRune("!$&'()*+,;=:@", rune(c))
}
