package mailcom

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	mailIDRe       = regexp.MustCompile(`(?:^|/)Mail/([^/?#]+)`)
	attachmentIDRe = regexp.MustCompile(`(?:^|/)Attachment/([^/?#]+)`)
	folderIDRe     = regexp.MustCompile(`(?:^|/)Folder/([^/?#]+)`)
)

func safeDecode(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

func normalizeMailID(input string) string {
	decoded := safeDecode(strings.TrimSpace(input))
	if match := mailIDRe.FindStringSubmatch(decoded); len(match) > 1 {
		return match[1]
	}
	decoded = regexp.MustCompile(`^(\.\./)*Mail/`).ReplaceAllString(decoded, "")
	return strings.TrimLeft(decoded, "/")
}

func normalizeAttachmentID(input string) string {
	decoded := safeDecode(strings.TrimSpace(input))
	if match := attachmentIDRe.FindStringSubmatch(decoded); len(match) > 1 {
		return match[1]
	}
	decoded = regexp.MustCompile(`^(\.\./)*Attachment/`).ReplaceAllString(decoded, "")
	return strings.TrimLeft(decoded, "/")
}

func normalizeFolderID(input string) string {
	decoded := safeDecode(strings.TrimSpace(input))
	if match := folderIDRe.FindStringSubmatch(decoded); len(match) > 1 {
		return match[1]
	}
	decoded = regexp.MustCompile(`^(\.\./)*Folder/`).ReplaceAllString(decoded, "")
	return strings.TrimLeft(decoded, "/")
}

func mailURI(mailID string) string {
	return "../../Mail/" + normalizeMailID(mailID)
}

func folderURI(folderID string) string {
	return "/Folder/" + normalizeFolderID(folderID)
}

func parseURIList(text string) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		ids = append(ids, normalizeMailID(line))
	}
	return ids
}
