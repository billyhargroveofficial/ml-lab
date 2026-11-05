package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExamplePackIsValid(t *testing.T) {
	c, status := LoadPacks("../examples/packs")
	if len(status) == 0 || !status[0].Loaded {
		t.Fatalf("example pack did not load: %+v", status)
	}
	if len(c.Ideas) == 0 || !c.HasTrack("softmax") {
		t.Fatal("example pack has no ideas or no track")
	}
}

// Every pack field has a note, so the generated docs and schema are complete.
func TestFieldNotesCoverPackStructs(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(PackFile{}), reflect.TypeOf(PackTopic{}), reflect.TypeOf(PackIdea{}), reflect.TypeOf(PackLink{}), reflect.TypeOf(Question{})} {
		for i := 0; i < typ.NumField(); i++ {
			tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			if fieldNotes[typ.Name()+"."+tag] == "" {
				t.Errorf("no fieldNotes entry for %s.%s", typ.Name(), tag)
			}
		}
	}
}

func TestDocsAreFresh(t *testing.T) {
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	_ = os.Chdir("..")
	if docs(true) != 0 {
		t.Fatal("generated docs are stale: run ./fire docs")
	}
}

// writePack stores a two-idea pack whose links can be bent per test.
func writePack(t *testing.T, root, id string, mutate func(p *PackFile)) {
	raw, err := os.ReadFile("../examples/packs/softmax/pack.json")
	if err != nil {
		t.Fatal(err)
	}
	var p PackFile
	_ = json.Unmarshal(raw, &p)
	p.ID = id
	for ti := range p.Topics {
		for ii := range p.Topics[ti].Ideas {
			idea := &p.Topics[ti].Ideas[ii]
			if idea.LessonFile != "" {
				b, _ := os.ReadFile(filepath.Join("../examples/packs/softmax", idea.LessonFile))
				idea.Lesson, idea.LessonFile = string(b), ""
			}
			for k := range idea.Requires {
				idea.Requires[k].Code = strings.Replace(idea.Requires[k].Code, "softmax.", id+".", 1)
			}
		}
	}
	if mutate != nil {
		mutate(&p)
	}
	dir := filepath.Join(root, id)
	_ = os.MkdirAll(dir, 0o755)
	out, _ := json.Marshal(p)
	if err := os.WriteFile(filepath.Join(dir, "pack.json"), out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBrokenPacksAreReported(t *testing.T) {
	root := t.TempDir()
	writePack(t, root, "good", nil)
	writePack(t, root, "dangling", func(p *PackFile) {
		p.Topics[0].Ideas[0].Uses = []PackLink{{Code: "good.softmax/normalize-exb", W: 0.5}}
	})
	writePack(t, root, "cyclic", func(p *PackFile) {
		p.Topics[0].Ideas[0].Requires = []PackLink{{Code: "cyclic.softmax/temperature", W: 0.5}}
	})
	writePack(t, root, "badmath", func(p *PackFile) {
		p.Topics[0].Ideas[1].Questions[0].Question = "Чему равно $x \f$?"
	})
	c, status := LoadPacks(root)
	got := map[string]PackStatus{}
	for _, s := range status {
		got[s.ID] = s
	}
	if !got["good"].Loaded {
		t.Fatalf("good pack failed: %v", got["good"].Errors)
	}
	for _, id := range []string{"dangling", "cyclic", "badmath"} {
		if got[id].Loaded || len(got[id].Errors) == 0 {
			t.Errorf("%s should fail", id)
		}
	}
	if e := strings.Join(got["dangling"].Errors, " "); !strings.Contains(e, "did you mean good.softmax/normalize-exp") {
		t.Errorf("no suggestion: %s", e)
	}
	if e := strings.Join(got["cyclic"].Errors, " "); !strings.Contains(e, "cycle") {
		t.Errorf("cycle not reported: %s", e)
	}
	if len(c.Packs) != 1 {
		t.Errorf("only the good pack should load, got %d", len(c.Packs))
	}
}
