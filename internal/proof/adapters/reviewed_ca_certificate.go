package adapters

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"math/big"
)

type reviewedCertificate struct {
	TBS       reviewedTBSCertificate
	Algorithm pkix.AlgorithmIdentifier
	Signature asn1.BitString
}

type reviewedTBSCertificate struct {
	Version   int `asn1:"optional,explicit,default:0,tag:0"`
	Serial    *big.Int
	Algorithm pkix.AlgorithmIdentifier
	Issuer    []reviewedAttributeSET
	Validity  struct{ Before, After asn1.RawValue }
	Subject   []reviewedAttributeSET
	PublicKey struct {
		Algorithm pkix.AlgorithmIdentifier
		Key       asn1.BitString
	}
	IssuerID   asn1.BitString   `asn1:"optional,tag:1"`
	SubjectID  asn1.BitString   `asn1:"optional,tag:2"`
	Extensions []pkix.Extension `asn1:"omitempty,optional,explicit,tag:3"`
}

type reviewedAttributeSET []struct {
	Type  asn1.ObjectIdentifier
	Value asn1.RawValue
}

// Keep the proof's complete-envelope check independent of production parsing.
func parseReviewedCertificate(der []byte) (*x509.Certificate, error) {
	invalid := errors.New("fixed CA bundle contains invalid certificate DER")
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, invalid
	}
	var envelope reviewedCertificate
	rest, err := asn1.Unmarshal(der, &envelope)
	if err != nil || len(rest) != 0 {
		return nil, invalid
	}
	// Go 1.24 skips some nested SEQUENCE tails; no RawContent may hide them here.
	encoded, err := asn1.Marshal(envelope)
	if err != nil || !bytes.Equal(encoded, der) {
		return nil, invalid
	}
	return certificate, nil
}
