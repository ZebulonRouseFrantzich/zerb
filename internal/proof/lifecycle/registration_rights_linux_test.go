//go:build linux && amd64

package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// ARCH-009/010, VAL-019: private registration framing only. The caller must
// authenticate the role and validate the returned descriptor's pidfd identity.
func receiveRegistrationRights(control *os.File) (_ *os.File, err error) {
	if control == nil {
		return nil, errors.New("missing registration control")
	}
	// The fixture exclusively owns this socket's options. Refuse before recvmsg
	// can install an automatic SCM_PIDFD outside the one-rights message contract.
	passPIDFD, optionErr := unix.GetsockoptInt(int(control.Fd()), unix.SOL_SOCKET, unix.SO_PASSPIDFD)
	if optionErr != nil {
		return nil, fmt.Errorf("inspect registration control options: %w", optionErr)
	}
	if passPIDFD != 0 {
		return nil, errors.New("registration control enables automatic pidfds")
	}
	var marker [1]byte
	oob := make([]byte, unix.CmsgSpace(4))
	var n, oobn, flags int
	for {
		n, oobn, flags, _, err = unix.Recvmsg(int(control.Fd()), marker[:], oob, unix.MSG_CMSG_CLOEXEC)
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("receive registration rights: %w", err)
	}
	var fds []int
	defer func() {
		if err != nil {
			for _, fd := range fds {
				if closeErr := unix.Close(fd); closeErr != nil {
					err = errors.Join(err, fmt.Errorf("close rejected registration right: %w", closeErr))
				}
			}
		}
	}()
	valid := n == 1 && marker[0] == 1 && flags&(unix.MSG_CTRUNC|unix.MSG_TRUNC) == 0
	messages := 0
	// Parse incrementally: an invalid later control must not discard an already
	// installed rights prefix. Linux truncation returns the delivered fd prefix.
	for remaining := oob[:oobn]; len(remaining) != 0; {
		if len(remaining) < unix.CmsgLen(0) {
			valid = false
			break
		}
		header, data, rest, parseErr := unix.ParseOneSocketControlMessage(remaining)
		if parseErr != nil {
			valid = false
			break
		}
		messages++
		if header.Level != unix.SOL_SOCKET || header.Type != unix.SCM_RIGHTS {
			valid = false
		} else {
			if len(data)%4 != 0 {
				valid = false
			}
			// Keep complete delivered rights even if a malformed tail is rejected.
			rights, rightsErr := unix.ParseUnixRights(&unix.SocketControlMessage{Header: header, Data: data[:len(data)/4*4]})
			fds = append(fds, rights...)
			if rightsErr != nil {
				valid = false
			}
		}
		remaining = rest
	}
	if !valid || messages != 1 || len(fds) != 1 {
		return nil, errors.New("invalid registration rights message")
	}
	fdFlags, flagErr := unix.FcntlInt(uintptr(fds[0]), unix.F_GETFD, 0)
	if flagErr != nil {
		return nil, fmt.Errorf("inspect registration right: %w", flagErr)
	}
	if fdFlags&unix.FD_CLOEXEC == 0 {
		return nil, errors.New("registration right lacks CLOEXEC")
	}
	return os.NewFile(uintptr(fds[0]), "registration-right"), nil
}

func TestRegistrationRights(t *testing.T) {
	for _, row := range []struct {
		name        string
		rights      int
		marker      []byte
		credentials bool
		passPIDFD   bool
		packet      bool
		accept      bool
	}{
		{name: "one-CLOEXEC", rights: 1, marker: []byte{1}, accept: true},
		{name: "zero", marker: []byte{1}},
		{name: "multiple", rights: 2, marker: []byte{1}},
		{name: "truncated-rights", rights: 3, marker: []byte{1}},
		{name: "bad-marker", rights: 1, marker: []byte{2}},
		{name: "bad-marker-multiple", rights: 2, marker: []byte{2}},
		{name: "credential-control-truncation", rights: 1, marker: []byte{1}, credentials: true},
		{name: "automatic-pidfd-preflight", rights: 1, marker: []byte{1}, passPIDFD: true},
		{name: "truncated-payload", rights: 1, marker: []byte{1, 2}, packet: true},
	} {
		t.Run(row.name, func(t *testing.T) {
			kind := unix.SOCK_STREAM
			if row.packet {
				kind = unix.SOCK_SEQPACKET
			}
			sockets, err := unix.Socketpair(unix.AF_UNIX, kind|unix.SOCK_CLOEXEC, 0)
			if err != nil {
				t.Fatal(err)
			}
			sender, receiver := os.NewFile(uintptr(sockets[0]), "rights-sender"), os.NewFile(uintptr(sockets[1]), "rights-receiver")
			registrationRightsCleanup(t, sender)
			registrationRightsCleanup(t, receiver)
			if row.credentials {
				if err := unix.SetsockoptInt(int(receiver.Fd()), unix.SOL_SOCKET, unix.SO_PASSCRED, 1); err != nil {
					t.Fatal(err)
				}
			}
			if row.passPIDFD {
				if err := unix.SetsockoptInt(int(receiver.Fd()), unix.SOL_SOCKET, unix.SO_PASSPIDFD, 1); err != nil {
					t.Fatal(err)
				}
			}
			var readers, writers []*os.File
			var rights []int
			for i := 0; i < row.rights; i++ {
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				registrationRightsCleanup(t, reader)
				registrationRightsCleanup(t, writer)
				readers, writers = append(readers, reader), append(writers, writer)
				rights = append(rights, int(writer.Fd()))
			}
			var ancillary []byte
			if len(rights) != 0 {
				ancillary = unix.UnixRights(rights...)
			}
			if err := unix.Sendmsg(int(sender.Fd()), row.marker, ancillary, nil, 0); err != nil {
				t.Fatal(err)
			}
			received, err := receiveRegistrationRights(receiver)
			if received != nil {
				registrationRightsCleanup(t, received)
			}
			if row.accept {
				if err != nil || received == nil {
					t.Fatalf("valid right refused: %v", err)
				}
				flags, err := unix.FcntlInt(received.Fd(), unix.F_GETFD, 0)
				if err != nil || flags&unix.FD_CLOEXEC == 0 {
					t.Fatalf("received right lacks CLOEXEC: flags=%#x err=%v", flags, err)
				}
				if _, err := received.Write([]byte{1}); err != nil {
					t.Fatal(err)
				}
				var marker [1]byte
				if n, err := readers[0].Read(marker[:]); err != nil || n != 1 || marker[0] != 1 {
					t.Fatalf("received capability is not sender pipe: n=%d marker=%v err=%v", n, marker, err)
				}
				if err := received.Close(); err != nil {
					t.Fatal(err)
				}
			} else if err == nil || received != nil {
				t.Fatalf("invalid right accepted: file=%v err=%v", received, err)
			}
			if row.passPIDFD {
				if err.Error() != "registration control enables automatic pidfds" {
					t.Fatalf("automatic pidfd socket refused for wrong reason: %v", err)
				}
				if err := unix.SetsockoptInt(int(receiver.Fd()), unix.SOL_SOCKET, unix.SO_PASSPIDFD, 0); err != nil {
					t.Fatal(err)
				}
				// The queued right must remain untouched by the pre-receive refusal.
				received, err = receiveRegistrationRights(receiver)
				if err != nil || received == nil {
					t.Fatalf("preflight consumed queued capability: %v", err)
				}
				registrationRightsCleanup(t, received)
				if err := received.Close(); err != nil {
					t.Fatal(err)
				}
			}
			for i, writer := range writers {
				// A rejected duplicate must not close or otherwise consume its sender.
				if _, err := writer.Write([]byte{1}); err != nil {
					t.Fatalf("sender ownership lost: %v", err)
				}
				var marker [1]byte
				if n, err := readers[i].Read(marker[:]); err != nil || n != 1 || marker[0] != 1 {
					t.Fatalf("sender capability changed: n=%d marker=%v err=%v", n, marker, err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				// Pipe EOF detects a leaked received writer without a global fd count.
				poll := []unix.PollFd{{Fd: int32(readers[i].Fd()), Events: unix.POLLHUP}}
				if n, err := unix.Poll(poll, 1000); err != nil || n != 1 || poll[0].Revents != unix.POLLHUP {
					t.Fatalf("received write capability leaked: n=%d events=%#x err=%v", n, poll[0].Revents, err)
				}
			}
		})
	}
}

func registrationRightsCleanup(t *testing.T, file *os.File) {
	t.Helper()
	t.Cleanup(func() {
		if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close registration fixture: %v", err)
		}
	})
}
