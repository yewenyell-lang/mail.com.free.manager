package mailcom

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
)

func (c *Client) ListFolders() ([]Folder, error) {
	if err := c.ensureSession(); err != nil {
		return nil, err
	}
	var data FoldersResponse
	if err := c.apiJSON(http.MethodGet, c.mailboxBase()+"/folders?absoluteURI=false", nil, MIMEFolders, "", &data); err != nil {
		return nil, err
	}
	return data.Folders, nil
}

func (c *Client) ListByFolder(folderID string, opts ListOptions) (MessagesResponse, error) {
	if err := c.ensureSession(); err != nil {
		return MessagesResponse{}, err
	}
	params := url.Values{}
	params.Set("absoluteURI", "false")
	orderBy := opts.OrderBy
	if orderBy == "" {
		orderBy = "INTERNALDATE desc"
	}
	params.Set("orderBy", orderBy)
	amount := opts.Amount
	if amount <= 0 {
		amount = 25
	}
	params.Set("amount", strconv.Itoa(amount))
	if opts.Condition != "" {
		params.Set("condition", opts.Condition)
	}
	if opts.TagsShowAll != nil {
		params.Set("tagsShowAll", boolString(*opts.TagsShowAll))
	}
	rawURL := c.mailboxBase() + "/Folder/" + url.PathEscape(normalizeFolderID(folderID)) + "/Mail?" + params.Encode()
	headers := http.Header{}
	headers.Set("Accept", MIMEMessages)
	text, err := c.apiText(http.MethodGet, rawURL, nil, headers)
	if err != nil {
		return MessagesResponse{}, err
	}
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") {
		var resp MessagesResponse
		if err := json.Unmarshal([]byte(trimmed), &resp); err != nil {
			return MessagesResponse{}, err
		}
		return resp, nil
	}
	return MessagesResponse{Mail: nil, TotalCount: len(parseURIList(text))}, nil
}

func (c *Client) ListIncoming(opts ListOptions) (IncomingResponse, error) {
	excluded := opts.ExcludeFolderTypeOrID
	if len(excluded) == 0 {
		excluded = DefaultExcludedFolders
	}
	excludedSet := map[string]struct{}{}
	for _, item := range excluded {
		excludedSet[strings.ToUpper(item)] = struct{}{}
	}
	if opts.IncludeSpam != nil && !*opts.IncludeSpam {
		excludedSet["SPAM"] = struct{}{}
	}
	folders, err := c.ListFolders()
	if err != nil {
		return IncomingResponse{}, err
	}
	flat := flattenFolders(folders)
	sources := make([]SourceFolder, 0, len(flat))
	for _, folder := range flat {
		if folder.FolderIdentifier == "" || folder.Attribute == nil || folder.Attribute.FolderType == "" {
			continue
		}
		if _, ok := excludedSet[strings.ToUpper(folder.Attribute.FolderType)]; ok {
			continue
		}
		if _, ok := excludedSet[strings.ToUpper(folder.FolderIdentifier)]; ok {
			continue
		}
		name := folder.Attribute.FolderName
		if name == "" {
			name = folder.Attribute.FolderFullname
		}
		sources = append(sources, SourceFolder{
			FolderIdentifier: folder.FolderIdentifier,
			FolderType:       folder.Attribute.FolderType,
			FolderName:       name,
		})
	}

	amount := opts.Amount
	if amount <= 0 {
		amount = 25
	}
	tags := true
	if opts.TagsShowAll != nil {
		tags = *opts.TagsShowAll
	}
	type result struct {
		mail []MailMessage
		err  error
	}
	out := make([]result, len(sources))
	sem := make(chan struct{}, ListIncomingConcurrency)
	var wg sync.WaitGroup
	for i, source := range sources {
		wg.Add(1)
		sem <- struct{}{}
		go func(index int, folder SourceFolder) {
			defer wg.Done()
			defer func() { <-sem }()
			resp, listErr := c.ListByFolder(folder.FolderIdentifier, ListOptions{
				Amount:      amount,
				OrderBy:     opts.OrderBy,
				Condition:   opts.Condition,
				TagsShowAll: &tags,
			})
			if listErr != nil {
				out[index] = result{err: listErr}
				return
			}
			mails := make([]MailMessage, 0, len(resp.Mail))
			for _, mail := range resp.Mail {
				copied := mail
				folderCopy := folder
				copied.SourceFolder = &folderCopy
				mails = append(mails, copied)
			}
			out[index] = result{mail: mails}
		}(i, source)
	}
	wg.Wait()

	mail := make([]MailMessage, 0)
	for _, item := range out {
		if item.err != nil {
			return IncomingResponse{}, item.err
		}
		mail = append(mail, item.mail...)
	}
	sort.Slice(mail, func(i, j int) bool {
		var left, right int64
		if mail[i].MailHeader != nil {
			left = mail[i].MailHeader.Date
		}
		if mail[j].MailHeader != nil {
			right = mail[j].MailHeader.Date
		}
		return left > right
	})
	unread := 0
	for _, item := range mail {
		if item.Attribute != nil && item.Attribute.Read != nil && !*item.Attribute.Read {
			unread++
		}
	}
	return IncomingResponse{Mail: mail, TotalCount: len(mail), UnreadCount: unread, Folders: sources}, nil
}

func (c *Client) Search(query string, opts ListOptions) (MessagesResponse, error) {
	if err := c.ensureSession(); err != nil {
		return MessagesResponse{}, err
	}
	amount := opts.Amount
	if amount <= 0 {
		amount = 25
	}
	excluded := opts.ExcludeFolderTypeOrID
	if len(excluded) == 0 {
		excluded = DefaultExcludedFolders
	}
	orderBy := opts.OrderBy
	if orderBy == "" {
		orderBy = "INTERNALDATE desc"
	}
	body := map[string]any{
		"amount":               amount,
		"excludeFolderTypeOrId": excluded,
		"include": []map[string]any{
			{"conditions": []string{"mail.header:from,replyTo,cc,bcc,to,subject:" + escapeMailCondition(query)}},
		},
		"orderBy":            orderBy,
		"preferAbsoluteURIs": false,
	}
	var resp MessagesResponse
	if err := c.apiJSON(http.MethodPost, c.mailboxBase()+"/Mail/Query?absoluteURI=false", body, MIMEMessages, MIMEMailQuery, &resp); err != nil {
		return MessagesResponse{}, err
	}
	return resp, nil
}

func (c *Client) GetBody(mailID string, format string, markRead bool) (string, error) {
	if err := c.ensureSession(); err != nil {
		return "", err
	}
	normalized := normalizeMailID(mailID)
	accept := MIMEBodyHTML
	if format == "text" {
		accept = "text/plain"
	}
	headers := http.Header{}
	headers.Set("Accept", accept)
	text, err := c.apiText(http.MethodGet, c.mailboxBase()+"/Mail/"+url.PathEscape(normalized)+"/Body?absoluteURI=false", nil, headers)
	if err != nil {
		return "", err
	}
	if markRead {
		_ = c.BatchUpdate([]string{normalized}, map[string]any{"read": true})
	}
	return text, nil
}

func (c *Client) GetPreview(mailIDs []string) ([]MailPreview, error) {
	if err := c.ensureSession(); err != nil {
		return nil, err
	}
	form := url.Values{}
	for _, id := range mailIDs {
		form.Add("mailIdentifier", normalizeMailID(id))
	}
	headers := http.Header{}
	headers.Set("Accept", MIMEBodyPreviewSSE)
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	text, err := c.apiText(http.MethodPost, c.mailboxBase()+"/Mail/bodypreviews", []byte(form.Encode()), headers)
	if err != nil {
		return nil, err
	}
	return parseSSEJSON[MailPreview](text)
}

func (c *Client) Send(input SendInput) (SubmissionResult, error) {
	if err := c.ensureSession(); err != nil {
		return SubmissionResult{}, err
	}
	payload, err := c.buildPayload(input)
	if err != nil {
		return SubmissionResult{}, err
	}
	return c.submitMessage(c.submissionURL("", "", true), payload)
}

func (c *Client) Reply(input ReplyInput) (SubmissionResult, error) {
	if err := c.ensureSession(); err != nil {
		return SubmissionResult{}, err
	}
	if len(input.To) == 0 {
		return SubmissionResult{}, validationError("reply requires to")
	}
	if input.Subject == "" {
		input.Subject = replySubject("")
	}
	payload, err := c.buildPayload(input.SendInput)
	if err != nil {
		return SubmissionResult{}, err
	}
	return c.submitMessage(c.submissionURL(normalizeMailID(input.OriginalMailID), "", false), payload)
}

func (c *Client) Forward(input ForwardInput) (SubmissionResult, error) {
	if err := c.ensureSession(); err != nil {
		return SubmissionResult{}, err
	}
	if input.Subject == "" {
		input.Subject = forwardSubject("")
	}
	payload, err := c.buildPayload(input.SendInput)
	if err != nil {
		return SubmissionResult{}, err
	}
	return c.submitMessage(c.submissionURL("", normalizeMailID(input.OriginalMailID), false), payload)
}

func (c *Client) BatchUpdate(mailIDs []string, patch map[string]any) error {
	if err := c.ensureSession(); err != nil {
		return err
	}
	uris := make([]string, 0, len(mailIDs))
	for _, id := range mailIDs {
		uris = append(uris, mailURI(id))
	}
	body := map[string]any{}
	for key, value := range patch {
		body[key] = value
	}
	body["mailURIs"] = uris
	return c.apiJSON(http.MethodPost, c.mailboxBase()+"/MailBatchUpdate", body, MIMEBatchUpdateResult, MIMEBatchUpdate, nil)
}

func (c *Client) MoveToFolderType(mailIDs []string, folderType string) error {
	return c.BatchUpdate(mailIDs, map[string]any{"folderType": folderType, "flagged": false})
}

func (c *Client) DeletePermanent(mailIDs []string) error {
	if err := c.ensureSession(); err != nil {
		return err
	}
	form := url.Values{}
	for _, id := range mailIDs {
		form.Add("mailURI", mailURI(id))
	}
	form.Set("moveToTrash", "false")
	headers := http.Header{}
	headers.Set("Accept", "*/*")
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	_, err := c.apiText(http.MethodPost, c.mailboxBase()+"/MailBatchDelete", []byte(form.Encode()), headers)
	return err
}

func (c *Client) DownloadAttachment(mailID, attachmentID string) (DownloadedAttachment, error) {
	if err := c.ensureSession(); err != nil {
		return DownloadedAttachment{}, err
	}
	rawURL := c.mailboxBase() + "/Mail/" + url.PathEscape(normalizeMailID(mailID)) + "/Attachment/" + url.PathEscape(normalizeAttachmentID(attachmentID))
	headers := http.Header{}
	headers.Set("Accept", "*/*")
	resp, raw, err := c.apiRequest(http.MethodGet, rawURL, nil, headers, false)
	if err != nil {
		return DownloadedAttachment{}, err
	}
	contentType := "application/octet-stream"
	filename := normalizeAttachmentID(attachmentID)
	if resp != nil {
		if value := resp.Header.Get("Content-Type"); value != "" {
			contentType = value
		}
		if value := filenameFromDisposition(resp.Header.Get("Content-Disposition")); value != "" {
			filename = value
		}
	}
	return DownloadedAttachment{
		Filename:    filename,
		ContentType: contentType,
		Base64Data:  base64.StdEncoding.EncodeToString(raw),
	}, nil
}

func filenameFromDisposition(value string) string {
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	key := "filename="
	idx := strings.Index(lower, key)
	if idx < 0 {
		return ""
	}
	name := strings.TrimSpace(value[idx+len(key):])
	name = strings.Trim(name, `"`)
	if cut := strings.IndexAny(name, ";"); cut >= 0 {
		name = name[:cut]
	}
	return strings.Trim(name, `"`)
}

func (c *Client) Aliases() (AliasesResponse, error) {
	if err := c.ensureSession(); err != nil {
		return AliasesResponse{}, err
	}
	var resp AliasesResponse
	err := c.apiJSON(
		http.MethodGet,
		HSP2BaseURL+"/massrv/MailAccount/accountId/emailaddresses?absoluteURI=false&q.type.in=SENDER,MAIL_COLLECT&q.state.in=ACTIVE",
		nil,
		MIMEMailAddresses,
		"",
		&resp,
	)
	return resp, err
}

func (c *Client) Quota() (map[string]any, error) {
	if err := c.ensureSession(); err != nil {
		return nil, err
	}
	var resp map[string]any
	err := c.apiJSON(http.MethodGet, HSP2BaseURL+"/massrv/MailAccount/accountId/Quota", nil, MIMEQuotas, "", &resp)
	return resp, err
}

func (c *Client) UserData() (map[string]any, error) {
	if err := c.ensureSession(); err != nil {
		return nil, err
	}
	var resp map[string]any
	err := c.apiJSON(http.MethodGet, MobsiBaseURL+"/UserData", nil, "application/json", "", &resp)
	return resp, err
}

func (c *Client) ValidateRecipients(addresses []string) (map[string]any, error) {
	if err := c.ensureSession(); err != nil {
		return nil, err
	}
	var resp map[string]any
	err := c.apiJSON(http.MethodPost, HSP2BaseURL+"/massrv/MailAccount/emailaddressvalidations", addresses, MIMEValidationResp, MIMEValidationReq, &resp)
	return resp, err
}

func (c *Client) buildPayload(input SendInput) (minimalMailPayload, error) {
	if err := validateAttachments(input.Attachments); err != nil {
		return minimalMailPayload{}, err
	}
	from := input.From
	if from == "" {
		aliases, err := c.Aliases()
		if err == nil {
			for _, alias := range aliases.MailAddressList {
				if alias.DefaultSenderAddress && alias.Address != "" {
					if alias.DisplayName != "" {
						from = formatDisplayName(alias.DisplayName) + " <" + alias.Address + ">"
					} else {
						from = alias.Address
					}
					break
				}
			}
			if from == "" && len(aliases.MailAddressList) > 0 {
				from = aliases.MailAddressList[0].Address
			}
		}
		if from == "" {
			from = c.email
		}
	}
	priority := input.Priority
	if priority == "" {
		priority = "3"
	}
	payload := minimalMailPayload{}
	payload.MailHeader.From = from
	payload.MailHeader.To = asList(input.To)
	payload.MailHeader.CC = asList(input.CC)
	payload.MailHeader.BCC = asList(input.BCC)
	payload.MailHeader.Subject = input.Subject
	payload.MailHeader.Date = time.Now().UnixMilli()
	payload.MailHeader.Priority = priority
	payload.HTMLBody = input.HTMLBody
	for _, attachment := range input.Attachments {
		payload.Attachments = append(payload.Attachments, struct {
			ContentType string `json:"contentType"`
			Filename    string `json:"filename"`
			Base64Data  string `json:"base64data"`
			ContentID   string `json:"contentId,omitempty"`
			Inline      bool   `json:"inline,omitempty"`
		}{
			ContentType: attachment.ContentType,
			Filename:    attachment.Filename,
			Base64Data:  attachment.Base64Data,
			ContentID:   attachment.ContentID,
			Inline:      attachment.Inline,
		})
	}
	if payload.Attachments == nil {
		payload.Attachments = []struct {
			ContentType string `json:"contentType"`
			Filename    string `json:"filename"`
			Base64Data  string `json:"base64data"`
			ContentID   string `json:"contentId,omitempty"`
			Inline      bool   `json:"inline,omitempty"`
		}{}
	}
	return payload, nil
}

func (c *Client) submitMessage(rawURL string, payload minimalMailPayload) (SubmissionResult, error) {
	headers := http.Header{}
	headers.Set("Accept", MIMEEventStream)
	headers.Set("Content-Type", MIMEMinimalMail)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return SubmissionResult{}, err
	}
	text, err := c.apiText(http.MethodPost, rawURL, encoded, headers)
	if err != nil {
		return SubmissionResult{}, err
	}
	return parseMailSubmissionResult(text)
}

func (c *Client) submissionURL(inReplyTo, forwardedOriginal string, includeMeta bool) string {
	params := url.Values{}
	if inReplyTo != "" {
		params.Set("@SUBMISSION-TRANSIENT-IN-REPLY-TO", inReplyTo)
	}
	if forwardedOriginal != "" {
		params.Set("@SUBMISSION-TRANSIENT-FORWARDED-ORIGINAL", forwardedOriginal)
	}
	if includeMeta || inReplyTo != "" || forwardedOriginal != "" {
		params.Set("@SUBMISSION-TRANSIENT-UUID", newUUID())
		params.Set("MailSizeLimitExceededExceptionMapper.explicitCode", "true")
	}
	query := params.Encode()
	if query == "" {
		return c.mailboxBase() + "/Mailsubmission"
	}
	return c.mailboxBase() + "/Mailsubmission?" + query
}

func flattenFolders(folders []Folder) []Folder {
	out := make([]Folder, 0, len(folders))
	var walk func([]Folder)
	walk = func(items []Folder) {
		for _, item := range items {
			out = append(out, item)
			if len(item.Folders) > 0 {
				walk(item.Folders)
			}
		}
	}
	walk(folders)
	return out
}

func escapeMailCondition(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, ":", `\:`)
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func validateAttachments(attachments []AttachmentInput) error {
	total := 0
	for _, attachment := range attachments {
		if attachment.Base64Data == "" {
			return validationError(`Attachment "` + attachment.Filename + `" requires base64data.`)
		}
		decoded, err := base64.StdEncoding.DecodeString(attachment.Base64Data)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(attachment.Base64Data)
			if err != nil {
				return validationError(`Attachment "` + attachment.Filename + `" is not valid base64.`)
			}
		}
		total += len(decoded)
	}
	if total > MaxTotalAttachmentBytes {
		return validationError("Attachments exceed the 25 MB limit")
	}
	return nil
}

func formatDisplayName(name string) string {
	if strings.ContainsAny(name, `",()<>[]:;@\`) {
		name = strings.ReplaceAll(name, `\`, `\\`)
		name = strings.ReplaceAll(name, `"`, `\"`)
		return `"` + name + `"`
	}
	return name
}

func replySubject(subject string) string {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return "Re:"
	}
	if len(trimmed) >= 3 && strings.EqualFold(trimmed[:3], "Re:") {
		return trimmed
	}
	return "Re: " + trimmed
}

func forwardSubject(subject string) string {
	trimmed := strings.TrimSpace(subject)
	if trimmed == "" {
		return "Fwd:"
	}
	if len(trimmed) >= 4 && (strings.EqualFold(trimmed[:4], "Fwd:") || strings.EqualFold(trimmed[:4], "Fw: ")) {
		return trimmed
	}
	if len(trimmed) >= 3 && strings.EqualFold(trimmed[:3], "Fw:") {
		return trimmed
	}
	return "Fwd: " + trimmed
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func newUUID() string {
	b := randomBytes(16)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return sprintfUUID(b)
}

func sprintfUUID(b []byte) string {
	return strings.ToLower(
		hexByte(b[0]) + hexByte(b[1]) + hexByte(b[2]) + hexByte(b[3]) + "-" +
			hexByte(b[4]) + hexByte(b[5]) + "-" +
			hexByte(b[6]) + hexByte(b[7]) + "-" +
			hexByte(b[8]) + hexByte(b[9]) + "-" +
			hexByte(b[10]) + hexByte(b[11]) + hexByte(b[12]) + hexByte(b[13]) + hexByte(b[14]) + hexByte(b[15]),
	)
}

func hexByte(b byte) string {
	const hex = "0123456789abcdef"
	return string([]byte{hex[b>>4], hex[b&0x0f]})
}
