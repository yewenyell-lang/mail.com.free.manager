package mailcom

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type Options struct {
	Email        string
	Password     string
	AccessToken  string
	RefreshToken string
	ProxyURL     string
}

type Client struct {
	email        string
	password     string
	proxyURL     string
	session      Session
	loginHTTP    doer
	apiHTTP      doer
	refreshMu    sync.Mutex
}

func New(opts Options) (*Client, error) {
	loginHTTP, err := newFingerprintClient(profiles.Chrome_103, opts.ProxyURL, false)
	if err != nil {
		return nil, err
	}
	apiHTTP, err := newFingerprintClient(profiles.Okhttp4Android13, opts.ProxyURL, true)
	if err != nil {
		return nil, err
	}
	client := &Client{
		email:     strings.TrimSpace(strings.ToLower(opts.Email)),
		password:  opts.Password,
		proxyURL:  opts.ProxyURL,
		loginHTTP: loginHTTP,
		apiHTTP:   apiHTTP,
	}
	if opts.AccessToken != "" {
		client.session = Session{
			AccessToken:  opts.AccessToken,
			RefreshToken: opts.RefreshToken,
			AccountEmail: client.email,
			UpdatedAt:    nowMS(),
		}
	}
	return client, nil
}

func (c *Client) Session() Session {
	return c.session
}

func (c *Client) mailboxBase() string {
	return HSP2BaseURL + "/msgsrv/Mailbox/primaryMailbox"
}

func (c *Client) Login() (Session, error) {
	if c.session.AccessToken != "" {
		ok, err := c.ValidateToken(c.session.AccessToken)
		if err == nil && ok {
			return c.session, nil
		}
		if c.session.RefreshToken != "" {
			session, refreshErr := c.Refresh(c.session.RefreshToken)
			if refreshErr == nil {
				return session, nil
			}
		}
	}
	if c.password == "" {
		return Session{}, authError("Password is required when no valid session exists.")
	}
	return c.loginWithAndroidOAuth()
}

func (c *Client) ValidateToken(token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	req, err := http.NewRequest(http.MethodHead, MobsiBaseURL+"/UserData", nil)
	if err != nil {
		return false, err
	}
	c.applyAppHeaders(req, true)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.apiHTTP.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

func (c *Client) Refresh(refreshToken string) (Session, error) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	if refreshToken == "" {
		refreshToken = c.session.RefreshToken
	}
	if refreshToken == "" {
		return Session{}, authError("No refresh token available.")
	}
	token, err := c.oauthToken(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"scope":         FullAccessScope,
	})
	if err != nil {
		return Session{}, err
	}
	if token.AccessToken == "" {
		msg := token.ErrorDescription
		if msg == "" {
			msg = token.Error
		}
		if msg == "" {
			msg = "mail.com token refresh failed."
		}
		return Session{}, authError(msg)
	}
	c.session = c.toSession(token, refreshToken)
	return c.session, nil
}

func (c *Client) loginWithAndroidOAuth() (Session, error) {
	verifier := base64URL(randomBytes(48))
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64URL(sum[:])
	state := base64URL(randomBytes(48))

	authorizeURL, _ := url.Parse(OAuthBaseURL + "/authorize")
	query := authorizeURL.Query()
	query.Set("client_id", AndroidClientID)
	query.Set("redirect_uri", AndroidRedirectURI)
	query.Set("response_type", "code")
	query.Set("state", state)
	query.Set("code_challenge", challenge)
	query.Set("login_hint", c.email)
	query.Set("code_challenge_method", "S256")
	authorizeURL.RawQuery = query.Encode()

	authorize, err := c.webviewRequest(authorizeURL.String(), http.MethodGet, nil, nil)
	if err != nil {
		return Session{}, err
	}
	loginAppURL, err := requiredLocation(authorize, "authorize redirect")
	if err != nil {
		return Session{}, err
	}
	loginAppURL = resolveLocation(authorizeURL.String(), loginAppURL)
	authcodeContext := ""
	if parsed, parseErr := url.Parse(loginAppURL); parseErr == nil {
		authcodeContext = parsed.Query().Get("authcode-context")
	}
	if authcodeContext == "" {
		return Session{}, authError("Android OAuth login did not return authcode-context.")
	}
	if _, err = c.webviewRequest(loginAppURL, http.MethodGet, nil, nil); err != nil {
		return Session{}, err
	}

	loginFailed := url.URL{Scheme: "https", Host: "auth.mail.com", Path: "/loginapp/oauth2"}
	failedQuery := loginFailed.Query()
	failedQuery.Set("status", "login_failed")
	failedQuery.Set("login_hint", c.email)
	failedQuery.Set("authcode-context", authcodeContext)
	loginFailed.RawQuery = failedQuery.Encode()

	form := url.Values{}
	form.Set("password", c.password)
	form.Set("service", "oauth2")
	form.Set("successURL", OAuthBaseURL+"/authcode?authcode-context="+url.QueryEscape(authcodeContext))
	form.Set("loginFailedURL", loginFailed.String())
	form.Set("loginErrorURL", "https://auth.mail.com/login/error")
	form.Set("statistics", "")
	form.Set("username", c.email)

	headers := http.Header{
		"Accept":       {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		"Content-Type": {"application/x-www-form-urlencoded"},
		"Origin":       {"https://auth.mail.com"},
		"Referer":      {loginAppURL},
		http.HeaderOrderKey: {
			"accept", "accept-language", "content-type", "origin", "referer", "user-agent",
		},
	}
	login, err := c.webviewRequest("https://login.mail.com/login", http.MethodPost, strings.NewReader(form.Encode()), headers)
	if err != nil {
		return Session{}, err
	}
	authcodeURL, err := requiredLocation(login, "login redirect")
	if err != nil {
		return Session{}, err
	}
	authcodeURL = resolveLocation("https://login.mail.com/login", authcodeURL)
	authcode, err := c.webviewRequest(authcodeURL, http.MethodGet, nil, nil)
	if err != nil {
		return Session{}, err
	}
	appRedirect, err := requiredLocation(authcode, "authcode redirect")
	if err != nil {
		return Session{}, err
	}
	redirectURL, err := url.Parse(appRedirect)
	if err != nil {
		return Session{}, authError("Android OAuth login returned invalid redirect.")
	}
	code := redirectURL.Query().Get("code")
	returnedState := redirectURL.Query().Get("state")
	if code == "" {
		return Session{}, authError("Android OAuth login did not return authorization code.")
	}
	if returnedState != state {
		return Session{}, authError("Android OAuth state mismatch.")
	}

	token, err := c.oauthToken(map[string]string{
		"grant_type":    "authorization_code",
		"code":          code,
		"redirect_uri":  AndroidRedirectURI,
		"client_id":     AndroidClientID,
		"code_verifier": verifier,
	})
	if err != nil {
		return Session{}, err
	}
	if token.AccessToken == "" || token.RefreshToken == "" {
		msg := token.ErrorDescription
		if msg == "" {
			msg = token.Error
		}
		if msg == "" {
			msg = "mail.com Android OAuth token exchange failed."
		}
		return Session{}, authError(msg)
	}
	c.session = c.toSession(token, token.RefreshToken)
	return c.Refresh(token.RefreshToken)
}

func (c *Client) webviewRequest(rawURL, method string, body io.Reader, extra http.Header) (*http.Response, error) {
	req, err := http.NewRequest(method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", WebViewUserAgent)
	req.Header.Set("Accept-Language", "en-IN,en-GB;q=0.9,en;q=0.8")
	if extra != nil {
		for key, values := range extra {
			if key == http.HeaderOrderKey {
				req.Header[http.HeaderOrderKey] = values
				continue
			}
			for _, value := range values {
				req.Header.Set(key, value)
			}
		}
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	}
	resp, err := c.loginHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp, nil
}

func headerLocation(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	if value := resp.Header.Get("Location"); value != "" {
		return value
	}
	if values := resp.Header["Location"]; len(values) > 0 {
		return values[0]
	}
	if values := resp.Header["location"]; len(values) > 0 {
		return values[0]
	}
	return ""
}

func requiredLocation(resp *http.Response, label string) (string, error) {
	location := headerLocation(resp)
	if location == "" {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		if status == 200 || status == 401 || status == 403 {
			return "", authError("邮箱或密码错误")
		}
		return "", authError(fmt.Sprintf("Android OAuth %s missing Location (status %d)", label, status))
	}
	if strings.Contains(location, "login_failed") || strings.Contains(location, "/login/error") {
		return "", authError("邮箱或密码错误")
	}
	return location, nil
}

func (c *Client) oauthToken(form map[string]string) (oauthTokenResponse, error) {
	values := url.Values{}
	for key, value := range form {
		values.Set(key, value)
	}
	req, err := http.NewRequest(http.MethodPost, OAuthBaseURL+"/token", strings.NewReader(values.Encode()))
	if err != nil {
		return oauthTokenResponse{}, err
	}
	c.applyAppHeaders(req, false)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Authorization", AndroidOAuthBasic)
	req.Header.Set("Content-Type", `application/x-www-form-urlencoded;charset="UTF-8"`)
	resp, err := c.apiHTTP.Do(req)
	if err != nil {
		return oauthTokenResponse{}, err
	}
	raw, err := readBody(resp)
	if err != nil {
		return oauthTokenResponse{}, err
	}
	var token oauthTokenResponse
	_ = json.Unmarshal(raw, &token)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := token.ErrorDescription
		if msg == "" {
			msg = token.Error
		}
		if msg == "" {
			msg = fmt.Sprintf("OAuth token request failed with %d", resp.StatusCode)
		}
		return oauthTokenResponse{}, authError(msg)
	}
	return token, nil
}

func (c *Client) toSession(token oauthTokenResponse, retainedRefresh string) Session {
	now := nowMS()
	refresh := token.RefreshToken
	if refresh == "" {
		refresh = retainedRefresh
	}
	session := Session{
		AccessToken:  token.AccessToken,
		RefreshToken: refresh,
		AccountEmail: c.email,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if c.session.CreatedAt > 0 {
		session.CreatedAt = c.session.CreatedAt
	}
	if token.ExpiresIn > 0 {
		session.ExpiresAt = now + token.ExpiresIn*1000
	}
	return session
}

func (c *Client) applyAppHeaders(req *http.Request, withAuth bool) {
	req.Header.Set("Accept-Charset", "utf-8")
	req.Header.Set("Accept-Language", "en-IN,en-GB;q=0.9,en;q=0.8")
	req.Header.Set("User-Agent", AppUserAgent)
	req.Header.Set("X-Ui-App", "mailcom.android.androidmail/9.8.0")
	if withAuth && c.session.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.session.AccessToken)
	}
	req.Header[http.HeaderOrderKey] = []string{
		"accept", "accept-charset", "accept-language", "authorization", "content-type", "user-agent", "x-ui-app",
	}
}

func (c *Client) apiRequest(method, rawURL string, body []byte, headers http.Header, retry bool) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, rawURL, reader)
	if err != nil {
		return nil, nil, err
	}
	c.applyAppHeaders(req, true)
	for key, values := range headers {
		if key == http.HeaderOrderKey {
			continue
		}
		for _, value := range values {
			req.Header.Set(key, value)
		}
	}
	resp, err := c.apiHTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode == 401 && !retry {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if _, refreshErr := c.Refresh(""); refreshErr != nil {
			return nil, nil, refreshErr
		}
		return c.apiRequest(method, rawURL, body, headers, true)
	}
	raw, err := readBody(resp)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := truncate(strings.TrimSpace(string(raw)), 300)
		msg := fmt.Sprintf("%s %s failed with %d", method, rawURL, resp.StatusCode)
		if snippet != "" {
			msg += ": " + snippet
		}
		return resp, raw, apiError(msg, resp.StatusCode)
	}
	return resp, raw, nil
}

func (c *Client) apiJSON(method, rawURL string, payload any, accept, contentType string, dest any) error {
	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = encoded
	}
	headers := http.Header{}
	if accept != "" {
		headers.Set("Accept", accept)
	}
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}
	_, raw, err := c.apiRequest(method, rawURL, body, headers, false)
	if err != nil {
		return err
	}
	if dest == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dest)
}

func (c *Client) apiText(method, rawURL string, payload []byte, headers http.Header) (string, error) {
	_, raw, err := c.apiRequest(method, rawURL, payload, headers, false)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (c *Client) ensureSession() error {
	if c.session.AccessToken != "" {
		return nil
	}
	_, err := c.Login()
	return err
}

func randomBytes(n int) []byte {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return buf
}

func base64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func closeIdle(client doer) {
	if c, ok := client.(tls_client.HttpClient); ok {
		c.CloseIdleConnections()
	}
}

func (c *Client) Close() {
	closeIdle(c.loginHTTP)
	closeIdle(c.apiHTTP)
}
