package adapters

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-017/023: read, write, mount, archive, and network classes remain closed.
func TestAdapterTraceClassifications(t *testing.T) {
	root := t.TempDir()
	policy := tracePolicy{owned: root, proofRoot: root, writeRoot: filepath.Join(root, "manager"), cwd: root, authenticatedArchive: filepath.Join(root, "verified.tar.gz")}
	for _, malformed := range []bool{false, true} {
		prefix := filepath.Join(root, "trace")
		lines := []string{
			`101 execve("/nix/store/proof-manager", [], []) = 0`,
			`101 chdir("` + policy.writeRoot + `") = 0`,
			`101 clone(child_stack=NULL, flags=SIGCHLD) = 102`,
			`102 openat(AT_FDCWD, "file", O_WRONLY|O_CREAT, 0600) = 3`,
			`102 openat(AT_FDCWD, "` + policy.authenticatedArchive + `", O_RDONLY) = 4`,
			`102 openat(AT_FDCWD, "` + filepath.Join(root, "other.zip") + `", O_RDONLY) = 5`,
			`102 openat(AT_FDCWD, "/etc/proof-denied", O_WRONLY) = -1 EACCES (Permission denied)`,
			`102 connect(3, {sa_family=AF_INET, sin_port=htons(443), sin_addr=inet_addr("8.8.8.8")}, 16) = 0`,
			`102 mount("tmpfs", "` + policy.writeRoot + `", "tmpfs", 0, NULL) = -1 EPERM (Operation not permitted)`,
		}
		if malformed {
			lines = append(lines, "102 incomplete")
		}
		if err := os.WriteFile(prefix+".log", []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := parseAdapterTrace(prefix, policy)
		if (err != nil) != malformed || got.complete == malformed {
			t.Fatalf("closed classification: complete=%v error=%v", got.complete, err)
		}
		if got.authenticatedArchiveReads != 1 || got.forbiddenAccesses["unauthenticated-archive-input"] != 1 || got.forbiddenAccesses["manager-mount-control-attempt"] != 1 || got.writes["declared-manager-root"] != 1 || got.deniedWrites["outside-etc-openat"] != 1 || got.networkAttempts != 1 || got.outsideWrites != 0 {
			t.Fatalf("classification changed: %+v", got)
		}
		if _, err := os.Stat(prefix + ".log"); !os.IsNotExist(err) {
			t.Fatalf("raw trace not deleted: %v", err)
		}
	}
}
