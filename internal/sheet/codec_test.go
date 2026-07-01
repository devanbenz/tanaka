package sheet

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func sample() *model.Sheet {
	return &model.Sheet{
		Source: model.SheetSource{
			Title:  "My Paper",
			Origin: "https://example.com",
			Sections: []model.SheetSection{
				{Idx: 0, Title: "Intro", Markdown: "# hi", Study: &model.SheetStudy{
					Summary:     "s",
					KeyConcepts: []string{"a", "b"},
					Questions: []model.SheetQuestion{
						{Idx: 0, Kind: model.KindMCQ, Prompt: "pick", Options: []string{"x", "y"}, CorrectIndex: 1, Explanation: "because y"},
						{Idx: 1, Kind: model.KindFree, Prompt: "explain", Rubric: "mentions a"},
					},
				}},
				{Idx: 1, Title: "No quiz", Markdown: "body", Study: nil},
			},
		},
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, sample()); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Format != model.SheetFormat || got.Version != model.SheetVersion {
		t.Fatalf("envelope not set: %+v", got)
	}
	if got.Source.Title != "My Paper" || len(got.Source.Sections) != 2 {
		t.Fatalf("source not round-tripped: %+v", got.Source)
	}
	s0 := got.Source.Sections[0]
	if s0.Study == nil || len(s0.Study.Questions) != 2 || s0.Study.Questions[0].Options[1] != "y" {
		t.Fatalf("study not round-tripped: %+v", s0.Study)
	}
	if got.Source.Sections[1].Study != nil {
		t.Fatalf("expected nil study for section 1")
	}
}

func TestEncodeIsGzip(t *testing.T) {
	var buf bytes.Buffer
	if err := Encode(&buf, sample()); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := gzip.NewReader(bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("output is not gzip: %v", err)
	}
}

func TestDecodeRejectsBadGzip(t *testing.T) {
	if _, err := Decode(strings.NewReader("not gzip at all")); err == nil {
		t.Fatal("expected error for non-gzip input")
	}
}

func TestDecodeRejectsBadJSON(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte("{not json"))
	zw.Close()
	if _, err := Decode(&buf); err == nil {
		t.Fatal("expected error for bad json")
	}
}

func TestDecodeRejectsWrongFormat(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(`{"format":"nope","version":1}`))
	zw.Close()
	if _, err := Decode(&buf); err == nil {
		t.Fatal("expected error for wrong format")
	}
}

func TestDecodeRejectsUnsupportedVersion(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(`{"format":"tanaka.sheet","version":2}`))
	zw.Close()
	if _, err := Decode(&buf); err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestEncodeDoesNotMutateCaller(t *testing.T) {
	s := sample()
	s.Version = 99
	var buf bytes.Buffer
	if err := Encode(&buf, s); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if s.Version != 99 {
		t.Fatalf("Encode mutated caller: Version = %d, want 99", s.Version)
	}
}

func TestFilename(t *testing.T) {
	cases := map[string]string{
		"My Paper":                 "my-paper.tanaka",
		"Attention Is All You Need": "attention-is-all-you-need.tanaka",
		"  Spaces  &  Symbols!! ":  "spaces-symbols.tanaka",
		"":                         "sheet.tanaka",
	}
	for in, want := range cases {
		if got := Filename(in); got != want {
			t.Errorf("Filename(%q) = %q, want %q", in, got, want)
		}
	}
}
