package git

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// VAL-007/009/017: inherited cwd, endpoints, escapes, and unknown records fail closed.
func TestGitTraceClassifications(t *testing.T) {
	root := t.TempDir()
	manager := "/nix/store/proof-git"
	prefix := filepath.Join(root, "trace")
	parent := `execve("` + manager + `", [], []) = 0` + "\n" + `chdir("` + filepath.Join(root, "staging") + `") = 0` + "\n" + `clone(child_stack=NULL, flags=SIGCHLD) = 102` + "\n"
	child := strings.Join([]string{
		`openat(AT_FDCWD, "object", O_WRONLY|O_CREAT, 0600) = 3`,
		`openat(AT_FDCWD, "` + filepath.Join(root, "home", ".gitconfig") + `", O_RDONLY) = 4`,
		`connect(3, {sa_family=AF_INET, sin_port=htons(443), sin_addr=inet_addr("8.8.8.8")}, 16) = 0`,
		`unshare(CLONE_NEWNS) = 0`,
		`openat(AT_FDCWD, "unterminated`,
	}, "\n") + "\n"
	for suffix, data := range map[string]string{".101": parent, ".102": child} {
		if err := os.WriteFile(prefix+suffix, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := parseTrace(prefix, root, manager, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.complete || got.unknownEventClasses["malformed-record"] != 1 || got.forbiddenAccesses != 2 || got.configurationAccesses != 1 || len(got.network) != 1 || got.network[0].port != 443 || len(got.writes) != 1 || got.writes[0] != "owned-path" || got.unexpectedReads != 0 {
		t.Fatalf("classification changed: %+v", got)
	}
	r := runner{git: manager}
	if err := r.validateTraceObservation("fixture", got); !errors.Is(err, errTraceViolation) {
		t.Fatalf("unknown/forbidden trace admitted: %v", err)
	}
	for _, suffix := range []string{".101", ".102"} {
		if _, err := os.Stat(prefix + suffix); !os.IsNotExist(err) {
			t.Fatalf("raw trace not deleted: %v", err)
		}
	}
}
