package importparse

import (
	"regexp"
	"strings"
)

type Account struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Raw      string `json:"raw,omitempty"`
}

var emailPass = regexp.MustCompile(`(?i)^([^\s:]+@[^\s:]+)(?::|----)(.+)$`)

func Parse(text string) []Account {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]Account, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		email, password, ok := splitLine(line)
		if !ok {
			continue
		}
		key := strings.ToLower(email)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, Account{Email: email, Password: password, Raw: line})
	}
	return out
}

func splitLine(line string) (string, string, bool) {
	if strings.Contains(line, "----") {
		email, password, ok := strings.Cut(line, "----")
		email = strings.TrimSpace(email)
		password = strings.TrimSpace(password)
		if !ok || !looksLikeEmail(email) || password == "" {
			return "", "", false
		}
		return email, password, true
	}
	match := emailPass.FindStringSubmatch(line)
	if len(match) != 3 {
		return "", "", false
	}
	return strings.TrimSpace(match[1]), strings.TrimSpace(match[2]), true
}

func looksLikeEmail(value string) bool {
	at := strings.LastIndex(value, "@")
	if at <= 0 || at == len(value)-1 {
		return false
	}
	return strings.Contains(value[at+1:], ".")
}
