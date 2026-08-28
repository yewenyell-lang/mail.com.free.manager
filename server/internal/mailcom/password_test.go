package mailcom

import (
	"io"
	"net/url"
	"strings"
	"testing"

	http "github.com/bogdanfinn/fhttp"
)

type scriptedResp struct {
	status   int
	location string
	body     string
}

type fakeDoer struct {
	t        *testing.T
	calls    []*http.Request
	bodies   []string
	handlers []func(req *http.Request, body string) *scriptedResp
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	var raw []byte
	if req.Body != nil {
		raw, _ = io.ReadAll(req.Body)
	}
	body := string(raw)
	f.calls = append(f.calls, req)
	f.bodies = append(f.bodies, body)
	for _, h := range f.handlers {
		if resp := h(req, body); resp != nil {
			out := &http.Response{
				StatusCode: resp.status,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(resp.body)),
				Request:    req,
			}
			if resp.location != "" {
				out.Header.Set("Location", resp.location)
			}
			return out, nil
		}
	}
	f.t.Fatalf("unexpected request %s %s", req.Method, req.URL.String())
	return nil, nil
}

func (f *fakeDoer) on(substr string, method string, resp scriptedResp) {
	f.handlers = append(f.handlers, func(req *http.Request, _ string) *scriptedResp {
		if method != "" && req.Method != method {
			return nil
		}
		if !strings.Contains(req.URL.String(), substr) {
			return nil
		}
		copy := resp
		return &copy
	})
}

func TestExtractTokens(t *testing.T) {
	loc := "https://navigator-lxa.mail.com/login?edition=us&ott=bae38d0b-b95d-4774-a1dd-72d9d2045aca&auth_time=2026-08-10T05:58:15Z"
	if got := firstSub(ottRe, loc); got != "bae38d0b-b95d-4774-a1dd-72d9d2045aca" {
		t.Fatalf("ott=%s", got)
	}
	if got := firstSub(authRe, loc); got != "2026-08-10T05:58:15Z" {
		t.Fatalf("auth_time=%s", got)
	}
	sidLoc := "https://navigator-lxa.mail.com/?sid=748b653152e145826941c3a2020dc366f424ec2dc058980bdd638abae50ff170500094fd8f4518c30c7f48fbeb9d0ff5"
	if got := firstSub(sidRe, sidLoc); got != "748b653152e145826941c3a2020dc366f424ec2dc058980bdd638abae50ff170500094fd8f4518c30c7f48fbeb9d0ff5" {
		t.Fatalf("sid=%s", got)
	}
	overview := "https://account-lxa.mail.com/ciss/myAccountOverview?1&srttkn=db023f3b-b7cf-4255-8b48-f67bf6a7f652"
	if got := firstSub(srttknRe, overview); got != "db023f3b-b7cf-4255-8b48-f67bf6a7f652" {
		t.Fatalf("srttkn=%s", got)
	}
}

func TestValidatePasswordChange(t *testing.T) {
	if err := validatePasswordChange("", "a", "b"); err == nil {
		t.Fatal("empty email")
	}
	if err := validatePasswordChange("a@mail.com", "same", "same"); err == nil {
		t.Fatal("same password")
	}
	if err := validatePasswordChange("a@mail.com", "old", "Newpass1"); err == nil {
		t.Fatal("short password")
	}
	if err := validatePasswordChange("a@mail.com", "old", "Newpass12abc"); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordChangeFormFields(t *testing.T) {
	form := passwordChangeForm("user@mail.com", "Old#1", "Newpass1")
	want := map[string]string{
		"editPanel:username": "user@mail.com",
		"editPanel:currentPasswordPanel:topWrapper:inputWrapper:input":        "Old#1",
		"editPanel:newPasswordFieldPanel:topWrapper:inputWrapper:input":       "Newpass1",
		"editPanel:retypeNewPasswordFieldPanel:topWrapper:inputWrapper:input": "Newpass1",
	}
	for key, value := range want {
		if form.Get(key) != value {
			t.Fatalf("%s=%q want %q", key, form.Get(key), value)
		}
	}
}

func TestExtractFeedback(t *testing.T) {
	html := `<ul class="feedbackPanel"><li class="feedbackPanelERROR">Password does not meet requirements</li></ul>`
	got := extractFeedback(html)
	if !strings.Contains(got, "Password does not meet requirements") {
		t.Fatalf("feedback=%q", got)
	}
}

func TestChangePasswordSuccessContract(t *testing.T) {
	fake := &fakeDoer{t: t}
	const ott = "bae38d0b-b95d-4774-a1dd-72d9d2045aca"
	const sid = "748b653152e145826941c3a2020dc366f424ec2dc058980bdd638abae50ff170"
	const srttkn = "db023f3b-b7cf-4255-8b48-f67bf6a7f652"
	fake.on("login.mail.com/login", http.MethodPost, scriptedResp{
		status:   303,
		location: "https://navigator-lxa.mail.com/login?edition=us&ott=" + ott + "&auth_time=2026-08-10T05:58:15Z",
	})
	fake.on("navigator-lxa.mail.com/login?", http.MethodGet, scriptedResp{status: 200, body: "<html/>"})
	fake.on("navigator-lxa.mail.com/halogin", http.MethodGet, scriptedResp{
		status:   302,
		location: "https://navigator-lxa.mail.com/?sid=" + sid,
	})
	fake.on("jump/to/ciss", http.MethodGet, scriptedResp{
		status:   302,
		location: "https://account-lxa.mail.com/ciss/myAccountOverview?1&srttkn=" + srttkn,
	})
	fake.on("myAccountOverview", http.MethodGet, scriptedResp{status: 200, body: "<html/>"})
	fake.on("passwordChange?1&srttkn=", http.MethodGet, scriptedResp{
		status: 200,
		body:   `<form id="id5" method="post" action="./passwordChange?1-2.-form&amp;srttkn=` + srttkn + `"></form>`,
	})
	fake.on("saveChanges=x", http.MethodPost, scriptedResp{
		status:   302,
		location: "https://account-lxa.mail.com/ciss/security/edit/passwordChange?2&srttkn=" + srttkn + "&passwordChangedSuccessfully=true",
	})

	changer := &passwordChanger{http: fake}
	if err := changer.run("user@mail.com", "Old#1", "Newpass12abc"); err != nil {
		t.Fatal(err)
	}

	var save *http.Request
	var saveBody string
	for i, req := range fake.calls {
		if req.Method == http.MethodPost && strings.Contains(req.URL.String(), "saveChanges=x") {
			save = req
			saveBody = fake.bodies[i]
		}
		if req.Header.Get("Wicket-Ajax") != "" || req.Header.Get("X-Requested-With") != "" {
			t.Fatalf("ajax header on %s %s", req.Method, req.URL.String())
		}
	}
	if save == nil {
		t.Fatal("missing save POST")
	}
	if save.Header.Get("Wicket-Ajax") != "" || save.Header.Get("X-Requested-With") != "" {
		t.Fatal("save must not send wicket-ajax")
	}
	if !strings.Contains(save.URL.RawQuery, "1-2.-form") {
		t.Fatalf("expected live form version, got %s", save.URL.RawQuery)
	}
	values, err := url.ParseQuery(saveBody)
	if err != nil {
		t.Fatal(err)
	}
	if values.Get("editPanel:username") != "user@mail.com" {
		t.Fatalf("username=%q", values.Get("editPanel:username"))
	}
	if values.Get("editPanel:currentPasswordPanel:topWrapper:inputWrapper:input") != "Old#1" {
		t.Fatal("old password missing")
	}
	if values.Get("editPanel:newPasswordFieldPanel:topWrapper:inputWrapper:input") != "Newpass12abc" {
		t.Fatal("new password missing")
	}
	if values.Get("editPanel:retypeNewPasswordFieldPanel:topWrapper:inputWrapper:input") != "Newpass12abc" {
		t.Fatal("retype missing")
	}
}

func TestChangePasswordSubmitFeedback(t *testing.T) {
	fake := &fakeDoer{t: t}
	const ott = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	const sid = "abc123sidhexvalue000000000000000000000000000000000000000000000000"
	const srttkn = "11111111-2222-3333-4444-555555555555"
	fake.on("login.mail.com/login", http.MethodPost, scriptedResp{
		status:   303,
		location: "https://navigator-lxa.mail.com/login?ott=" + ott + "&auth_time=2026-08-10T06:00:00Z",
	})
	fake.on("navigator-lxa.mail.com/login?", http.MethodGet, scriptedResp{status: 200})
	fake.on("navigator-lxa.mail.com/halogin", http.MethodGet, scriptedResp{
		status:   302,
		location: "https://navigator-lxa.mail.com/?sid=" + sid,
	})
	fake.on("jump/to/ciss", http.MethodGet, scriptedResp{
		status:   302,
		location: "https://account-lxa.mail.com/ciss/myAccountOverview?1&srttkn=" + srttkn,
	})
	fake.on("myAccountOverview", http.MethodGet, scriptedResp{status: 200})
	fake.on("passwordChange?1&srttkn=", http.MethodGet, scriptedResp{
		status: 200,
		body:   `<form action="./passwordChange?1-2.-form&amp;srttkn=` + srttkn + `"></form>`,
	})
	fake.on("saveChanges=x", http.MethodPost, scriptedResp{
		status:   302,
		location: "https://account-lxa.mail.com/ciss/security/edit/passwordChange?2&srttkn=" + srttkn,
	})
	fake.on("passwordChange?2&srttkn=", http.MethodGet, scriptedResp{
		status: 200,
		body:   `<span class="feedbackPanelERROR">The current password is invalid</span>`,
	})

	changer := &passwordChanger{http: fake}
	err := changer.run("user@mail.com", "Old#1", "Newpass1")
	if err == nil {
		t.Fatal("expected submit error")
	}
	if !strings.Contains(err.Error(), "The current password is invalid") {
		t.Fatalf("err=%v", err)
	}
}

func TestNavLoginOmitsAuthTime(t *testing.T) {
	fake := &fakeDoer{t: t}
	const ott = "bae38d0b-b95d-4774-a1dd-72d9d2045aca"
	fake.on("login.mail.com/login", http.MethodPost, scriptedResp{
		status:   303,
		location: "https://navigator-lxa.mail.com/login?ott=" + ott + "&auth_time=2026-08-10T05:58:15Z",
	})
	fake.on("navigator-lxa.mail.com/login?", http.MethodGet, scriptedResp{status: 200})
	fake.on("navigator-lxa.mail.com/halogin", http.MethodGet, scriptedResp{
		status:   302,
		location: "https://navigator-lxa.mail.com/?sid=abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
	})
	fake.on("jump/to/ciss", http.MethodGet, scriptedResp{status: 500})

	changer := &passwordChanger{http: fake}
	_ = changer.run("user@mail.com", "Old#1", "Newpass1")
	var nav *http.Request
	for _, req := range fake.calls {
		if req.Method == http.MethodGet && strings.Contains(req.URL.String(), "navigator-lxa.mail.com/login?") {
			nav = req
		}
	}
	if nav == nil {
		t.Fatal("missing navigator login")
	}
	if nav.URL.Query().Get("auth_time") != "" {
		t.Fatalf("nav login must omit auth_time: %s", nav.URL.String())
	}
	if nav.URL.Query().Get("ott") != ott {
		t.Fatalf("ott=%s", nav.URL.Query().Get("ott"))
	}
}

func TestPasswordSubmitURLFromForm(t *testing.T) {
	page := "https://account-lxa.mail.com/ciss/security/edit/passwordChange?1&srttkn=abc"
	html := `<form id="id5" method="post" action="./passwordChange?1-2.-form&amp;srttkn=abc"></form>`
	got := passwordSubmitURL(page, html, "abc")
	if !strings.Contains(got, "passwordChange?1-2.-form") {
		t.Fatalf("url=%s", got)
	}
	if !strings.Contains(got, "saveChanges=x") {
		t.Fatalf("missing saveChanges: %s", got)
	}
}
