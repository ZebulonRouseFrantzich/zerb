//go:build linux

package execenv

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func streamFD(t *testing.T, file *os.File) int {
	t.Helper()
	raw, err := file.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	fd := -1
	if err = raw.Control(func(value uintptr) { fd = int(value) }); err != nil {
		t.Fatal(err)
	}
	return fd
}

func streamFlags(t *testing.T, file *os.File) int {
	t.Helper()
	raw, err := file.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var flags int
	var flagsErr error
	if err = raw.Control(func(fd uintptr) { flags, flagsErr = unix.FcntlInt(fd, unix.F_GETFL, 0) }); err != nil || flagsErr != nil {
		t.Fatal("inspect stream flags", err, flagsErr)
	}
	return flags
}

func streamDescriptors(t *testing.T) map[int]uint64 {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[int]uint64)
	for _, entry := range entries {
		fd, err := strconv.Atoi(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		var stat unix.Stat_t
		if err = unix.Fstat(fd, &stat); errors.Is(err, unix.EBADF) {
			continue
		} else if err != nil {
			t.Fatal(err)
		}
		result[fd] = stat.Ino
	}
	return result
}

func assertStreamDescriptors(t *testing.T, before map[int]uint64) {
	t.Helper()
	after := streamDescriptors(t)
	if len(after) != len(before) {
		t.Fatalf("descriptor count changed: %d -> %d", len(before), len(after))
	}
	for fd, inode := range before {
		if after[fd] != inode {
			t.Fatal("descriptor identity changed")
		}
	}
}

func closeStreams(t *testing.T, owned *ownedStreams) {
	t.Helper()
	if err := owned.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStreamsRegularAliasesPreserveCaller(t *testing.T) {
	file, name := fingerprintFile(t, []byte("0123456789"))
	if _, err := file.Seek(3, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	beforeFlags, beforeFD := streamFlags(t, file), streamFD(t, file)
	beforeDescriptors := streamDescriptors(t)
	owned, err := duplicateStreams([3]*os.File{file, file, file})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeStreams(t, owned) })
	seen := map[int]bool{beforeFD: true}
	for _, duplicate := range owned.files {
		fd := streamFD(t, duplicate)
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 || seen[fd] {
			t.Fatal("duplicate is aliased or lacks close-on-exec")
		}
		seen[fd] = true
		originalInfo, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		duplicateInfo, err := duplicate.Stat()
		if err != nil || !os.SameFile(originalInfo, duplicateInfo) {
			t.Fatal("duplicate changed file identity")
		}
	}
	if offset, err := file.Seek(0, io.SeekCurrent); err != nil || offset != 3 || streamFlags(t, file) != beforeFlags {
		t.Fatal("acquisition changed caller offset or flags")
	}
	if _, err := owned.files[0].Seek(6, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if offset, err := file.Seek(0, io.SeekCurrent); err != nil || offset != 6 {
		t.Fatal("duplicates do not share the open-file offset")
	}
	closeStreams(t, owned)
	closeStreams(t, owned)
	if _, err := file.Stat(); err != nil {
		t.Fatal("owned closure closed caller", err)
	}
	assertStreamDescriptors(t, beforeDescriptors)
}

func TestStreamsPipeFlagsAndCallerLifetime(t *testing.T) {
	for _, nonblocking := range []bool{false, true} {
		var descriptors [2]int
		flags := unix.O_CLOEXEC
		if nonblocking {
			flags |= unix.O_NONBLOCK
		}
		if err := unix.Pipe2(descriptors[:], flags); err != nil {
			t.Fatal(err)
		}
		reader, writer := os.NewFile(uintptr(descriptors[0]), "input"), os.NewFile(uintptr(descriptors[1]), "output")
		closeFingerprintFile(t, reader)
		closeFingerprintFile(t, writer)
		readFlags, writeFlags := streamFlags(t, reader), streamFlags(t, writer)
		owned, err := duplicateStreams([3]*os.File{reader, writer, writer})
		if err != nil {
			t.Fatal(err)
		}
		if streamFlags(t, reader) != readFlags || streamFlags(t, writer) != writeFlags {
			t.Fatal("acquisition changed pipe status flags")
		}
		if _, err = owned.files[1].Write([]byte("a")); err != nil {
			t.Fatal(err)
		}
		var value [1]byte
		if _, err = io.ReadFull(owned.files[0], value[:]); err != nil || value[0] != 'a' {
			t.Fatal("pipe duplicate lost data", err)
		}
		closeStreams(t, owned)
		if _, err = writer.Write([]byte("b")); err != nil {
			t.Fatal("caller pipe writer closed", err)
		}
		if _, err = io.ReadFull(reader, value[:]); err != nil || value[0] != 'b' {
			t.Fatal("caller pipe reader closed", err)
		}
	}
}

func TestStreamsGoPipePreservesNonblockingMode(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	closeFingerprintFile(t, reader)
	closeFingerprintFile(t, writer)
	readFlags, writeFlags := streamFlags(t, reader), streamFlags(t, writer)
	if readFlags&unix.O_NONBLOCK == 0 || writeFlags&unix.O_NONBLOCK == 0 {
		t.Fatal("Go pipe did not establish nonblocking descriptors")
	}
	owned, err := duplicateStreams([3]*os.File{reader, writer, writer})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeStreams(t, owned) })
	if streamFlags(t, reader) != readFlags || streamFlags(t, writer) != writeFlags {
		t.Fatal("acquisition changed Go-managed pipe status flags")
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = owned.files[1].Write([]byte("a")); err != nil {
		t.Fatal("caller closure closed owned writer", err)
	}
	var value [1]byte
	if _, err = io.ReadFull(owned.files[0], value[:]); err != nil || value[0] != 'a' {
		t.Fatal("caller closure closed owned reader", err)
	}
	closeStreams(t, owned)
}

func TestStreamsFailureIsTransactional(t *testing.T) {
	file, _ := fingerprintFile(t, []byte("caller"))
	closed, _ := fingerprintFile(t, nil)
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	for _, position := range []int{0, 1, 2} {
		for _, invalid := range []*os.File{nil, closed} {
			before := streamDescriptors(t)
			files := [3]*os.File{file, file, file}
			files[position] = invalid
			owned, err := duplicateStreams(files)
			if owned != nil || err != errStreams || err.Error() != "process streams unavailable" {
				t.Fatal("failure returned partial ownership or non-fixed error")
			}
			assertStreamDescriptors(t, before)
			if invalid == nil {
				if allocations := testing.AllocsPerRun(3, func() { _, _ = duplicateStreams(files) }); allocations != 0 {
					t.Fatal("nil endpoint preflight allocated", allocations)
				}
			}
			if _, err := file.Stat(); err != nil {
				t.Fatal("failure closed caller", err)
			}
		}
	}
	var zero *ownedStreams
	closeStreams(t, zero)
	closeStreams(t, &ownedStreams{})
}

func TestStreamsSparseCallerDescriptor(t *testing.T) {
	file, _ := fingerprintFile(t, []byte("sparse"))
	before := streamDescriptors(t)
	callerFD := streamFD(t, file)
	fd, err := unix.FcntlInt(uintptr(callerFD), unix.F_DUPFD_CLOEXEC, callerFD+64)
	if err != nil {
		t.Fatal(err)
	}
	sparse := os.NewFile(uintptr(fd), "sparse stream")
	owned, err := duplicateStreams([3]*os.File{sparse, sparse, sparse})
	if err != nil {
		t.Fatal(err)
	}
	closeStreams(t, owned)
	if _, err := sparse.Stat(); err != nil {
		t.Fatal("closed sparse caller", err)
	}
	if err := sparse.Close(); err != nil {
		t.Fatal(err)
	}
	assertStreamDescriptors(t, before)
}

func TestStreamsConcurrentOriginalClose(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	for _, name := range []string{first, second} {
		if err := os.WriteFile(name, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	before := streamDescriptors(t)
	for range 100 {
		file, err := os.Open(first)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		closed := make(chan *os.File)
		go func() {
			<-start
			if err := file.Close(); err != nil {
				t.Error(err)
			}
			replacement, err := os.Open(second)
			if err != nil {
				t.Error(err)
			}
			closed <- replacement
		}()
		close(start)
		owned, err := duplicateStreams([3]*os.File{file, file, file})
		replacement := <-closed
		if replacement != nil {
			if err := replacement.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if err != nil {
			if owned != nil || err != errStreams {
				t.Fatal("concurrent failure returned partial ownership")
			}
			continue
		}
		for _, duplicate := range owned.files {
			got, err := duplicate.Stat()
			if err != nil || !os.SameFile(info, got) {
				t.Fatal("concurrent caller close duplicated a reused descriptor")
			}
		}
		closeStreams(t, owned)
	}
	assertStreamDescriptors(t, before)
}

func TestStreamsConcurrentCloseCachesCleanupFailure(t *testing.T) {
	for _, broken := range []bool{false, true} {
		file, _ := fingerprintFile(t, []byte("caller"))
		before := streamDescriptors(t)
		owned, err := duplicateStreams([3]*os.File{file, file, file})
		if err != nil {
			t.Fatal(err)
		}
		var replacement *os.File
		if broken {
			releasedFD := streamFD(t, owned.files[1])
			if err := owned.files[1].Close(); err != nil {
				t.Fatal(err)
			}
			reused, err := unix.FcntlInt(uintptr(streamFD(t, file)), unix.F_DUPFD_CLOEXEC, releasedFD)
			if err != nil {
				t.Fatal(err)
			}
			if reused != releasedFD {
				if err := unix.Close(reused); err != nil {
					t.Fatal(err)
				}
				t.Fatal("released descriptor was not available for reuse")
			}
			replacement = os.NewFile(uintptr(reused), "replacement stream")
		}
		var want error
		if broken {
			want = errStreams
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := owned.Close(); err != want {
					t.Error("cleanup error was not retained", err)
				}
			}()
		}
		wg.Wait()
		if owned.files != ([3]*os.File{}) || owned.Close() != want {
			t.Fatal("cleanup did not drain or cache its failure")
		}
		if replacement != nil {
			if _, err := replacement.Stat(); err != nil {
				t.Fatal("cleanup retried a released descriptor number", err)
			}
			if err := replacement.Close(); err != nil {
				t.Fatal(err)
			}
		}
		assertStreamDescriptors(t, before)
	}
}

func TestStreamsDescriptorExhaustion(t *testing.T) {
	if os.Getenv("ZERB_STREAM_EXHAUSTION_TEST") != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestStreamsDescriptorExhaustion$")
		command.Env = append(os.Environ(), "ZERB_STREAM_EXHAUSTION_TEST=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("isolated descriptor exhaustion: %v\n%s", err, output)
		}
		return
	}
	file, _ := fingerprintFile(t, []byte("caller"))
	fd := streamFD(t, file)
	var original unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &original); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &original); err != nil {
			t.Error(err)
		}
	})
	for available := 0; available < 3; available++ {
		before := streamDescriptors(t)
		limit := original
		if limit.Cur > 64 {
			limit.Cur = 64
		}
		if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &limit); err != nil {
			t.Fatal(err)
		}
		var retained []int
		var fillErr error
		for len(retained) <= int(limit.Cur) {
			duplicate, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
			if err != nil {
				fillErr = err
				break
			}
			retained = append(retained, duplicate)
		}
		if len(retained) < available || !errors.Is(fillErr, unix.EMFILE) {
			if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &original); err != nil {
				t.Fatal(err)
			}
			t.Fatal("did not reach bounded descriptor exhaustion", fillErr)
		}
		for range available {
			last := retained[len(retained)-1]
			if err := unix.Close(last); err != nil {
				fillErr = err
			}
			retained = retained[:len(retained)-1]
		}
		owned, acquisitionErr := duplicateStreams([3]*os.File{file, file, file})
		// Restore before any assertion, logging, enumeration or coverage shutdown.
		if err := unix.Setrlimit(unix.RLIMIT_NOFILE, &original); err != nil {
			t.Fatal(err)
		}
		if owned != nil || acquisitionErr != errStreams || !errors.Is(fillErr, unix.EMFILE) {
			t.Fatal("exhaustion did not refuse transactionally", acquisitionErr, fillErr)
		}
		for _, duplicate := range retained {
			if err := unix.Close(duplicate); err != nil {
				t.Fatal(err)
			}
		}
		assertStreamDescriptors(t, before)
	}
}
