//go:build linux

package state

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func processStatFixture(pid, comm, start string) []byte {
	fields := []string{"S"}
	for i := 4; i < 22; i++ {
		fields = append(fields, "0")
	}
	fields = append(fields, start, "123", "-4")
	return []byte(pid + " (" + comm + ") " + strings.Join(fields, " ") + "\n")
}

func TestCurrentProcessStatGrammar(t *testing.T) {
	valid := processStatFixture("123", "ordinary", "456")
	tests := []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"ordinary", valid, true},
		{"empty comm", processStatFixture("123", "", "456"), true},
		{"comm whitespace parentheses newline", processStatFixture("123", "a ) (\t\r\nb) (", "456"), true},
		{"comm Linux bytes", processStatFixture("123", "a\xffb", "456"), true},
		{"wrong PID", processStatFixture("124", "ordinary", "456"), false},
		{"noncanonical PID", processStatFixture("0123", "ordinary", "456"), false},
		{"missing PID space", []byte(strings.Replace(string(valid), "123 (", "123(", 1)), false},
		{"missing close separator", []byte(strings.Replace(string(valid), ") S", ")S", 1)), false},
		{"missing newline", valid[:len(valid)-1], false},
		{"extra newline", append(append([]byte(nil), valid...), '\n'), false},
		{"NUL comm", processStatFixture("123", "a\x00b", "456"), false},
		{"missing start", []byte("123 (ordinary) S 0\n"), false},
		{"zero start", processStatFixture("123", "ordinary", "0"), false},
		{"negative start", processStatFixture("123", "ordinary", "-1"), false},
		{"signed start", processStatFixture("123", "ordinary", "+1"), false},
		{"overflow start", processStatFixture("123", "ordinary", "18446744073709551616"), false},
		{"nondecimal start", processStatFixture("123", "ordinary", "0xff"), false},
		{"split start", processStatFixture("123", "ordinary", "4\t56"), false},
		{"empty suffix field", processStatFixture("123", "ordinary", " 456"), false},
		{"multi-byte state", []byte(strings.Replace(string(valid), ") S", ") SS", 1)), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, err := parseProcessStat(tt.data, "123")
			if (err == nil) != tt.valid {
				t.Fatalf("accepted=%v", err == nil)
			}
			if tt.valid && start != 456 {
				t.Fatal("wrong field22")
			}
		})
	}
	// Accounting values may change while the identity fields remain the same.
	changed := []byte(strings.Replace(string(valid), " 123 -4\n", " 999 -8 42\n", 1))
	if start, err := parseProcessStat(changed, "123"); err != nil || start != 456 {
		t.Fatal("dynamic accounting affected identity")
	}
	base := processStatFixture("123", "", "456")
	exact := processStatFixture("123", strings.Repeat("a", processStatLimit-len(base)), "456")
	if len(exact) != processStatLimit {
		t.Fatal("invalid boundary fixture")
	}
	if _, err := parseProcessStat(exact, "123"); err != nil {
		t.Fatal("exact stat limit refused")
	}
	over := append([]byte{'x'}, exact...)
	if _, err := parseProcessStat(over, "123"); err == nil {
		t.Fatal("overflow stat admitted")
	}
	max := processStatFixture("123", "ordinary", strconv.FormatUint(^uint64(0), 10))
	if start, err := parseProcessStat(max, "123"); err != nil || start != ^uint64(0) {
		t.Fatal("uint64 boundary")
	}
}

func TestCurrentProcessNamespaceGrammar(t *testing.T) {
	for _, value := range []string{"pid:[]", "pid:[0]", "pid:[-1]", "pid:[+1]", "pid:[18446744073709551616]", "pid:[1]suffix", "pid:[1]\n", "mnt:[1]", "pid:[1\x00]"} {
		if _, ok := processNamespaceNumber(value); ok {
			t.Fatalf("invalid namespace link admitted: %q", value)
		}
	}
	if n, ok := processNamespaceNumber("pid:[4026531836]"); !ok || n != 4026531836 {
		t.Fatal("positive decimal namespace inode")
	}
}

// This proves only the current fixed proc observation, independently of
// protected-root admission, FreshInit ordering, and frozen-host qualification.
func TestCurrentProcessKernelMetadata(t *testing.T) {
	p, err := captureCurrentProcess()
	if err != nil {
		t.Skip("current kernel/proc process capability unavailable")
	}
	defer func() {
		if err := p.close(); err != nil {
			t.Error(err)
		}
	}()
	want := p.identity
	if want.PID != unix.Getpid() || want.Start == 0 || want.PIDNamespace == 0 || want.PIDNamespace != p.namespace.id.Inode {
		t.Fatal("invalid current process tuple")
	}
	number, err := p.namespaceNumber()
	if err != nil || number != want.PIDNamespace {
		t.Fatal("namespace target inode mismatch")
	}
	owned := append([]processFD(nil), p.chain...)
	owned = append(owned, p.self, p.stat, p.nsLink, p.namespace)
	for _, entry := range owned {
		flags, err := unix.FcntlInt(uintptr(entry.fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("descriptor inheritance")
		}
	}
	for i := 0; i < 3; i++ {
		if p.revalidate() != nil || p.identity != want {
			t.Fatal("stable process revalidation")
		}
	}
}

func TestCurrentProcessTerminalEvidenceMismatch(t *testing.T) {
	for _, name := range []string{"PID", "start", "PID namespace", "root", "proc", "numeric PID", "ns directory", "self", "stat", "namespace link", "namespace descriptor"} {
		t.Run(name, func(t *testing.T) {
			p, err := captureCurrentProcess()
			if err != nil {
				t.Skip("current kernel/proc process capability unavailable")
			}
			defer func() {
				if err := p.close(); err != nil {
					t.Error(err)
				}
			}()
			switch name {
			case "PID":
				p.pid = "0"
			case "start":
				p.identity.Start++
			case "PID namespace":
				p.identity.PIDNamespace++
			case "root":
				p.chain[0].id.Inode++
			case "proc":
				p.chain[1].id.Inode++
			case "numeric PID":
				p.chain[2].id.Inode++
			case "ns directory":
				p.chain[3].id.Inode++
			case "self":
				p.self.id.Inode++
			case "stat":
				p.stat.id.Inode++
			case "namespace link":
				p.nsLink.id.Inode++
			case "namespace descriptor":
				p.namespace.id.Inode++
			}
			if p.revalidate() == nil || !p.closed || p.identity != (processIdentity{}) {
				t.Fatal("mismatch did not terminally invalidate identity")
			}
			if len(p.chain) != 0 || p.self.fd != -1 || p.stat.fd != -1 || p.nsLink.fd != -1 || p.namespace.fd != -1 {
				t.Fatal("owned descriptors not cleared")
			}
			if p.revalidate() == nil {
				t.Fatal("closed observation authorized continuation")
			}
		})
	}
}

func TestCurrentProcessProcAndNamespaceRefusals(t *testing.T) {
	p, err := captureCurrentProcess()
	if err != nil {
		t.Skip("current kernel/proc process capability unavailable")
	}
	defer func() {
		if err := p.close(); err != nil {
			t.Error(err)
		}
	}()
	if processFilesystem(p.chain[0].fd, unix.PROC_SUPER_MAGIC) {
		t.Skip("root itself uses procfs")
	}
	if processNamespaceValid(p.chain[0]) {
		t.Fatal("directory accepted as PID namespace")
	}
	wrong := p.stat
	wrong.id.MountID++
	if p.checkProcObject(wrong, p.chain[2], unix.S_IFREG) == nil {
		t.Fatal("wrong proc mount accepted")
	}
	wrong = p.stat
	wrong.name = "ns"
	if p.checkProcObject(wrong, p.chain[2], unix.S_IFREG) == nil {
		t.Fatal("changed stat name edge accepted")
	}
	failed, err := p.openProcObject(p.chain[2], "stat", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, unix.S_IFLNK)
	if err == nil || failed.fd != -1 {
		t.Fatal("failed acquisition returned a descriptor number")
	}
	if p.revalidate() != nil {
		t.Fatal("failed temporary acquisition damaged retained observation")
	}
	// The generated PID path remains unchanged; no foreign process is inspected.
}

func TestCurrentProcessConcurrentLifetime(t *testing.T) {
	p, err := captureCurrentProcess()
	if err != nil {
		t.Skip("current kernel/proc process capability unavailable")
	}
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := p.revalidate(); err != nil && !errors.Is(err, errCurrentProcess) {
				t.Error(err)
			}
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		if err := p.close(); err != nil {
			t.Error(err)
		}
	}()
	group.Wait()
	if !p.closed || p.identity != (processIdentity{}) || p.close() != nil || p.revalidate() == nil {
		t.Fatal("lifetime after concurrent close")
	}
}

func TestCurrentProcessCleanupFailure(t *testing.T) {
	p := &currentProcess{self: processFD{fd: 1 << 30, owned: true}, stat: processFD{fd: -1}, nsLink: processFD{fd: -1}, namespace: processFD{fd: -1}, identity: processIdentity{1, 2, 3}}
	if p.close() == nil || !p.closed || p.identity != (processIdentity{}) || p.self.fd != -1 {
		t.Fatal("cleanup failure not reported and cleared")
	}
	if p.close() != nil {
		t.Fatal("cleanup retried released descriptor number")
	}
	var absent *currentProcess
	if absent.close() != nil || absent.revalidate() == nil {
		t.Fatal("nil process lifetime")
	}
}

func FuzzCurrentProcessStat(f *testing.F) {
	f.Add(processStatFixture("123", "ordinary", "456"))
	f.Add(processStatFixture("123", "a ) (\n b", "456"))
	f.Fuzz(func(t *testing.T, data []byte) {
		start, err := parseProcessStat(data, "123")
		if err == nil && (start == 0 || len(data) > processStatLimit) {
			t.Fatal("invalid accepted process identity")
		}
	})
}

func TestCurrentProcessZeroValueLifetime(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "stdin-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := input.Close(); err != nil {
			t.Error(err)
		}
	}()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCurrentProcessZeroValueChild$", "-test.v")
	cmd.Env = append(os.Environ(), "ZERB_PROCESS_LIFETIME_CHILD=1")
	cmd.Stdin = input
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated lifetime check: %v\n%s", err, output)
	}
	if !bytes.Contains(output, []byte("process descriptor zero constructor verified")) {
		t.Skip("zero-value lifetime checks passed; current-process FD0 constructor unavailable")
	}
}

func TestCurrentProcessZeroValueChild(t *testing.T) {
	if os.Getenv("ZERB_PROCESS_LIFETIME_CHILD") != "1" {
		t.Skip("isolated subprocess only")
	}
	before, err := processObserve(0, "")
	if err != nil {
		t.Fatal("safe child input unavailable")
	}
	empty := &currentProcess{}
	for i := 0; i < 2; i++ {
		if empty.close() != nil {
			t.Fatal("zero-value close")
		}
	}
	rejected := &currentProcess{}
	if rejected.revalidate() == nil || !rejected.closed || rejected.close() != nil {
		t.Fatal("zero-value revalidation")
	}
	after, err := processObserve(0, "")
	if err != nil || before != after {
		t.Fatal("unowned descriptor zero was changed")
	}
	// The child owns this disposable stdin; parent descriptors are untouched.
	owned := &currentProcess{self: processFD{fd: 0, owned: true}}
	for i := 0; i < 2; i++ {
		if owned.close() != nil {
			t.Fatal("owned descriptor zero cleanup with zero identity")
		}
	}
	if _, err := unix.FcntlInt(0, unix.F_GETFD, 0); err != unix.EBADF {
		t.Fatal("owned descriptor zero was retained")
	}
	// With slot zero vacant, the real constructor must explicitly own its root FD.
	captured, err := captureCurrentProcess()
	if err != nil {
		t.Skip("zero-value lifetime checks passed; current kernel/proc constructor unavailable")
	}
	if len(captured.chain) == 0 || captured.chain[0].fd != 0 || !captured.chain[0].owned {
		_ = captured.close()
		t.Fatal("constructor did not own allocated descriptor zero")
	}
	if captured.close() != nil {
		t.Fatal("constructor descriptor zero cleanup")
	}
	if _, err := unix.FcntlInt(0, unix.F_GETFD, 0); err != unix.EBADF {
		t.Fatal("constructor descriptor zero leaked")
	}
	t.Log("process descriptor zero constructor verified")
}
