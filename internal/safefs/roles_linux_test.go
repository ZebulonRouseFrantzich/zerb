//go:build linux

package safefs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/ZebulonRouseFrantzich/zerb/internal/paths"
)

// These matrix tests are lexical units, not mount/namespace qualification.
func TestRoleLayoutMatrix(t *testing.T) {
	data := roleEntry{role: RoleData, target: "/data"}
	cache := roleEntry{role: RoleCache, target: "/cache"}
	stage := roleEntry{role: RoleStaging, target: "/cache/instances/0123456789abcdef0123456789abcdef/downloads/work"}
	for _, test := range []struct {
		name            string
		parent, child   roleEntry
		registeredStage bool
		valid           bool
	}{
		{"source-branch", data, roleEntry{role: RoleSource, target: "/data/sources"}, false, true},
		{"source-descendant", data, roleEntry{role: RoleSource, target: "/data/sources/a"}, false, true},
		{"tree-branch", data, roleEntry{role: RoleTree, target: "/data/trees/a"}, false, true},
		{"toolchain-branch", data, roleEntry{role: RoleToolchain, target: "/data/toolchain/a"}, false, true},
		{"manager-branch", data, roleEntry{role: RoleManager, target: "/data/installs/a"}, false, true},
		{"wrong-branch", data, roleEntry{role: RoleSource, target: "/data/trees/a"}, false, false},
		{"branch-component", data, roleEntry{role: RoleSource, target: "/data/sources-other/a"}, false, false},
		{"no-data-stage", data, roleEntry{role: RoleStaging, target: "/data/sources/a"}, false, false},
		{"no-state-child", roleEntry{role: RoleState, target: "/state"}, roleEntry{role: RoleSource, target: "/state/a"}, false, false},
		{"stage", cache, stage, false, true},
		{"stage-envelope", stage, roleEntry{role: RoleSource, target: stage.target + "/source"}, false, true},
		{"transitive", cache, roleEntry{role: RoleTree, target: stage.target + "/tree"}, true, true},
		{"unregistered-transitive", cache, roleEntry{role: RoleTree, target: stage.target + "/tree"}, false, false},
		{"cache-general-descendant", cache, roleEntry{role: RoleManager, target: "/cache/elsewhere"}, true, false},
		{"stage-nesting", stage, roleEntry{role: RoleStaging, target: stage.target + "/nested"}, false, false},
		{"operational-nesting", roleEntry{role: RoleSource, target: "/data/sources/a"}, roleEntry{role: RoleTree, target: "/data/sources/a/b"}, false, false},
		{"same-role-nesting", roleEntry{role: RoleSource, target: "/data/sources/a"}, roleEntry{role: RoleSource, target: "/data/sources/a/b"}, false, false},
		{"destination-child", roleEntry{role: RoleDestination, target: "/dest"}, roleEntry{role: RoleDestination, target: "/dest/a"}, false, false},
		{"equal", data, roleEntry{role: RoleSource, target: "/data"}, false, false},
		{"siblings", roleEntry{role: RoleSource, target: "/data/sources/a"}, roleEntry{role: RoleSource, target: "/data/sources/b"}, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			entries := []roleEntry{data, cache, test.parent, test.child}
			if test.registeredStage {
				entries = append(entries, stage)
			}
			for _, pair := range [][2]roleEntry{{test.parent, test.child}, {test.child, test.parent}} {
				allowed, valid := roleRelation(entries, pair[0], pair[1])
				if valid != test.valid {
					t.Fatal("unexpected matrix admission")
				}
				if valid && roleContains(test.parent.target, test.child.target) && !allowed {
					t.Fatal("permitted route lost containment relation")
				}
			}
		})
	}
	for _, target := range []string{
		"/cache/instances/0123456789abcdef0123456789abcdef/downloads",
		"/cache/instances/0123456789abcdef0123456789abcdeF/downloads/work",
		"/cache/instances/short/downloads/work",
		"/cache/other/0123456789abcdef0123456789abcdef/downloads/work",
	} {
		if roleStagingPath(cache.target, target) {
			t.Fatal("invalid staging spelling admitted")
		}
	}
	for _, entry := range []roleEntry{{role: RoleSource, target: "/elsewhere"}, {role: RoleStaging, target: "/elsewhere"}, {role: RoleDestination, target: "/home"}} {
		if rolePlacement([]roleEntry{data, cache}, entry, "/home") {
			t.Fatal("unanchored or whole-home registration admitted")
		}
	}
}

func TestRolePathLimits(t *testing.T) {
	for _, name := range []string{"", "relative", "/a//b", "/a/../b", "/a/", "/private\x00marker", "/" + strings.Repeat("a", 4096), "/" + strings.Repeat("a/", 256) + "b"} {
		if rolePath(name) {
			t.Fatal("invalid or over-limit target admitted")
		}
	}
	for _, name := range []string{"/", "/" + strings.Repeat("a", 4095), "/" + strings.Repeat("a/", 255) + "b"} {
		if !rolePath(name) {
			t.Fatal("bounded canonical target refused")
		}
	}
	chain := []edge{{}, {name: "a"}, {name: "b"}}
	if name, ok := roleChainPath(chain); !ok || name != "/a/b" {
		t.Fatal("wrong chain path")
	}
	chain[2].name = "../b"
	if _, ok := roleChainPath(chain); ok {
		t.Fatal("invalid edge name admitted")
	}
}

// Real local descriptors supply identities; absolute-root admission is not claimed.
func TestRolePrivateMkdirAndSharedAdvance(t *testing.T) {
	_, parent := localFixture(t)
	child, err := childAt(parent.fd, "shared", parent.id.Owner, true)
	if err != nil {
		t.Fatal(err)
	}
	ownChild(t, child)
	if !child.mkdir {
		t.Fatal("successful mkdir lost private provenance")
	}
	existing, err := childAt(parent.fd, "shared", parent.id.Owner, true)
	if err != nil {
		t.Fatal(err)
	}
	ownChild(t, existing)
	if existing.mkdir {
		t.Fatal("existing create=true acquired mkdir provenance")
	}
	entries := []roleEntry{
		{role: RoleState, target: "/shared/state/zerb", chain: []edge{parent}, missing: []string{"shared", "state", "zerb"}, owner: parent.id.Owner},
		{role: RoleData, target: "/shared/data/zerb", chain: []edge{parent}, missing: []string{"shared", "data", "zerb"}, owner: parent.id.Owner},
		{role: RoleCache, target: "/cache/zerb", chain: []edge{parent}, missing: []string{"cache", "zerb"}, owner: parent.id.Owner},
	}
	d := dirState{chain: []edge{parent, child}, owner: parent.id.Owner, mkdir: true}
	indices, ok := roleAdvance(entries, RoleState, &d, "/shared")
	if !ok || !reflect.DeepEqual(indices, []int{0, 1}) {
		t.Fatal("shared canonical frontier did not advance atomically")
	}
	for _, test := range []struct {
		name string
		edit func(*dirState)
	}{
		{"existing", func(d *dirState) { d.mkdir = false }},
		{"wrong-owner", func(d *dirState) { d.owner++ }},
		{"changed-prefix", func(d *dirState) { d.chain[0].id.Inode++ }},
		{"wrong-mode", func(d *dirState) { d.chain[1].id.Mode = unix.S_IFDIR | 0o755 }},
		{"skipped", func(d *dirState) { d.chain = append(d.chain, child) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := dirState{chain: append([]edge(nil), d.chain...), owner: d.owner, mkdir: d.mkdir}
			test.edit(&candidate)
			if _, ok := roleAdvance(entries, RoleState, &candidate, "/shared"); ok {
				t.Fatal("invalid prefix transition admitted")
			}
		})
	}
	mismatch := append([]roleEntry(nil), entries...)
	mismatch[1].chain = append([]edge(nil), entries[1].chain...)
	mismatch[1].chain[0].id.Inode++
	if _, ok := roleAdvance(mismatch, RoleState, &d, "/shared"); ok {
		t.Fatal("shared route with conflicting identity partly adopted")
	}
	complete := append([]roleEntry(nil), entries...)
	complete[0].missing = nil
	if _, ok := roleAdvance(complete, RoleState, &d, "/shared"); ok {
		t.Fatal("completed base rebound")
	}
	if len(entries[0].chain) != 1 || len(entries[0].missing) != 3 {
		t.Fatal("candidate selection mutated prior reservation")
	}
}

func TestRolesOwnedLifetime(t *testing.T) {
	_, parent := localFixture(t)
	callerChain, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal(err)
	}
	caller := &Dir{state: &dirState{chain: callerChain, owner: parent.id.Owner, identity: parent.id}}
	owned, err := duplicateChain(callerChain)
	if err != nil {
		t.Fatal(err)
	}
	r := &Roles{state: &rolesState{entries: []roleEntry{{role: RoleSource, target: "/fixture", chain: owned, owner: parent.id.Owner}}}}
	copyRoles := *r
	if err := caller.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validateEdges(owned, parent.id.Owner); err != nil {
		t.Fatal("caller closure closed retained descriptors")
	}
	for _, entry := range owned {
		flags, err := unix.FcntlInt(uintptr(entry.fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("retained descriptor lacks CLOEXEC")
		}
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := copyRoles.Close(); err != nil {
				t.Error(err)
			}
			if r.Validate() != errRoles {
				t.Error("closed roles validated")
			}
		}()
	}
	wg.Wait()
	if len(r.state.entries) != 0 || !r.state.closed {
		t.Fatal("Close retained ownership")
	}
	for _, invalid := range []*Roles{nil, {}, r} {
		if invalid.Add(RoleSource, nil) != errRoles || invalid.Validate() != errRoles {
			t.Fatal("invalid roles retained authority")
		}
		if err := invalid.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRolesCloseFailureDoesNotRetry(t *testing.T) {
	_, parent := localFixture(t)
	owned, err := duplicateChain([]edge{parent})
	if err != nil {
		t.Fatal(err)
	}
	fd := owned[0].fd
	if unix.Close(fd) != nil {
		t.Fatal("cannot inject released descriptor")
	}
	r := &Roles{state: &rolesState{entries: []roleEntry{{chain: owned}}}}
	if r.Close() != errRoles {
		t.Fatal("close failure not reported")
	}
	// Reuse exactly that number to detect an erroneous retry by another value copy.
	replacement, err := unix.FcntlInt(uintptr(parent.fd), unix.F_DUPFD_CLOEXEC, fd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if unix.Close(replacement) != nil {
			t.Error("replacement descriptor was closed")
		}
	}()
	if replacement != fd {
		t.Skip("released descriptor number was occupied before exact-number reuse")
	}
	copyRoles := *r
	if copyRoles.Close() != nil {
		t.Fatal("idempotent Close retried failed descriptor")
	}
	if _, err := observeFD(replacement); err != nil {
		t.Fatal("Close retried released descriptor")
	}
}

func TestOpenRolesReadOnlyRefusalAndCleanup(t *testing.T) {
	if got, err := OpenRoles(paths.Roots{}, uint32(os.Geteuid())); got != nil || err != errRoles {
		t.Fatal("zero Roots did not receive fixed zero-result refusal")
	}
	home := t.TempDir()
	input, err := paths.NewInput(home, []string{"XDG_DATA_HOME=" + filepath.Join(home, "data"), "XDG_STATE_HOME=" + filepath.Join(home, "state"), "XDG_CACHE_HOME=" + filepath.Join(home, "cache")})
	if err != nil {
		t.Fatal(err)
	}
	roots, err := paths.Resolve(input)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for range 12 {
		r, err := OpenRoles(roots, uint32(os.Geteuid()))
		if err == nil {
			if r == nil {
				t.Fatal("success returned no Roles")
			}
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
		} else if r != nil || err != errRoles {
			t.Fatal("failure leaked result or path-bearing error")
		}
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil || len(before) != len(after) {
		t.Fatal("OpenRoles leaked operation resources")
	}
	for _, target := range []string{roots.Data(), roots.State(), roots.Cache()} {
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatal("read-only reservation created or adopted a target")
		}
	}
}

func TestPublicRolesOnAdmissibleTopology(t *testing.T) {
	fixture := t.TempDir()
	owner := uint32(os.Geteuid())
	if !admissibleFixture(fixture, owner) || !roleFixtureFilesystems(fixture) {
		t.Skip("positive protected-role topology unavailable under unchanged owner/mode and btrfs/tmpfs policy")
	}
	setup := func(t *testing.T) (*Roles, *Dir, paths.Roots) {
		t.Helper()
		root := filepath.Join(fixture, t.Name()[strings.LastIndex(t.Name(), "/")+1:])
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		input, err := paths.NewInput(root, []string{"XDG_DATA_HOME=" + root + "/shared/data", "XDG_STATE_HOME=" + root + "/shared/state", "XDG_CACHE_HOME=" + root + "/cache"})
		if err != nil {
			t.Fatal(err)
		}
		roots, err := paths.Resolve(input)
		if err != nil {
			t.Fatal(err)
		}
		r, err := OpenRoles(roots, owner)
		if err != nil {
			t.Fatal("admissible role reservation failed")
		}
		t.Cleanup(func() {
			if err := r.Close(); err != nil {
				t.Error(err)
			}
		})
		parent, err := OpenRoot(root, owner)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := parent.Close(); err != nil {
				t.Error(err)
			}
		})
		return r, parent, roots
	}
	create := func(t *testing.T, parent *Dir, name string) *Dir {
		t.Helper()
		d, err := parent.OpenDir(name, true)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := d.Close(); err != nil {
				t.Error(err)
			}
		})
		return d
	}
	t.Run("transition-and-membership", func(t *testing.T) {
		r, parent, roots := setup(t)
		shared := create(t, parent, "shared")
		if err := r.Add(RoleData, shared); err != nil {
			t.Fatal("shared prefix transition failed")
		}
		if len(r.state.entries[0].missing) != 2 || len(r.state.entries[1].missing) != 2 {
			t.Fatal("shared frontier partly committed")
		}
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
		dataParent := create(t, shared, "data")
		if err := r.Add(RoleData, dataParent); err != nil {
			t.Fatal(err)
		}
		data := create(t, dataParent, "zerb")
		if err := r.Add(RoleData, data); err != nil {
			t.Fatal(err)
		}
		if err := r.Add(RoleData, data); err != errRoles {
			t.Fatal("complete base accepted duplicate binding")
		}
		if err := r.Validate(); err != nil {
			t.Fatal("duplicate base damaged prior set")
		}
		stateParent := create(t, shared, "state")
		if err := r.Add(RoleState, stateParent); err != nil {
			t.Fatal(err)
		}
		state := create(t, stateParent, "zerb")
		if err := r.Add(RoleState, state); err != nil {
			t.Fatal(err)
		}
		cacheParent := create(t, parent, "cache")
		if err := r.Add(RoleCache, cacheParent); err != nil {
			t.Fatal(err)
		}
		cache := create(t, cacheParent, "zerb")
		if err := r.Add(RoleCache, cache); err != nil {
			t.Fatal(err)
		}
		sources := create(t, data, "sources")
		source := create(t, sources, "source")
		if err := r.Add(RoleSource, source); err != nil {
			t.Fatal(err)
		}
		if err := r.Add(RoleSource, source); err != errRoles {
			t.Fatal("duplicate operational role admitted")
		}
		outside := create(t, parent, "outside")
		if err := r.Add(RoleSource, outside); err != errRoles {
			t.Fatal("unlisted placement admitted")
		}
		if err := r.Validate(); err != nil {
			t.Fatal("candidate-only failure damaged prior set")
		}
		if err := source.Close(); err != nil {
			t.Fatal(err)
		}
		if err := r.Validate(); err != nil {
			t.Fatal("caller close damaged retained membership")
		}
		old := filepath.Join(roots.Data(), "sources", "source")
		if err := os.Rename(old, old+"-moved"); err != nil {
			t.Fatal(err)
		}
		if r.Validate() != errRoles || !r.state.poisoned {
			t.Fatal("visible retained mismatch did not poison set")
		}
		if err := os.Rename(old+"-moved", old); err != nil {
			t.Fatal(err)
		}
		if r.Validate() != errRoles {
			t.Fatal("restoration cleared poison")
		}
	})
	t.Run("appeared-prefix", func(t *testing.T) {
		r, parent, _ := setup(t)
		create(t, parent, "shared")
		if r.Validate() != errRoles || !r.state.poisoned {
			t.Fatal("Validate adopted appeared prefix")
		}
		if len(r.state.entries[0].missing) != 3 {
			t.Fatal("Validate changed reservation membership")
		}
	})
	t.Run("existing-create-is-not-provenance", func(t *testing.T) {
		r, parent, _ := setup(t)
		create(t, parent, "shared")
		existing := create(t, parent, "shared")
		if existing.state.mkdir || r.Add(RoleState, existing) != errRoles {
			t.Fatal("existing child supplied private mkdir provenance")
		}
		if len(r.state.entries[0].missing) != 3 {
			t.Fatal("invalid candidate changed prefix membership")
		}
	})
}

func roleFixtureFilesystems(name string) bool {
	current := "/"
	for _, part := range append([]string{""}, strings.Split(strings.TrimPrefix(name, "/"), "/")...) {
		current = filepath.Join(current, part)
		var st unix.Statfs_t
		if unix.Statfs(current, &st) != nil || (st.Type != unix.BTRFS_SUPER_MAGIC && st.Type != unix.TMPFS_MAGIC) {
			return false
		}
	}
	return true
}
