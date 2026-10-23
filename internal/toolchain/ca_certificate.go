package toolchain

import (
	"bytes"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
)

type caCertificate struct {
	TBS       caTBSCertificate
	Algorithm pkix.AlgorithmIdentifier
	Signature asn1.BitString
}

type caTBSCertificate struct {
	Version   int `asn1:"optional,explicit,default:0,tag:0"`
	Serial    *big.Int
	Algorithm pkix.AlgorithmIdentifier
	Issuer    []caAttributeSET
	Validity  struct{ Before, After asn1.RawValue }
	Subject   []caAttributeSET
	PublicKey struct {
		Algorithm pkix.AlgorithmIdentifier
		Key       asn1.BitString
	}
	IssuerID   asn1.BitString   `asn1:"optional,tag:1"`
	SubjectID  asn1.BitString   `asn1:"optional,tag:2"`
	Extensions []pkix.Extension `asn1:"omitempty,optional,explicit,tag:3"`
}

type caAttributeSET []struct {
	Type  asn1.ObjectIdentifier
	Value asn1.RawValue
}

func parseCACertificate(der []byte) (*x509.Certificate, error) {
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, errFixedCA
	}
	var envelope caCertificate
	rest, err := asn1.Unmarshal(der, &envelope)
	if err != nil || len(rest) != 0 {
		return nil, errFixedCA
	}
	// Go 1.24 skips some nested SEQUENCE tails; no RawContent may hide them here.
	encoded, err := asn1.Marshal(envelope)
	if err != nil || !bytes.Equal(encoded, der) {
		return nil, errFixedCA
	}
	return certificate, nil
}
