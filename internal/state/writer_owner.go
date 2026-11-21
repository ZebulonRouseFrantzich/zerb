package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const writerOwnerRecordLimit = 4096

var errWriterOwnerRecord = errors.New("invalid writer owner record")

func encodeWriterOwnerRecord(record writerOwner) ([]byte, error) {
	if !validWriterOwnerRecord(record) {
		return nil, errWriterOwnerRecord
	}
	data, err := json.Marshal(record)
	if err != nil || len(data) > writerOwnerRecordLimit {
		return nil, errWriterOwnerRecord
	}
	return data, nil
}

func decodeWriterOwnerRecord(data []byte) (writerOwner, error) {
	if len(data) == 0 || len(data) > writerOwnerRecordLimit {
		return writerOwner{}, errWriterOwnerRecord
	}
	var record writerOwner
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil {
		return writerOwner{}, errWriterOwnerRecord
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return writerOwner{}, errWriterOwnerRecord
	}
	canonical, err := encodeWriterOwnerRecord(record)
	if err != nil || !bytes.Equal(canonical, data) {
		return writerOwner{}, errWriterOwnerRecord
	}
	return record, nil
}

// ARCH-024/031: scalar shape alone proves no lease, liveness or retained history.
func validWriterOwnerRecord(record writerOwner) bool {
	if record.Schema != 1 || record.Domain != "zerb.writer-owner" || !replacementHex(record.Instance, 32) {
		return false
	}
	if record.PreviousToken != "" &&
		(!replacementHex(record.PreviousToken, 32) || record.PreviousToken == record.Instance) {
		return false
	}
	switch record.Status {
	case "held":
		if record.Generation == 0 || !replacementHex(record.Token, 32) || record.Token == record.Instance ||
			record.Process.PID <= 0 || record.Process.Start == 0 || record.Process.PIDNamespace == 0 ||
			record.PreviousToken == record.Token {
			return false
		}
		if record.Generation == 1 {
			return record.PreviousGeneration == 0 && record.PreviousToken == ""
		}
		return record.PreviousGeneration == record.Generation-1 &&
			(record.Generation > 2 || record.PreviousToken != "")
	case "released":
		if record.Token != "" || record.Process != (processIdentity{}) {
			return false
		}
		if record.Generation == 0 {
			return record.PreviousGeneration == 0 && record.PreviousToken == ""
		}
		return record.Generation >= 2 && record.PreviousGeneration == record.Generation-1 &&
			record.PreviousToken != ""
	default:
		return false
	}
}

// ARCH-024: this correspondence grants no graph, process or writer authority.
func currentMatchesWriter(current currentRecord, writer writerOwner) bool {
	if !validCurrentRecord(current) || !validWriterOwnerRecord(writer) || current.Instance != writer.Instance {
		return false
	}
	if writer.Status == "held" {
		if current.WriterGeneration == writer.Generation {
			return current.WriterToken == writer.Token
		}
		if writer.PreviousToken == "" || current.WriterToken == writer.Token {
			return false
		}
	}
	if current.WriterGeneration >= writer.Generation {
		return false
	}
	if current.WriterGeneration == writer.PreviousGeneration {
		return current.WriterToken == writer.PreviousToken
	}
	return current.WriterGeneration < writer.PreviousGeneration && current.WriterToken != writer.PreviousToken
}
