package tui

import "testing"

func TestStageStepSwitchesLastfm(t *testing.T) {
	var got []string
	m := auxModel(t, nil, nil, Options{Lastfm: func(session string) {
		got = append(got, session)
	}})
	m.Update(stageStepMsg{step: 2, session: "abc"})
	if len(got) != 1 || got[0] != "abc" {
		t.Fatalf("Lastfm hook calls = %v, want [abc]", got)
	}
	if m.cfg.LastfmSessionKey != "abc" {
		t.Fatalf("LastfmSessionKey = %q, want abc", m.cfg.LastfmSessionKey)
	}

	m.Update(stageStepMsg{step: 2, err: "x"})
	if len(got) != 1 {
		t.Fatalf("a failed step called the Lastfm hook again: %v", got)
	}
}
