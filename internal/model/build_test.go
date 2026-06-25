package model

import "testing"

func TestBuildConstants(t *testing.T) {
	if LangRust != "rust" || LangGo != "go" || LangCPP != "cpp" || LangC != "c" || LangPython != "python" {
		t.Fatal("language constant values must match the spec strings")
	}
	if DiffGuided != "guided" || DiffSpecTests != "spec+tests" || DiffBlank != "blank-page" {
		t.Fatal("difficulty constant values must match the spec strings")
	}
}

func TestValidLanguageAndDifficulty(t *testing.T) {
	for _, l := range []string{"rust", "go", "cpp", "c", "python"} {
		if !ValidLanguage(l) {
			t.Fatalf("ValidLanguage(%q) = false", l)
		}
	}
	if ValidLanguage("haskell") {
		t.Fatal("ValidLanguage(haskell) should be false")
	}
	for _, d := range []string{"guided", "spec+tests", "blank-page"} {
		if !ValidDifficulty(d) {
			t.Fatalf("ValidDifficulty(%q) = false", d)
		}
	}
	if ValidDifficulty("impossible") {
		t.Fatal("ValidDifficulty(impossible) should be false")
	}
}

func TestBuildHoldsSteps(t *testing.T) {
	b := Build{ID: "b1", SourceID: "s1", Language: LangGo, Difficulty: DiffSpecTests, Workspace: "/tmp/x",
		Steps: []BuildStep{{ID: "st1", BuildID: "b1", Idx: 0, Goal: "do it", Status: StatusUnlocked,
			Files: []BuildFile{{Path: "main.go", Content: "package main"}}}}}
	if len(b.Steps) != 1 || b.Steps[0].Files[0].Path != "main.go" {
		t.Fatalf("unexpected: %+v", b)
	}
}
