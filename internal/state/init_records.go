package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const initRecordLimit = 4096

var errInitRecord = errors.New("invalid initialization record")

type rootRecord struct {
	Schema     int             `json:"schema"`
	Domain     string          `json:"domain"`
	Status     string          `json:"status"`
	Instance   string          `json:"instance"`
	Token      string          `json:"token"`
	Process    processIdentity `json:"process"`
	Generation uint64          `json:"generation"`
}

type namespaceOwner struct {
	Schema   int    `json:"schema"`
	Domain   string `json:"domain"`
	Instance string `json:"instance"`
}

type writerOwner struct {
	Schema             int             `json:"schema"`
	Domain             string          `json:"domain"`
	Status             string          `json:"status"`
	Instance           string          `json:"instance"`
	Generation         uint64          `json:"generation"`
	Token              string          `json:"token,omitempty"`
	Process            processIdentity `json:"process,omitempty"`
	PreviousGeneration uint64          `json:"previous_generation"`
	PreviousToken      string          `json:"previous_token,omitempty"`
}

func encodeRootRecord(record rootRecord) ([]byte, error) {
	if !validRootRecord(record) {
		return nil, errInitRecord
	}
	data, err := json.Marshal(record)
	if err != nil || len(data) > initRecordLimit {
		return nil, errInitRecord
	}
	return data, nil
}

func decodeRootRecord(data []byte) (rootRecord, error) {
	if len(data) == 0 || len(data) > initRecordLimit {
		return rootRecord{}, errInitRecord
	}
	var record rootRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil {
		return rootRecord{}, errInitRecord
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return rootRecord{}, errInitRecord
	}
	canonical, err := encodeRootRecord(record)
	if err != nil || !bytes.Equal(canonical, data) {
		return rootRecord{}, errInitRecord
	}
	return record, nil
}

func encodeNamespaceOwner(record namespaceOwner) ([]byte, error) {
	if !validNamespaceOwner(record) {
		return nil, errInitRecord
	}
	data, err := json.Marshal(record)
	if err != nil || len(data) > initRecordLimit {
		return nil, errInitRecord
	}
	return data, nil
}

func decodeNamespaceOwner(data []byte) (namespaceOwner, error) {
	if len(data) == 0 || len(data) > initRecordLimit {
		return namespaceOwner{}, errInitRecord
	}
	var record namespaceOwner
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil {
		return namespaceOwner{}, errInitRecord
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return namespaceOwner{}, errInitRecord
	}
	canonical, err := encodeNamespaceOwner(record)
	if err != nil || !bytes.Equal(canonical, data) {
		return namespaceOwner{}, errInitRecord
	}
	return record, nil
}

func encodeWriterOwner(record writerOwner) ([]byte, error) {
	if !validWriterOwner(record) {
		return nil, errInitRecord
	}
	data, err := encodeWriterOwnerRecord(record)
	if err != nil {
		return nil, errInitRecord
	}
	return data, nil
}

func decodeWriterOwner(data []byte) (writerOwner, error) {
	record, err := decodeWriterOwnerRecord(data)
	if err != nil || !validWriterOwner(record) {
		return writerOwner{}, errInitRecord
	}
	return record, nil
}

// ARCH-017/024/031 admit only this FreshInit subset, not takeover or recovery.
func validRootRecord(record rootRecord) bool {
	if record.Schema != 1 || record.Domain != "zerb.root" ||
		!replacementHex(record.Instance, 32) || record.Generation != 0 {
		return false
	}
	switch record.Status {
	case "initializing":
		return replacementHex(record.Token, 32) && record.Token != record.Instance &&
			record.Process.PID > 0 && record.Process.Start > 0 && record.Process.PIDNamespace > 0
	case "initialized":
		return record.Token == "" && record.Process == (processIdentity{})
	default:
		return false
	}
}

func validNamespaceOwner(record namespaceOwner) bool {
	return record.Schema == 1 && (record.Domain == "zerb.data-owner" || record.Domain == "zerb.cache-owner") &&
		replacementHex(record.Instance, 32)
}

func validWriterOwner(record writerOwner) bool {
	if record.Schema != 1 || record.Domain != "zerb.writer-owner" || !replacementHex(record.Instance, 32) {
		return false
	}
	switch record.Status {
	case "held":
		return record.Generation == 1 && replacementHex(record.Token, 32) && record.Token != record.Instance &&
			record.Process.PID > 0 && record.Process.Start > 0 && record.Process.PIDNamespace > 0 &&
			record.PreviousGeneration == 0 && record.PreviousToken == ""
	case "released":
		if record.Token != "" || record.Process != (processIdentity{}) {
			return false
		}
		return (record.Generation == 0 && record.PreviousGeneration == 0 && record.PreviousToken == "") ||
			(record.Generation == 2 && record.PreviousGeneration == 1 &&
				replacementHex(record.PreviousToken, 32) && record.PreviousToken != record.Instance)
	default:
		return false
	}
}
