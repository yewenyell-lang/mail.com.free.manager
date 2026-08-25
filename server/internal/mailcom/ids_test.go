package mailcom

import "testing"

func TestNormalizeMailID(t *testing.T) {
	cases := map[string]string{
		"../../Mail/abc123": "abc123",
		"/Mail/xyz":         "xyz",
		"plain-id":          "plain-id",
	}
	for input, want := range cases {
		if got := normalizeMailID(input); got != want {
			t.Fatalf("%s => %s want %s", input, got, want)
		}
	}
}

func TestParseSSESubmission(t *testing.T) {
	text := "event: success\ndata: ../../Mail/msg-1\n\n"
	got, err := parseMailSubmissionResult(text)
	if err != nil {
		t.Fatal(err)
	}
	if got.MessageID != "msg-1" {
		t.Fatalf("id=%s", got.MessageID)
	}
}
