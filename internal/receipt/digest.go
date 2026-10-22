// Package receipt owns canonical durable value types under ARCH-030. Parsing a
// digest token checks its syntax, not artifact integrity, approval, or authority.
package receipt

import "errors"

type Digest string

var errDigest = errors.New("invalid digest")

// ParseDigest accepts exactly PC-010/050's sha256:<64-lowercase-hex> form.
// It rejects before examining the payload unless the fixed byte length matches.
func ParseDigest(value string) (Digest, error) {
	const prefix = "sha256:"
	if len(value) != len(prefix)+64 || value[:len(prefix)] != prefix {
		return "", errDigest
	}
	for index := len(prefix); index < len(value); index++ {
		digit := value[index]
		if digit < '0' || digit > '9' {
			if digit < 'a' || digit > 'f' {
				return "", errDigest
			}
		}
	}
	return Digest(value), nil
}
