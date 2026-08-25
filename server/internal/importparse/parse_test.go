package importparse

import "testing"

func TestParseFormats(t *testing.T) {
	text := "coralie_doloribushc@mail.com:2WnL3G6cJ\ncoralie_two@mail.com----p:ass:word\n# comment\n\nbadline\ncoralie_doloribushc@mail.com:dup\n"
	got := Parse(text)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2 %#v", len(got), got)
	}
	if got[0].Email != "coralie_doloribushc@mail.com" || got[0].Password != "2WnL3G6cJ" {
		t.Fatalf("first=%+v", got[0])
	}
	if got[1].Email != "coralie_two@mail.com" || got[1].Password != "p:ass:word" {
		t.Fatalf("second=%+v", got[1])
	}
}
