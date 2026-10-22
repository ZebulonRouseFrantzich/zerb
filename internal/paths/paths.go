// Package paths owns immutable input and lexical Linux XDG roots (ARCH-016/030).
// Filesystem identity and role validation belong to safefs.
package paths

import (
	"errors"
	"path"
	"slices"
	"strings"
)

var (
	errInput = errors.New("invalid path input")
	errRoots = errors.New("invalid protected roots")
)

// Input captures bytes without resolving roots or consulting the environment.
type Input struct {
	home    string
	environ []string
	valid   bool
}

// NewInput validates Linux environment encoding and retains an independent copy.
// Root semantics are deferred to Resolve for argv/version precedence.
func NewInput(home string, environ []string) (Input, error) {
	if strings.IndexByte(home, 0) >= 0 {
		return Input{}, errInput
	}
	for _, entry := range environ {
		name, _, found := strings.Cut(entry, "=")
		if !found || name == "" || strings.IndexByte(entry, 0) >= 0 {
			return Input{}, errInput
		}
	}
	return Input{home: home, environ: slices.Clone(environ), valid: true}, nil
}

func (i Input) Home() string      { return i.home }
func (i Input) Environ() []string { return slices.Clone(i.environ) }

// LookupEnv returns the first captured match, preserving ordinary raw duplicates.
func (i Input) LookupEnv(name string) (string, bool) {
	for _, entry := range i.environ {
		key, value, _ := strings.Cut(entry, "=")
		if key == name {
			return value, true
		}
	}
	return "", false
}

// Roots are lexical paths, not evidence of filesystem safety or ownership.
type Roots struct {
	home, data, cache, state string
}

// Resolve uses only the captured input and never reads or creates a path.
func Resolve(input Input) (Roots, error) {
	if !input.valid || !path.IsAbs(input.home) {
		return Roots{}, errRoots
	}
	home := path.Clean(input.home)
	names := [...]string{"XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"}
	bases := [...]string{path.Join(home, ".local/share"), path.Join(home, ".cache"), path.Join(home, ".local/state")}
	for i, name := range names {
		seen := false
		for _, entry := range input.environ {
			key, value, _ := strings.Cut(entry, "=")
			if key != name {
				continue
			}
			if seen {
				return Roots{}, errRoots
			}
			seen = true
			// XDG 0.8 ignores relative values; empty/unset values use defaults.
			if path.IsAbs(value) {
				bases[i] = path.Clean(value)
			}
		}
		bases[i] = path.Join(bases[i], "zerb")
	}
	for i := range bases {
		for j := i + 1; j < len(bases); j++ {
			if bases[i] == bases[j] || strings.HasPrefix(bases[i], bases[j]+"/") || strings.HasPrefix(bases[j], bases[i]+"/") {
				return Roots{}, errRoots
			}
		}
	}
	return Roots{home: home, data: bases[0], cache: bases[1], state: bases[2]}, nil
}

func (r Roots) Home() string  { return r.home }
func (r Roots) Data() string  { return r.data }
func (r Roots) Cache() string { return r.cache }
func (r Roots) State() string { return r.state }
