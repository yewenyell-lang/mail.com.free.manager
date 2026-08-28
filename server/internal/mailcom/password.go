package mailcom

import (
	"fmt"
	htmlpkg "html"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/tls-client/profiles"
)

const (
	passwordRetryMax  = 8
	passwordRetryWait = 2 * time.Second

	webLoginURL     = "https://login.mail.com/login"
	webNavLoginBase = "https://navigator-lxa.mail.com/login"
	webHaloginBase  = "https://navigator-lxa.mail.com/halogin"
	webJumpCISS     = "https://navigator-lxa.mail.com/navigator/jump/to/ciss"
	webPwChangePath = "/ciss/security/edit/passwordChange"
	webAccountHost  = "https://account-lxa.mail.com"

	chrome150UA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	chrome150CH = `"Not;A=Brand";v="8", "Chromium";v="150", "Google Chrome";v="150"`
)

var (
	ottRe    = regexp.MustCompile(`[?&]ott=([0-9a-f-]+)`)
	sidRe    = regexp.MustCompile(`[?&]sid=([0-9a-f]+)`)
	srttknRe = regexp.MustCompile(`[?&]srttkn=([0-9a-f-]+)`)
	authRe   = regexp.MustCompile(`auth_time=([^&]+)`)

	feedbackErrorRe = regexp.MustCompile(`(?is)class="feedbackPanel(?:ERROR|error)[^"]*"[^>]*>(.*?)</(?:span|div|li)>`)
	feedbackAnyRe   = regexp.MustCompile(`(?is)class="[^"]*feedbackPanel[^"]*"[^>]*>(.*?)</(?:span|div|li)>`)
	htmlTagRe       = regexp.MustCompile(`<[^>]+>`)
	spaceRe         = regexp.MustCompile(`\s+`)
	formTagRe       = regexp.MustCompile(`(?is)<form\b([^>]*)>`)
	formActionRe    = regexp.MustCompile(`(?i)\baction="([^"]*)"`)
)

type passwordChanger struct {
	http doer
}

func ChangePassword(email, oldPassword, newPassword, proxyURL string) error {
	if err := validatePasswordChange(email, oldPassword, newPassword); err != nil {
		return err
	}
	var last error
	for attempt := 0; attempt < passwordRetryMax; attempt++ {
		client, err := newFingerprintClient(profiles.Chrome_133, proxyURL, false)
		if err != nil {
			return err
		}
		changer := &passwordChanger{http: client}
		err = changer.run(email, oldPassword, newPassword)
		closeIdle(client)
		if err == nil {
			return nil
		}
		last = err
		if attempt < passwordRetryMax-1 {
			time.Sleep(passwordRetryWait)
		}
	}
	if last == nil {
		last = apiError("多次尝试失败", 0)
	}
	if _, ok := last.(*Error); ok {
		return &Error{Message: "多次尝试失败: " + last.Error(), Kind: "api"}
	}
	return apiError("多次尝试失败: "+last.Error(), 0)
}

func validatePasswordChange(email, oldPassword, newPassword string) error {
	if strings.TrimSpace(email) == "" || oldPassword == "" || newPassword == "" {
		return validationError("邮箱、旧密码、新密码不能为空")
	}
	if oldPassword == newPassword {
		return validationError("新密码不能与旧密码相同")
	}
	if len(newPassword) < 12 {
		return validationError("新密码至少 12 位")
	}
	return nil
}

func (p *passwordChanger) run(email, oldPassword, newPassword string) error {
	ott, authTime, err := p.login(email, oldPassword)
	if err != nil {
		return err
	}
	sid, err := p.halogin(ott, authTime)
	if err != nil {
		return err
	}
	srttkn, myAccountURL, err := p.srttkn(sid)
	if err != nil {
		return err
	}
	pwURL, html, err := p.openPasswordChange(srttkn, myAccountURL)
	if err != nil {
		return err
	}
	return p.submit(srttkn, pwURL, html, email, oldPassword, newPassword)
}

func (p *passwordChanger) login(email, password string) (ott, authTime string, err error) {
	form := url.Values{}
	form.Set("username", email)
	form.Set("password", password)
	form.Set("service", "mailint")
	form.Set("uasServiceID", "mc_starter_mailcom")
	form.Set("successURL", "https://$(clientName)-$(dataCenter).mail.com/login")
	form.Set("loginFailedURL", "https://www.mail.com/logout/?ls=wd")
	form.Set("loginErrorURL", "https://www.mail.com/logout/?ls=te")
	form.Set("edition", "us")
	form.Set("lang", "en")
	form.Set("usertype", "standard")
	form.Set("ibaInfo", "abd=false")

	headers := http.Header{
		"User-Agent":   {chrome150UA},
		"Origin":       {"https://www.mail.com"},
		"Referer":      {"https://www.mail.com/"},
		"Content-Type": {"application/x-www-form-urlencoded"},
		"Accept":       {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
	}
	resp, _, err := p.do(http.MethodPost, webLoginURL, strings.NewReader(form.Encode()), headers)
	if err != nil {
		return "", "", passwordErr("login", err.Error())
	}
	if !isRedirect(resp.StatusCode) {
		return "", "", passwordAuth("login", fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
	location := headerLocation(resp)
	ott = firstSub(ottRe, location)
	authTime = firstSub(authRe, location)
	if ott == "" {
		return "", "", passwordAuth("login", "Location 中缺少 ott 参数")
	}
	return ott, authTime, nil
}

func (p *passwordChanger) halogin(ott, authTime string) (string, error) {
	navLogin := webNavLoginBase +
		"?edition=us&usertype=standard&ibaInfo=abd%3Dfalse" +
		"&uasServiceID=mc_starter_mailcom&lang=en&ott=" + ott
	_, _, _ = p.do(http.MethodGet, navLogin, nil, http.Header{
		"User-Agent": {chrome150UA},
		"Accept":     {"text/html,*/*"},
	})

	haloginURL := webHaloginBase +
		"?edition=us&usertype=standard&ibaInfo=abd%3Dfalse" +
		"&auth_time=" + authTime +
		"&uasServiceID=mc_starter_mailcom&lang=en&ott=" + ott + "&tz=8"
	resp, _, err := p.do(http.MethodGet, haloginURL, nil, http.Header{
		"User-Agent": {chrome150UA},
		"Referer":    {"https://navigator-lxa.mail.com/"},
	})
	if err != nil {
		return "", passwordErr("halogin", err.Error())
	}
	location := headerLocation(resp)
	sid := firstSub(sidRe, location)
	if sid == "" {
		return "", passwordErr("halogin", fmt.Sprintf("Location 中缺少 sid, status=%d, loc=%s", resp.StatusCode, truncate(location, 80)))
	}
	return sid, nil
}

func (p *passwordChanger) srttkn(sid string) (string, string, error) {
	resp, _, err := p.do(http.MethodGet, webJumpCISS+"?sid="+sid, nil, http.Header{
		"User-Agent": {chrome150UA},
	})
	if err != nil {
		return "", "", passwordErr("jump_ciss", err.Error())
	}
	loc := headerLocation(resp)
	if !isRedirect(resp.StatusCode) || loc == "" {
		return "", "", passwordErr("jump_ciss", fmt.Sprintf("HTTP %d", resp.StatusCode))
	}
	current := loc
	for hop := 0; hop < 6; hop++ {
		resp, _, err = p.do(http.MethodGet, current, nil, chromeNavHeaders(""))
		if err != nil {
			return "", "", passwordErr("myaccount", err.Error())
		}
		next := headerLocation(resp)
		if !isRedirect(resp.StatusCode) || next == "" {
			if resp.StatusCode != 200 {
				return "", "", passwordErr("myaccount", fmt.Sprintf("HTTP %d, url=%s", resp.StatusCode, truncate(current, 120)))
			}
			break
		}
		current = resolveLocation(current, next)
	}
	token := firstSub(srttknRe, current)
	if token == "" {
		return "", "", passwordErr("srttkn", "最终 URL 缺少 srttkn: "+truncate(current, 150))
	}
	return token, current, nil
}

func (p *passwordChanger) openPasswordChange(srttkn, referer string) (string, string, error) {
	current := webAccountHost + webPwChangePath + "?1&srttkn=" + srttkn
	for hop := 0; hop < 5; hop++ {
		resp, raw, err := p.do(http.MethodGet, current, nil, chromeNavHeaders(referer))
		if err != nil {
			return "", "", passwordErr("pwchange_page", err.Error())
		}
		next := headerLocation(resp)
		if !isRedirect(resp.StatusCode) || next == "" {
			if resp.StatusCode != 200 {
				return "", "", passwordErr("pwchange_page", fmt.Sprintf("HTTP %d", resp.StatusCode))
			}
			return current, string(raw), nil
		}
		current = resolveLocation(current, next)
	}
	return "", "", passwordErr("pwchange_page", "重定向次数超限")
}

func (p *passwordChanger) submit(srttkn, pwchangeURL, html, email, oldPassword, newPassword string) error {
	rawURL := passwordSubmitURL(pwchangeURL, html, srttkn)
	form := passwordChangeForm(email, oldPassword, newPassword)
	headers := chromeNavHeaders(pwchangeURL)
	headers.Set("Cache-Control", "max-age=0")
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	headers.Set("Origin", "https://account-lxa.mail.com")
	headers.Set("Sec-Fetch-Site", "same-origin")
	resp, _, err := p.do(http.MethodPost, rawURL, strings.NewReader(form.Encode()), headers)
	if err != nil {
		return passwordErr("submit", err.Error())
	}
	location := headerLocation(resp)
	if isRedirect(resp.StatusCode) {
		if strings.Contains(location, "passwordChangedSuccessfully=true") {
			return nil
		}
		feedback := p.feedback(location)
		detail := location
		if feedback != "" {
			detail = feedback
		}
		return passwordErr("submit", "表单校验被拒（无 passwordChangedSuccessfully）: "+truncate(detail, 160))
	}
	return passwordErr("submit", fmt.Sprintf("HTTP %d", resp.StatusCode))
}

func (p *passwordChanger) feedback(rawURL string) string {
	current := rawURL
	var body []byte
	for hop := 0; hop < 5; hop++ {
		resp, raw, err := p.do(http.MethodGet, current, nil, chromeNavHeaders(""))
		if err != nil {
			return ""
		}
		next := headerLocation(resp)
		if isRedirect(resp.StatusCode) && next != "" {
			current = resolveLocation(current, next)
			continue
		}
		if resp.StatusCode != 200 {
			return ""
		}
		body = raw
		break
	}
	return extractFeedback(string(body))
}

func (p *passwordChanger) do(method, rawURL string, body io.Reader, extra http.Header) (*http.Response, []byte, error) {
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, nil, err
	}
	for key, values := range extra {
		if key == http.HeaderOrderKey {
			req.Header[http.HeaderOrderKey] = values
			continue
		}
		for _, value := range values {
			req.Header.Set(key, value)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", chrome150UA)
	}
	if len(req.Header[http.HeaderOrderKey]) == 0 {
		req.Header[http.HeaderOrderKey] = []string{
			"accept", "accept-encoding", "accept-language", "cache-control", "content-type",
			"origin", "priority", "referer", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
			"sec-fetch-dest", "sec-fetch-mode", "sec-fetch-site", "sec-fetch-user",
			"upgrade-insecure-requests", "user-agent",
		}
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	raw, err := readBody(resp)
	if err != nil {
		return resp, nil, err
	}
	return resp, raw, nil
}

func chromeNavHeaders(referer string) http.Header {
	headers := http.Header{
		"Accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
		"Accept-Encoding":           {"gzip, deflate, br, zstd"},
		"Accept-Language":           {"zh-CN,zh;q=0.9"},
		"Priority":                  {"u=0, i"},
		"Sec-Ch-Ua":                 {chrome150CH},
		"Sec-Ch-Ua-Mobile":          {"?0"},
		"Sec-Ch-Ua-Platform":        {`"Windows"`},
		"Sec-Fetch-Dest":            {"iframe"},
		"Sec-Fetch-Mode":            {"navigate"},
		"Sec-Fetch-Site":            {"same-site"},
		"Sec-Fetch-User":            {"?1"},
		"Upgrade-Insecure-Requests": {"1"},
		"User-Agent":                {chrome150UA},
	}
	if referer != "" {
		headers.Set("Referer", referer)
	}
	return headers
}

func passwordSubmitURL(pageURL, html, srttkn string) string {
	action := parseFormAction(html)
	if action != "" {
		resolved := resolveLocation(pageURL, htmlpkg.UnescapeString(action))
		if !strings.Contains(resolved, "saveChanges=") {
			if strings.Contains(resolved, "?") {
				resolved += "&saveChanges=x"
			} else {
				resolved += "?saveChanges=x"
			}
		}
		return resolved
	}
	return webAccountHost + webPwChangePath + "?1-1.-form&srttkn=" + srttkn + "&saveChanges=x"
}

func parseFormAction(html string) string {
	for _, m := range formTagRe.FindAllStringSubmatch(html, -1) {
		if am := formActionRe.FindStringSubmatch(m[1]); len(am) > 1 && strings.Contains(am[1], "passwordChange") {
			return am[1]
		}
	}
	return ""
}

func passwordChangeForm(email, oldPassword, newPassword string) url.Values {
	form := url.Values{}
	form.Set("editPanel:username", email)
	form.Set("editPanel:currentPasswordPanel:topWrapper:inputWrapper:input", oldPassword)
	form.Set("editPanel:newPasswordFieldPanel:topWrapper:inputWrapper:input", newPassword)
	form.Set("editPanel:retypeNewPasswordFieldPanel:topWrapper:inputWrapper:input", newPassword)
	return form
}

func extractFeedback(html string) string {
	if html == "" {
		return ""
	}
	if m := feedbackErrorRe.FindStringSubmatch(html); len(m) > 1 {
		if text := visibleText(m[1]); text != "" {
			return text
		}
	}
	if matches := feedbackAnyRe.FindAllStringSubmatch(html, -1); len(matches) > 0 {
		for _, m := range matches {
			if len(m) > 1 {
				if text := visibleText(m[1]); text != "" {
					return text
				}
			}
		}
	}
	lower := strings.ToLower(html)
	for _, kw := range []string{"password", "requirements", "must contain", "invalid", "does not match", "taken", "unavailable"} {
		idx := strings.Index(lower, kw)
		if idx < 0 {
			continue
		}
		start := idx - 60
		if start < 0 {
			start = 0
		}
		end := idx + len(kw) + 80
		if end > len(html) {
			end = len(html)
		}
		snippet := visibleText(html[start:end])
		if snippet == "" || strings.Contains(snippet, "/*") || strings.Contains(snippet, "base-url") {
			continue
		}
		return snippet
	}
	return ""
}

func visibleText(raw string) string {
	text := htmlTagRe.ReplaceAllString(raw, " ")
	text = spaceRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func firstSub(re *regexp.Regexp, text string) string {
	m := re.FindStringSubmatch(text)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func isRedirect(status int) bool {
	return status == 301 || status == 302 || status == 303 || status == 307 || status == 308
}

func passwordErr(stage, detail string) error {
	kind := "api"
	if stage == "login" {
		kind = "auth"
	}
	msg := "[" + stage + "]"
	if detail != "" {
		msg += " " + detail
	}
	return &Error{Message: msg, Kind: kind}
}

func passwordAuth(stage, detail string) error {
	msg := "[" + stage + "]"
	if detail != "" {
		msg += " " + detail
	}
	return authError(msg)
}
