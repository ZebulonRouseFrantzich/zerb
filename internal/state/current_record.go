package state

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/ZebulonRouseFrantzich/zerb/internal/receipt"
)

// ARCH-024: 174 fixed bytes, four 32-byte identities, two 71-byte digests,
// and at most 20 decimal digits for a positive uint64 generation.
const currentRecordLimit = 464

var errCurrentRecord = errors.New("invalid current record")

// Scalar validity is not owner/journal correspondence or receipt completion.
type currentRecord struct {
	Schema           int            `json:"schema"`
	Domain           string         `json:"domain"`
	Status           string         `json:"status"`
	Instance         string         `json:"instance"`
	OperationID      string         `json:"operation_id"`
	OperationToken   string         `json:"operation_token"`
	WriterGeneration uint64         `json:"writer_generation"`
	WriterToken      string         `json:"writer_token"`
	Candidate        receipt.Digest `json:"candidate"`
	Receipt          receipt.Digest `json:"receipt"`
}

func encodeCurrentRecord(record currentRecord) ([]byte, error) {
	if !validCurrentRecord(record) {
		return nil, errCurrentRecord
	}
	data, err := json.Marshal(record)
	if err != nil || len(data) > currentRecordLimit {
		return nil, errCurrentRecord
	}
	return data, nil
}

func decodeCurrentRecord(data []byte) (currentRecord, error) {
	if len(data) == 0 || len(data) > currentRecordLimit {
		return currentRecord{}, errCurrentRecord
	}
	var record currentRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil {
		return currentRecord{}, errCurrentRecord
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return currentRecord{}, errCurrentRecord
	}
	canonical, err := encodeCurrentRecord(record)
	if err != nil || !bytes.Equal(canonical, data) {
		return currentRecord{}, errCurrentRecord
	}
	return record, nil
}

func validCurrentRecord(record currentRecord) bool {
	if record.Schema != 1 || record.Domain != "zerb.current" || record.Status != "stable" ||
		!replacementHex(record.Instance, 32) || !replacementHex(record.OperationID, 32) ||
		!replacementHex(record.OperationToken, 32) || !replacementHex(record.WriterToken, 32) ||
		record.WriterGeneration == 0 || record.OperationID == record.OperationToken ||
		record.OperationID == record.WriterToken || record.OperationToken == record.WriterToken {
		return false
	}
	if _, err := receipt.ParseDigest(string(record.Candidate)); err != nil {
		return false
	}
	_, err := receipt.ParseDigest(string(record.Receipt))
	return err == nil
}
