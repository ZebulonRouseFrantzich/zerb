package state

import (
	"net/netip"
	"net/url"
	"strings"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// PC-020 syntax only: the source service authenticates source_id, object
// membership and public endpoint eligibility; this function performs no DNS.
func codecValidSource(v receipt.Source) bool {
	if !codecValidSourceURI(v.URI) || !codecValidDigest(v.SourceID) || len(v.Commit) != len(v.RootTree) || len(v.Commit) != 40 && len(v.Commit) != 64 {
		return false
	}
	for _, id := range []string{v.Commit, v.RootTree} {
		for i := range len(id) {
			c := id[i]
			if !codecLowerHex(c) {
				return false
			}
		}
	}
	return true
}

func codecValidSourceURI(raw string) bool {
	if len(raw) > 2048 || !strings.HasPrefix(raw, "https://") || strings.ContainsAny(raw, "\\?#\x00") {
		return false
	}
	for i := range len(raw) {
		if raw[i] < 0x21 || raw[i] >= 0x7f {
			return false
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Host == "" || u.Path == "" || u.Path[0] != '/' || u.String() != raw {
		return false
	}
	return codecValidSourceHost(u) && codecValidSourcePort(u) && codecValidSourcePath(u.EscapedPath())
}

func codecValidSourceHost(u *url.URL) bool {
	host := u.Hostname()
	if host == "" || strings.HasSuffix(host, ".") || strings.ContainsRune(host, ':') && !strings.HasPrefix(u.Host, "[") {
		return false
	}
	// net/url tolerates these raw host bytes, but URI reg-name syntax does not.
	if !strings.HasPrefix(u.Host, "[") && strings.ContainsAny(host, "[]<>\"") {
		return false
	}
	for i := range len(host) {
		if host[i] >= 'A' && host[i] <= 'Z' || host[i] < 0x21 || host[i] >= 0x7f {
			return false
		}
	}
	if address, err := netip.ParseAddr(host); err == nil && address.String() != host {
		return false
	}
	return true
}

func codecValidSourcePort(u *url.URL) bool {
	port := u.Port()
	if strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port != "" {
		if port == "443" || len(port) > 5 || len(port) > 1 && port[0] == '0' {
			return false
		}
		n := 0
		for i := range len(port) {
			if port[i] < '0' || port[i] > '9' {
				return false
			}
			n = n*10 + int(port[i]-'0')
		}
		if n == 0 || n > 65535 {
			return false
		}
	}
	return true
}

func codecValidSourcePath(repository string) bool {
	for segment := range strings.SplitSeq(repository, "/") {
		if segment == "." || segment == ".." {
			return false
		}
		for i := 0; i < len(segment); i++ {
			c := segment[i]
			if codecUnreserved(c) || strings.ContainsRune("!$&'()*+,;=:@", rune(c)) {
				continue
			}
			if c != '%' || i+2 >= len(segment) || !codecUpperHex(segment[i+1]) || !codecUpperHex(segment[i+2]) {
				return false
			}
			decoded := codecHexNibble(segment[i+1])<<4 | codecHexNibble(segment[i+2])
			if decoded < 0x20 || decoded == 0x7f || decoded == '/' || decoded == '\\' || codecUnreserved(decoded) {
				return false
			}
			i += 2
		}
	}
	return true
}
func codecUnreserved(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~", rune(c))
}
func codecLowerHex(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' }
func codecUpperHex(c byte) bool { return c >= '0' && c <= '9' || c >= 'A' && c <= 'F' }
func codecHexNibble(c byte) byte {
	if c <= '9' {
		return c - '0'
	}
	return c - 'A' + 10
}
