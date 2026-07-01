package obsidian

import "testing"

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		`a/b:c*d`:        "a b c d",
		`  spaced   out `: "spaced out",
		`x[[y]]#^z|<>"?\`: "x y z",
		`plain`:           "plain",
		``:                "",
		`///`:             "",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSourceName(t *testing.T) {
	if got := SourceName("My: Paper?"); got != "My Paper" {
		t.Errorf("SourceName = %q", got)
	}
	if got := SourceName("#^"); got != "Untitled" {
		t.Errorf("SourceName empty fallback = %q", got)
	}
}

func TestSectionName(t *testing.T) {
	if got := SectionName(0, "Intro"); got != "01 Intro" {
		t.Errorf("SectionName = %q", got)
	}
	if got := SectionName(11, "Deep/Dive"); got != "12 Deep Dive" {
		t.Errorf("SectionName = %q", got)
	}
	if got := SectionName(2, ""); got != "03 Untitled" {
		t.Errorf("SectionName empty = %q", got)
	}
}

func TestQuestionName(t *testing.T) {
	if got := QuestionName(0, "Intro", 1); got != "01 Intro Q2" {
		t.Errorf("QuestionName = %q", got)
	}
}
