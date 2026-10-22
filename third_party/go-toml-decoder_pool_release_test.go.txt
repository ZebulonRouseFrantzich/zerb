package toml

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2/unstable"
)

type releaseProfile struct {
	ID   string `toml:"id"`
	Kind string `toml:"kind"`
	Path string `toml:"path"`
}

type releaseDocument struct {
	Schema   int               `toml:"schema"`
	Title    string            `toml:"title"`
	Numbers  []int64           `toml:"numbers"`
	Metadata map[string]any    `toml:"metadata"`
	Settings map[string]string `toml:"settings"`
	Profiles []releaseProfile  `toml:"profiles"`
	Objects  map[string]struct {
		Value string `toml:"value"`
	} `toml:"objects"`
}

const releaseRichDocument = `schema=1
title="retained-\u0061"
numbers=[1,2,3]
metadata={name='inline',enabled=true}
[settings]
path='data/settings'
[[profiles]]
id='first'
kind='embedded'
path='profiles/first'
[[profiles]]
id='second'
kind='gitlink'
path='profiles/second'
[objects.first]
value='first object'
[objects.second]
value='second object'
`

type releaseCaptures struct {
	Inline unstable.RawMessage   `toml:"inline"`
	Table  unstable.RawMessage   `toml:"table"`
	Series []unstable.RawMessage `toml:"series"`
}

const releaseCaptureDocument = `inline={value='inline capture'}
[table]
value='table capture'
[[series]]
value='first capture'
[[series]]
value='second capture'
`

// These tests stay serial: inspect the shell immediately after its one Put,
// before any later decode can acquire it. No pool identity or GC is assumed.
func decodeAndCheckRelease(t *testing.T, source []byte, target any, strictMode, captures bool) error {
	t.Helper()
	d := getDecoder(strictMode, captures)
	err := d.unmarshal(source, target)
	putDecoder(d)
	if !reflect.ValueOf(d).Elem().IsZero() {
		t.Fatal("released decoder retains state or backing storage")
	}
	return err
}

func TestDecoderPoolReleaseSuccessPaths(t *testing.T) {
	var rich releaseDocument
	if err := decodeAndCheckRelease(t, []byte(releaseRichDocument), &rich, true, false); err != nil {
		t.Fatal(err)
	}
	if rich.Title != "retained-a" || len(rich.Profiles) != 2 || len(rich.Objects) != 2 || rich.Metadata["name"] != "inline" {
		t.Fatal("rich decode lost model data on release")
	}
	var generic map[string]any
	if err := decodeAndCheckRelease(t, []byte(releaseRichDocument), &generic, false, false); err != nil {
		t.Fatal(err)
	}
	if generic["title"] != "retained-a" {
		t.Fatal("generic model lost data on release")
	}
	var captured releaseCaptures
	if err := decodeAndCheckRelease(t, []byte(releaseCaptureDocument), &captured, true, true); err != nil {
		t.Fatal(err)
	}
	if len(captured.Series) != 2 || !bytes.Contains(captured.Inline, []byte("inline capture")) || !bytes.Contains(captured.Table, []byte("table capture")) {
		t.Fatal("raw capture lost data on release")
	}
	var root unstable.RawMessage
	if err := decodeAndCheckRelease(t, []byte(releaseCaptureDocument), &root, false, true); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(root, []byte("second capture")) {
		t.Fatal("root capture lost data on release")
	}
}

func TestDecoderPoolReleaseErrorPaths(t *testing.T) {
	cases := []struct {
		name, source string
		target       any
		strict       bool
	}{
		{"malformed", "title='valid'\nnumbers=[1,\n", &releaseDocument{}, false},
		{"duplicate", "title='first'\ntitle='second'\n", &releaseDocument{}, false},
		{"overflow", "small=128\n", &struct {
			Small int8 `toml:"small"`
		}{}, false},
		{"integer-overflow", "number=9223372036854775808\n", &map[string]any{}, false},
		{"type", "numbers=['invalid']\n", &releaseDocument{}, false},
		{"strict-unknown", "title='valid'\nunknown='metadata'\n[unknown_table]\ninside='unlisted'\n", &releaseDocument{}, true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := decodeAndCheckRelease(t, []byte(test.source), test.target, test.strict, false)
			if err == nil {
				t.Fatal("invalid document succeeded")
			}
			if test.strict {
				var missing *StrictMissingError
				if !errors.As(err, &missing) || len(missing.Errors) < 2 {
					t.Fatalf("strict diagnostics missing after release: %v", err)
				}
			} else {
				var detail *DecodeError
				if !errors.As(err, &detail) {
					t.Fatalf("decode detail missing after release: %v", err)
				}
			}
		})
	}
}

type releaseDecodeErrorView struct {
	Message, Human string
	Row, Column    int
	Key            Key
}

type releaseErrorView struct {
	Message, Human string
	Details        []releaseDecodeErrorView
}

func snapshotReleaseError(err error) releaseErrorView {
	result := releaseErrorView{Message: err.Error()}
	var strict *StrictMissingError
	var detail *DecodeError
	if errors.As(err, &strict) {
		result.Human = strict.String()
		for index := range strict.Errors {
			result.Details = append(result.Details, snapshotReleaseDecodeError(&strict.Errors[index]))
		}
	} else if errors.As(err, &detail) {
		result.Human = detail.String()
		result.Details = append(result.Details, snapshotReleaseDecodeError(detail))
	}
	return result
}

func snapshotReleaseDecodeError(detail *DecodeError) releaseDecodeErrorView {
	row, column := detail.Position()
	return releaseDecodeErrorView{Message: detail.Error(), Human: detail.String(), Row: row, Column: column, Key: append(Key(nil), detail.Key()...)}
}

func snapshotReleaseModel(t *testing.T, model any) []byte {
	t.Helper()
	data, err := json.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func releaseHistoryDocument(profiles int) []byte {
	var document strings.Builder
	document.WriteString("schema=1\ntitle='history'\nprofiles=[")
	for index := 0; index < profiles; index++ {
		if index != 0 {
			document.WriteByte(',')
		}
		fmt.Fprintf(&document, "{id='p%d',kind='embedded',path='profiles/p%d'}", index, index)
	}
	document.WriteString("]\n")
	for document.Len() < 900_000 {
		remaining := min(80, 900_000-document.Len())
		document.WriteByte('#')
		for index := 0; index < remaining-2; index++ {
			document.WriteByte('x')
		}
		if remaining > 1 {
			document.WriteByte('\n')
		}
	}
	return []byte(document.String())
}

func TestDecoderPoolReleaseDecreasingLargeHistory(t *testing.T) {
	var held []releaseDocument
	var snapshots [][]byte
	var heldErrors []error
	var errorSnapshots []releaseErrorView
	for profiles := 256; profiles >= 248; profiles-- {
		var model releaseDocument
		if err := decodeAndCheckRelease(t, releaseHistoryDocument(profiles), &model, true, false); err != nil {
			t.Fatal(err)
		}
		if len(model.Profiles) != profiles {
			t.Fatalf("history profile count: %d", len(model.Profiles))
		}
		held = append(held, model)
		snapshots = append(snapshots, snapshotReleaseModel(t, model))
		var tiny struct {
			Schema int    `toml:"schema"`
			Label  string `toml:"label"`
		}
		if err := decodeAndCheckRelease(t, []byte("schema=1\nlabel='tiny'\n"), &tiny, true, false); err != nil || tiny.Schema != 1 || tiny.Label != "tiny" {
			t.Fatalf("tiny component: %#v, %v", tiny, err)
		}
		for _, strict := range []bool{false, true} {
			source := "title='a'\ntitle='b'\n"
			if strict {
				source = "title='known'\nmissing='first'\n[extra]\nvalue='second'\n"
			}
			err := decodeAndCheckRelease(t, []byte(source), &releaseDocument{}, strict, false)
			if err == nil {
				t.Fatal("history error fixture succeeded")
			}
			heldErrors = append(heldErrors, err)
			errorSnapshots = append(errorSnapshots, snapshotReleaseError(err))
		}
		for index, prior := range held {
			if !bytes.Equal(snapshotReleaseModel(t, prior), snapshots[index]) {
				t.Fatal("later decode changed retained history model")
			}
		}
		for index, prior := range heldErrors {
			if !reflect.DeepEqual(snapshotReleaseError(prior), errorSnapshots[index]) {
				t.Fatal("later decode changed retained diagnostic")
			}
		}
	}
}

func TestDecoderPoolReleasePublicResultsRemainStable(t *testing.T) {
	for _, streamed := range []bool{false, true} {
		t.Run(fmt.Sprintf("Decoder-%v", streamed), func(t *testing.T) {
			decode := func(source string, target any) error {
				if streamed {
					return NewDecoder(strings.NewReader(source)).Decode(target)
				}
				return Unmarshal([]byte(source), target)
			}
			var model releaseDocument
			if err := decode(releaseRichDocument, &model); err != nil {
				t.Fatal(err)
			}
			modelSnapshot := snapshotReleaseModel(t, model)
			var generic map[string]any
			if err := decode(releaseRichDocument, &generic); err != nil {
				t.Fatal(err)
			}
			genericSnapshot := snapshotReleaseModel(t, generic)
			var retained []error
			var errorSnapshots []releaseErrorView
			for _, source := range []string{"title='unterminated", "title='one'\ntitle='two'", "numbers=[9223372036854775808]"} {
				err := decode(source, &releaseDocument{})
				if err == nil {
					t.Fatal("public error fixture succeeded")
				}
				retained = append(retained, err)
				errorSnapshots = append(errorSnapshots, snapshotReleaseError(err))
			}
			var captured releaseCaptures
			var captureSnapshot []byte
			if streamed {
				if err := NewDecoder(strings.NewReader(releaseCaptureDocument)).EnableUnmarshalerInterface().Decode(&captured); err != nil {
					t.Fatal(err)
				}
				captureSnapshot = snapshotReleaseModel(t, captured)
				err := NewDecoder(strings.NewReader("title='known'\nunknown='first'\n[extra]\nvalue='second'\n")).DisallowUnknownFields().Decode(&releaseDocument{})
				if err == nil {
					t.Fatal("public strict error fixture succeeded")
				}
				retained = append(retained, err)
				errorSnapshots = append(errorSnapshots, snapshotReleaseError(err))
			}
			for profiles := 256; profiles >= 248; profiles-- {
				var later releaseDocument
				if err := decode(string(releaseHistoryDocument(profiles)), &later); err != nil {
					t.Fatal(err)
				}
				var tiny struct {
					Schema int `toml:"schema"`
				}
				if err := decode("schema=1", &tiny); err != nil {
					t.Fatal(err)
				}
			}
			if !bytes.Equal(snapshotReleaseModel(t, model), modelSnapshot) || !bytes.Equal(snapshotReleaseModel(t, generic), genericSnapshot) {
				t.Fatal("public decode result changed after later calls")
			}
			if streamed && !bytes.Equal(snapshotReleaseModel(t, captured), captureSnapshot) {
				t.Fatal("public capture changed after later calls")
			}
			for index, prior := range retained {
				if !reflect.DeepEqual(snapshotReleaseError(prior), errorSnapshots[index]) {
					t.Fatal("public diagnostic message/position/key/context changed after later calls")
				}
			}
		})
	}
}
