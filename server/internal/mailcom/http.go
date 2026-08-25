package mailcom

import (
	"io"
	"net/url"
	"strings"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type doer interface {
	Do(req *http.Request) (*http.Response, error)
}

func newFingerprintClient(profile profiles.ClientProfile, proxyURL string, followRedirect bool) (tls_client.HttpClient, error) {
	jar := tls_client.NewCookieJar()
	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(45),
		tls_client.WithClientProfile(profile),
		tls_client.WithCookieJar(jar),
		tls_client.WithInsecureSkipVerify(),
	}
	if !followRedirect {
		options = append(options, tls_client.WithNotFollowRedirects())
	}
	if strings.TrimSpace(proxyURL) != "" {
		options = append(options, tls_client.WithProxyUrl(strings.TrimSpace(proxyURL)))
	}
	return tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
}

func resolveLocation(baseURL, location string) string {
	ref, err := url.Parse(location)
	if err != nil {
		return location
	}
	if ref.IsAbs() {
		return location
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return location
	}
	return base.ResolveReference(ref).String()
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func truncate(value string, length int) string {
	if len(value) <= length {
		return value
	}
	return value[:length] + "..."
}

func nowMS() int64 {
	return time.Now().UnixMilli()
}

func asList(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
