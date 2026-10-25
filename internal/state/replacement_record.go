// Package state owns durable state contracts. This increment only recognizes
// metadata values; it does not admit a state graph or execute maintenance.
package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"

	"github.com/ZebulonRouseFrantzich/zerb/internal/safefs"
)

const replacementRecordLimit = 16384

var errReplacementRecord = errors.New("invalid replacement record")

type processIdentity struct {
	PID          int    `json:"pid"`
	Start        uint64 `json:"start"`
	PIDNamespace uint64 `json:"pid_namespace"`
}

type replacementAuthority struct {
	Kind           string `json:"kind"`
	Generation     uint64 `json:"generation"`
	Token          string `json:"token"`
	OperationID    string `json:"operation_id"`
	OperationToken string `json:"operation_token"`
}

type replacementIdentity struct {
	Device  uint64 `json:"device"`
	Inode   uint64 `json:"inode"`
	Owner   uint32 `json:"owner"`
	Mode    uint32 `json:"mode"`
	Links   uint64 `json:"links"`
	Size    int64  `json:"size"`
	MountID uint64 `json:"mount_id"`
}

type replacementFileFacts struct {
	Identity            replacementIdentity `json:"identity"`
	ModifiedSeconds     int64               `json:"modified_seconds"`
	ModifiedNanoseconds uint32              `json:"modified_nanoseconds"`
	ChangedSeconds      int64               `json:"changed_seconds"`
	ChangedNanoseconds  uint32              `json:"changed_nanoseconds"`
	SHA256              string              `json:"sha256"`
}

type replacementFiles struct {
	Parent    replacementIdentity  `json:"parent"`
	Target    string               `json:"target"`
	Temporary string               `json:"temporary"`
	Previous  replacementFileFacts `json:"previous"`
	Staged    replacementFileFacts `json:"staged"`
}

type replacementRecord struct {
	Schema    int                  `json:"schema"`
	Domain    string               `json:"domain"`
	Instance  string               `json:"instance"`
	ID        string               `json:"id"`
	Publisher processIdentity      `json:"publisher"`
	Authority replacementAuthority `json:"authority"`
	Files     replacementFiles     `json:"files"`
}

func encodeReplacementRecord(record replacementRecord) ([]byte, error) {
	// The closed, small scalar grammar bounds every string before Marshal.
	// This is a byte bound, not a cumulative Go allocation guarantee.
	if !validReplacementRecord(record) {
		return nil, errReplacementRecord
	}
	data, err := json.Marshal(record)
	if err != nil || len(data) > replacementRecordLimit {
		return nil, errReplacementRecord
	}
	return data, nil
}

func decodeReplacementRecord(data []byte) (replacementRecord, error) {
	if len(data) == 0 || len(data) > replacementRecordLimit {
		return replacementRecord{}, errReplacementRecord
	}
	var record replacementRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil {
		return replacementRecord{}, errReplacementRecord
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return replacementRecord{}, errReplacementRecord
	}
	canonical, err := encodeReplacementRecord(record)
	if err != nil || !bytes.Equal(canonical, data) {
		return replacementRecord{}, errReplacementRecord
	}
	return record, nil
}

func replacementHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for i := range value {
		if (value[i] < '0' || value[i] > '9') && (value[i] < 'a' || value[i] > 'f') {
			return false
		}
	}
	return true
}

func validReplacementRecord(record replacementRecord) bool {
	if record.Schema != 1 || record.Domain != "zerb.replace-witness" ||
		!replacementHex(record.Instance, 32) || !replacementHex(record.ID, 32) ||
		record.Publisher.PID <= 0 || record.Publisher.Start == 0 || record.Publisher.PIDNamespace == 0 {
		return false
	}
	a := record.Authority
	target, valid := replacementAuthorityTarget(a)
	if !valid || record.ID == record.Instance || record.ID == a.Token || record.ID == a.OperationID || record.ID == a.OperationToken ||
		record.Files.Target != target || record.Files.Temporary != ".replace-"+record.ID {
		return false
	}
	f := record.Files
	p, old, stage := f.Parent, f.Previous.Identity, f.Staged.Identity
	return validReplacementParent(p) && validReplacementFile(f.Previous, 1) && validReplacementFile(f.Staged, 0) &&
		old.Device == p.Device && stage.Device == p.Device && old.MountID == p.MountID && stage.MountID == p.MountID &&
		old.Owner == p.Owner && stage.Owner == p.Owner && old.Inode != stage.Inode && old.Inode != p.Inode && stage.Inode != p.Inode
}

func replacementAuthorityTarget(a replacementAuthority) (string, bool) {
	operation := false
	target := ""
	switch a.Kind {
	case "initialization":
		target = "root.json"
	case "writer-acquire", "writer-transfer", "writer-release":
		target = "writer-owner.json"
	case "journal-phase", "journal-transfer":
		operation = true
	case "current-commit":
		operation = true
		target = "current.json"
	default:
		return "", false
	}
	if a.Kind == "writer-acquire" {
		if a.Token != "" {
			return "", false
		}
	} else if !replacementHex(a.Token, 32) {
		return "", false
	}
	if a.Kind != "initialization" && a.Kind != "writer-acquire" && a.Generation == 0 {
		return "", false
	}
	if (a.Kind == "writer-acquire" || a.Kind == "writer-transfer" || a.Kind == "writer-release" || a.Kind == "journal-transfer") && a.Generation == math.MaxUint64 {
		return "", false
	}
	if operation {
		if !replacementHex(a.OperationID, 32) || !replacementHex(a.OperationToken, 32) ||
			a.OperationID == a.OperationToken || a.OperationID == a.Token || a.OperationToken == a.Token {
			return "", false
		}
	} else if a.OperationID != "" || a.OperationToken != "" {
		return "", false
	}
	if target == "" {
		target = a.OperationID + ".json"
	}
	return target, true
}

func validReplacementParent(id replacementIdentity) bool {
	return id.Inode != 0 && id.MountID != 0 && id.Mode == 0o040700 && id.Links != 0 && id.Size >= 0
}

func validReplacementFile(f replacementFileFacts, links uint64) bool {
	id := f.Identity
	return id.Inode != 0 && id.MountID != 0 && id.Mode == 0o100600 && id.Links == links &&
		id.Size >= 0 && id.Size < int64(int(^uint(0)>>1)) &&
		f.ModifiedNanoseconds < 1_000_000_000 && f.ChangedNanoseconds < 1_000_000_000 && replacementHex(f.SHA256, 64)
}

func replacementWireIdentity(id safefs.Identity) replacementIdentity {
	return replacementIdentity{id.Device, id.Inode, id.Owner, id.Mode, id.Links, id.Size, id.MountID}
}

func replacementWireFile(f safefs.FileFacts) replacementFileFacts {
	return replacementFileFacts{replacementWireIdentity(f.Identity), f.ModifiedSeconds, f.ModifiedNanoseconds,
		f.ChangedSeconds, f.ChangedNanoseconds, f.SHA256}
}

func replacementWireFiles(f safefs.ReplacementFacts) replacementFiles {
	return replacementFiles{replacementWireIdentity(f.Parent), f.Target, f.Temporary,
		replacementWireFile(f.Previous), replacementWireFile(f.Staged)}
}

// The eventual state owner supplies this comparison separately after admission
// and graph validation. Matching it does not authenticate historical authorship.
type replacementExpectation struct {
	instance  string
	id        string
	publisher processIdentity
	authority replacementAuthority
	files     safefs.ReplacementFacts
}

type replacementObservationState uint8

const (
	replacementUnobserved replacementObservationState = iota
	replacementAbsent
	replacementPresent
	replacementObservationFailed
)

type replacementFileObservation struct {
	state replacementObservationState
	facts safefs.FileFacts
}

type replacementObservations struct {
	parentState replacementObservationState
	parent      safefs.Identity
	target      replacementFileObservation
	temporary   replacementFileObservation
}

type replacementPrefix uint8

const (
	replacementBad replacementPrefix = iota
	retireRecord
	removeTempThenRecord
	verifyThenRetire
)

// recognizeReplacementPrefix describes only filesystem shape. It grants no
// authority and performs no I/O, process proof, revalidation or cleanup.
func recognizeReplacementPrefix(record *replacementRecord, expected replacementExpectation, observed replacementObservations) replacementPrefix {
	want := replacementRecord{1, "zerb.replace-witness", expected.instance, expected.id,
		expected.publisher, expected.authority, replacementWireFiles(expected.files)}
	if record == nil || !validReplacementRecord(want) || *record != want || observed.parentState != replacementPresent {
		return replacementBad
	}
	parent := replacementWireIdentity(observed.parent)
	if !validReplacementParent(parent) {
		return replacementBad
	}
	// Legitimate child publication changes directory size and link count.
	parent.Links, parent.Size = want.Files.Parent.Links, want.Files.Parent.Size
	if parent != want.Files.Parent || observed.target.state != replacementPresent {
		return replacementBad
	}
	target := replacementWireFile(observed.target.facts)
	tempAbsent := observed.temporary.state == replacementAbsent && observed.temporary.facts == (safefs.FileFacts{})
	if target == want.Files.Previous {
		if tempAbsent {
			return retireRecord
		}
		if observed.temporary.state == replacementPresent && replacementObservedStage(observed.temporary.facts, want.Files.Staged) {
			return removeTempThenRecord
		}
	} else if tempAbsent && replacementObservedStage(observed.target.facts, want.Files.Staged) {
		return verifyThenRetire
	}
	return replacementBad
}

func replacementObservedStage(observed safefs.FileFacts, staged replacementFileFacts) bool {
	f := replacementWireFile(observed)
	if !validReplacementFile(f, 1) {
		return false
	}
	// Linking/renaming can refresh ctime, but not original mtime or other facts.
	staged.Identity.Links = 1
	staged.ChangedSeconds, staged.ChangedNanoseconds = f.ChangedSeconds, f.ChangedNanoseconds
	return f == staged
}
