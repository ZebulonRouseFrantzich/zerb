//go:build linux && amd64

package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

type assemblyMessage struct {
	Phase, Role, Nonce string
	IDs                [5]int
	ChildPID           int
	Image              executableIdentity
	Raw                uint32
	Armed              bool
}

func assemblySocketPair() (*os.File, *os.File, error) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("create assembly channel: %w", err)
	}
	for _, fd := range fds {
		if err = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_PASSCRED, 1); err == nil {
			var pass int
			pass, err = unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_PASSPIDFD)
			if err == nil && pass != 0 {
				err = errors.New("assembly channel enables automatic pidfds")
			}
		}
		if err != nil {
			return nil, nil, errors.Join(fmt.Errorf("configure assembly channel: %w", err), unix.Close(fds[0]), unix.Close(fds[1]))
		}
	}
	return os.NewFile(uintptr(fds[0]), "assembly-left"), os.NewFile(uintptr(fds[1]), "assembly-right"), nil
}

func sendAssemblyPacket(control *os.File, frame assemblyMessage, rights ...int) error {
	if control == nil || len(rights) > 4 {
		return errors.New("invalid assembly send arguments")
	}
	payload, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("encode assembly packet: %w", err)
	}
	if len(payload) > 2048 {
		return errors.New("assembly packet exceeds bound")
	}
	var ancillary []byte
	if len(rights) != 0 {
		ancillary = unix.UnixRights(rights...)
	}
	for {
		n, sendErr := unix.SendmsgN(int(control.Fd()), payload, ancillary, nil, 0)
		if sendErr == unix.EINTR && n == 0 {
			continue
		}
		if sendErr != nil {
			return fmt.Errorf("send assembly packet: %w", sendErr)
		}
		if n != len(payload) {
			return errors.New("short assembly packet send")
		}
		return nil
	}
}

// ARCH-009/010, VAL-018/019: sender-authenticated proof transport only. The
// caller owns socket options and validates role, nonce, image and pidfd identity.
func receiveAssemblyPacket(control *os.File, expectedPID int, expectedUID uint32, wantRights int) (_ assemblyMessage, _ []*os.File, err error) {
	if control == nil || expectedPID <= 0 || wantRights < 0 || wantRights > 4 {
		return assemblyMessage{}, nil, errors.New("invalid assembly receive arguments")
	}
	for _, option := range []struct{ key, value int }{
		{unix.SO_DOMAIN, unix.AF_UNIX}, {unix.SO_TYPE, unix.SOCK_SEQPACKET},
		{unix.SO_PASSCRED, 1}, {unix.SO_PASSPIDFD, 0},
	} {
		value, optionErr := unix.GetsockoptInt(int(control.Fd()), unix.SOL_SOCKET, option.key)
		if optionErr != nil {
			return assemblyMessage{}, nil, fmt.Errorf("inspect assembly channel: %w", optionErr)
		}
		if value != option.value {
			return assemblyMessage{}, nil, errors.New("invalid assembly channel options")
		}
	}
	var payload [2048]byte
	oob := make([]byte, unix.CmsgSpace(unix.SizeofUcred)+unix.CmsgSpace(4*4))
	var n, oobn, flags int
	for {
		n, oobn, flags, _, err = unix.Recvmsg(int(control.Fd()), payload[:], oob, unix.MSG_CMSG_CLOEXEC)
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		return assemblyMessage{}, nil, fmt.Errorf("receive assembly packet: %w", err)
	}
	var fds []int
	defer func() {
		if err != nil {
			for _, fd := range fds {
				if closeErr := unix.Close(fd); closeErr != nil {
					err = errors.Join(err, fmt.Errorf("close rejected assembly right: %w", closeErr))
				}
			}
		}
	}()
	valid := n > 0 && flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC) == 0
	credentialCount, rightsCount := 0, 0
	var credentials *unix.Ucred
	// Collect every installed right before rejecting credentials or payload.
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
		message := unix.SocketControlMessage{Header: header, Data: data}
		switch {
		case header.Level == unix.SOL_SOCKET && header.Type == unix.SCM_RIGHTS:
			rightsCount++
			if len(data)%4 != 0 {
				valid = false
			}
			message.Data = data[:len(data)/4*4]
			rights, parseErr := unix.ParseUnixRights(&message)
			fds = append(fds, rights...)
			if parseErr != nil {
				valid = false
			}
		case header.Level == unix.SOL_SOCKET && header.Type == unix.SCM_CREDENTIALS:
			credentialCount++
			if len(data) != unix.SizeofUcred {
				valid = false
				break
			}
			var parseErr error
			credentials, parseErr = unix.ParseUnixCredentials(&message)
			if parseErr != nil {
				valid = false
			}
		default:
			valid = false
		}
		remaining = rest
	}
	wantControls := 0
	if wantRights > 0 {
		wantControls = 1
	}
	if !valid || credentialCount != 1 || credentials == nil || int(credentials.Pid) != expectedPID || credentials.Uid != expectedUID || len(fds) != wantRights || rightsCount != wantControls {
		return assemblyMessage{}, nil, errors.New("invalid assembly packet controls or sender")
	}
	var frame assemblyMessage
	trimmed := bytes.TrimSpace(payload[:n])
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return assemblyMessage{}, nil, errors.New("assembly packet must be an object")
	}
	if err := decodeStrict(trimmed, &frame); err != nil {
		return assemblyMessage{}, nil, errors.New("invalid assembly packet JSON")
	}
	files := make([]*os.File, 0, len(fds))
	for _, fd := range fds {
		fdFlags, flagErr := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if flagErr != nil {
			return assemblyMessage{}, nil, fmt.Errorf("inspect assembly right: %w", flagErr)
		}
		if fdFlags&unix.FD_CLOEXEC == 0 {
			return assemblyMessage{}, nil, errors.New("assembly right lacks CLOEXEC")
		}
	}
	for _, fd := range fds {
		files = append(files, os.NewFile(uintptr(fd), "assembly-right"))
	}
	return frame, files, nil
}

func TestAssemblyPacket(t *testing.T) {
	frame := assemblyMessage{Phase: "transport", Role: "fixture", Nonce: "disposable", IDs: [5]int{1, 2, 3, 4, 5}}
	encoded, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name         string
		rights, want int
		wrongPID     bool
		wrongUID     bool
		payload      []byte
		timestamp    bool
		accept       bool
	}{
		{name: "zero-rights", accept: true},
		{name: "one-CLOEXEC", rights: 1, want: 1, accept: true},
		{name: "four-CLOEXEC", rights: 4, want: 4, accept: true},
		{name: "wrong-sender-PID", rights: 4, want: 4, wrongPID: true},
		{name: "wrong-sender-UID", rights: 4, want: 4, wrongUID: true},
		{name: "missing-right", want: 1},
		{name: "extra-right", rights: 2, want: 1},
		{name: "truncated-rights", rights: 8, want: 4},
		{name: "malformed-JSON", rights: 4, want: 4, payload: []byte("{")},
		{name: "nonobject-JSON", rights: 4, want: 4, payload: []byte("null")},
		{name: "unknown-JSON-field", rights: 4, want: 4, payload: []byte(`{"unknown":true}`)},
		{name: "nonce-in-unknown-JSON-key", rights: 4, want: 4, payload: []byte(`{"disposable":true}`)},
		{name: "trailing-JSON", rights: 4, want: 4, payload: append(append([]byte(nil), encoded...), []byte(" {}")...)},
		{name: "payload-bound", rights: 4, want: 4, payload: append(append([]byte(nil), encoded...), []byte(strings.Repeat(" ", 2048-len(encoded)))...), accept: true},
		{name: "truncated-payload", rights: 4, want: 4, payload: append(append([]byte(nil), encoded...), []byte(strings.Repeat(" ", 2049-len(encoded)))...)},
		{name: "unexpected-timestamp-control", rights: 4, want: 4, timestamp: true},
	} {
		t.Run(row.name, func(t *testing.T) {
			sender, receiver, err := assemblySocketPair()
			if err != nil {
				t.Fatal(err)
			}
			registrationRightsCleanup(t, sender)
			registrationRightsCleanup(t, receiver)
			if row.timestamp {
				if err := unix.SetsockoptInt(int(receiver.Fd()), unix.SOL_SOCKET, unix.SO_TIMESTAMPNS, 1); err != nil {
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
			if row.payload == nil && !row.timestamp && len(rights) <= 4 {
				err = sendAssemblyPacket(sender, frame, rights...)
			} else {
				payload := row.payload
				if payload == nil {
					payload = encoded
				}
				var ancillary []byte
				if len(rights) != 0 {
					ancillary = unix.UnixRights(rights...)
				}
				err = unix.Sendmsg(int(sender.Fd()), payload, ancillary, nil, 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			pid, uid := os.Getpid(), uint32(os.Geteuid())
			if row.wrongPID {
				pid++
			}
			if row.wrongUID {
				uid++
			}
			got, received, err := receiveAssemblyPacket(receiver, pid, uid, row.want)
			for _, file := range received {
				registrationRightsCleanup(t, file)
			}
			if row.accept {
				if err != nil || got != frame || len(received) != row.want {
					t.Fatalf("valid assembly packet refused: frame=%+v rights=%d err=%v", got, len(received), err)
				}
				for i, file := range received {
					flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFD, 0)
					if err != nil || flags&unix.FD_CLOEXEC == 0 {
						t.Fatalf("received right lacks CLOEXEC: flags=%#x err=%v", flags, err)
					}
					if _, err := file.Write([]byte{1}); err != nil {
						t.Fatal(err)
					}
					var marker [1]byte
					if n, err := readers[i].Read(marker[:]); err != nil || n != 1 || marker[0] != 1 {
						t.Fatalf("received pipe capability changed: n=%d err=%v", n, err)
					}
					if err := file.Close(); err != nil {
						t.Fatal(err)
					}
				}
			} else if err == nil || received != nil || got != (assemblyMessage{}) {
				t.Fatalf("invalid packet accepted or returned partial ownership: rights=%d err=%v", len(received), err)
			}
			if err != nil && strings.Contains(err.Error(), frame.Nonce) {
				t.Fatal("packet refusal exposed nonce")
			}
			for i, writer := range writers {
				if _, err := writer.Write([]byte{1}); err != nil {
					t.Fatalf("sender ownership lost: %v", err)
				}
				var marker [1]byte
				if n, err := readers[i].Read(marker[:]); err != nil || n != 1 || marker[0] != 1 {
					t.Fatalf("sender capability changed: n=%d err=%v", n, err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				poll := []unix.PollFd{{Fd: int32(readers[i].Fd()), Events: unix.POLLHUP}}
				if n, err := unix.Poll(poll, 1000); err != nil || n != 1 || poll[0].Revents != unix.POLLHUP {
					t.Fatalf("received capability leaked: n=%d events=%#x err=%v", n, poll[0].Revents, err)
				}
			}
		})
	}
}

func TestAssemblyPacketSender(t *testing.T) {
	if os.Getenv("ZERB_ASSEMBLY_PACKET_CHILD") == "send" {
		control := os.NewFile(3, "assembly-sender-child")
		registrationRightsCleanup(t, control)
		unix.CloseOnExec(3)
		if err := sendAssemblyPacket(control, assemblyMessage{Phase: "actual-sender"}); err != nil {
			t.Fatal(err)
		}
		return
	}
	parent, child, err := assemblySocketPair()
	if err != nil {
		t.Fatal(err)
	}
	registrationRightsCleanup(t, parent)
	registrationRightsCleanup(t, child)
	cmd := exec.Command(os.Args[0], "-test.run=^TestAssemblyPacketSender$")
	cmd.Env = append(isolatedEnv(t.TempDir(), ""), "ZERB_ASSEMBLY_PACKET_CHILD=send", "GORACE=atexit_sleep_ms=0")
	cmd.ExtraFiles = []*os.File{child}
	var output custodyOutput
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	frame, rights, receiveErr := receiveAssemblyPacket(parent, cmd.Process.Pid, uint32(os.Geteuid()), 0)
	waitErr := cmd.Wait()
	if receiveErr != nil || waitErr != nil || frame.Phase != "actual-sender" || len(rights) != 0 {
		t.Fatalf("actual writer differs from socket creator: frame=%+v rights=%d receive=%v wait=%v\n%s", frame, len(rights), receiveErr, waitErr, output.Bytes())
	}
}

func TestAssemblyPacketPreflight(t *testing.T) {
	for _, row := range []struct {
		name        string
		option, bad int
		want, pid   int
		wrongType   bool
		wrongDomain bool
	}{
		{name: "passcred-disabled", option: unix.SO_PASSCRED},
		{name: "automatic-pidfd", option: unix.SO_PASSPIDFD, bad: 1},
		{name: "negative-right-count", want: -1},
		{name: "excessive-right-count", want: 5},
		{name: "unknown-sender", pid: -1},
		{name: "stream-socket", wrongType: true},
		{name: "nonunix-socket", wrongDomain: true},
	} {
		t.Run(row.name, func(t *testing.T) {
			sender, receiver, err := assemblySocketPair()
			if err != nil {
				t.Fatal(err)
			}
			registrationRightsCleanup(t, sender)
			registrationRightsCleanup(t, receiver)
			for _, file := range []*os.File{sender, receiver} {
				flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFD, 0)
				if err != nil || flags&unix.FD_CLOEXEC == 0 {
					t.Fatalf("assembly channel lacks CLOEXEC: flags=%#x err=%v", flags, err)
				}
			}
			if row.wrongType || row.wrongDomain {
				domain, kind := unix.AF_UNIX, unix.SOCK_STREAM
				if row.wrongDomain {
					domain, kind = unix.AF_INET, unix.SOCK_DGRAM
				}
				fd, err := unix.Socket(domain, kind|unix.SOCK_CLOEXEC, 0)
				if err != nil {
					t.Fatal(err)
				}
				receiver = os.NewFile(uintptr(fd), "invalid-assembly-channel")
				registrationRightsCleanup(t, receiver)
			} else if row.option != 0 {
				if err := unix.SetsockoptInt(int(receiver.Fd()), unix.SOL_SOCKET, row.option, row.bad); err != nil {
					t.Fatal(err)
				}
			}
			if err := sendAssemblyPacket(sender, assemblyMessage{Phase: "still-queued"}); err != nil {
				t.Fatal(err)
			}
			pid := os.Getpid()
			if row.pid != 0 {
				pid = row.pid
			}
			frame, rights, err := receiveAssemblyPacket(receiver, pid, uint32(os.Geteuid()), row.want)
			if err == nil || rights != nil || frame != (assemblyMessage{}) {
				t.Fatalf("invalid preflight accepted: rights=%d err=%v", len(rights), err)
			}
			if row.wrongType || row.wrongDomain {
				return
			}
			if row.option != 0 {
				good := 0
				if row.option == unix.SO_PASSCRED {
					good = 1
				}
				if err := unix.SetsockoptInt(int(receiver.Fd()), unix.SOL_SOCKET, row.option, good); err != nil {
					t.Fatal(err)
				}
			}
			frame, rights, err = receiveAssemblyPacket(receiver, os.Getpid(), uint32(os.Geteuid()), 0)
			if err != nil || frame.Phase != "still-queued" || len(rights) != 0 {
				t.Fatalf("preflight consumed queued packet: frame=%+v rights=%d err=%v", frame, len(rights), err)
			}
		})
	}
}

func TestAssemblyPacketSendBounds(t *testing.T) {
	sender, receiver, err := assemblySocketPair()
	if err != nil {
		t.Fatal(err)
	}
	registrationRightsCleanup(t, sender)
	registrationRightsCleanup(t, receiver)
	if err := sendAssemblyPacket(sender, assemblyMessage{Nonce: strings.Repeat("x", 2048)}); err == nil {
		t.Fatal("oversized packet sent")
	}
	if err := sendAssemblyPacket(sender, assemblyMessage{}, 0, 0, 0, 0, 0); err == nil {
		t.Fatal("excessive rights sent")
	}
	if err := sendAssemblyPacket(sender, assemblyMessage{Phase: "after-refusal"}); err != nil {
		t.Fatal(err)
	}
	frame, rights, err := receiveAssemblyPacket(receiver, os.Getpid(), uint32(os.Geteuid()), 0)
	if err != nil || frame.Phase != "after-refusal" || len(rights) != 0 {
		t.Fatalf("sender refusal enqueued a packet: frame=%+v rights=%d err=%v", frame, len(rights), err)
	}
}
