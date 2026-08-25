package mailcom

type Session struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	AccountEmail string `json:"accountEmail,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
}

type MailHeader struct {
	From      string   `json:"from,omitempty"`
	To        []string `json:"to,omitempty"`
	CC        []string `json:"cc,omitempty"`
	BCC       []string `json:"bcc,omitempty"`
	Subject   string   `json:"subject,omitempty"`
	Date      int64    `json:"date,omitempty"`
	Priority  string   `json:"priority,omitempty"`
	MessageType string `json:"messageType,omitempty"`
}

type MailAttribute struct {
	MailIdentifier             string `json:"mailIdentifier,omitempty"`
	FolderIdentifier           string `json:"folderIdentifier,omitempty"`
	FolderType                 string `json:"folderType,omitempty"`
	Read                       *bool  `json:"read,omitempty"`
	Flagged                    *bool  `json:"flagged,omitempty"`
	HasDownloadableAttachments bool   `json:"hasDownloadableAttachments,omitempty"`
}

type MailAttachment struct {
	AttachmentURI string `json:"attachmentURI,omitempty"`
	ContentType   string `json:"contentType,omitempty"`
	ContentID     string `json:"contentId,omitempty"`
	Filename      string `json:"filename,omitempty"`
	EstimatedSize int64  `json:"estimatedSize,omitempty"`
	Inline        bool   `json:"inline,omitempty"`
}

type DownloadedAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	Base64Data  string `json:"base64data"`
}

type MailMessage struct {
	MailURI     string         `json:"mailURI,omitempty"`
	Attribute   *MailAttribute `json:"attribute,omitempty"`
	MailHeader  *MailHeader    `json:"mailHeader,omitempty"`
	MailBodyURI string         `json:"mailBodyURI,omitempty"`
	Attachments *struct {
		Attachment []MailAttachment `json:"attachment,omitempty"`
	} `json:"attachments,omitempty"`
	SourceFolder *SourceFolder `json:"sourceFolder,omitempty"`
}

type SourceFolder struct {
	FolderIdentifier string `json:"folderIdentifier"`
	FolderType       string `json:"folderType"`
	FolderName       string `json:"folderName,omitempty"`
}

type Folder struct {
	FolderIdentifier string `json:"folderIdentifier,omitempty"`
	Attribute        *struct {
		FolderName     string `json:"folderName,omitempty"`
		FolderFullname string `json:"folderFullname,omitempty"`
		FolderType     string `json:"folderType,omitempty"`
		SystemFolder   bool   `json:"systemFolder,omitempty"`
	} `json:"attribute,omitempty"`
	Folders []Folder `json:"folders,omitempty"`
}

type FoldersResponse struct {
	Folders []Folder `json:"folders"`
}

type MessagesResponse struct {
	Mail        []MailMessage `json:"mail"`
	TotalCount  int           `json:"totalCount,omitempty"`
	UnreadCount int           `json:"unreadCount,omitempty"`
}

type IncomingResponse struct {
	Mail        []MailMessage `json:"mail"`
	TotalCount  int           `json:"totalCount"`
	UnreadCount int           `json:"unreadCount"`
	Folders     []SourceFolder `json:"folders"`
}

type MailPreview struct {
	MailIdentifier string `json:"mailIdentifier"`
	Preview        string `json:"preview"`
}

type Alias struct {
	Type                   string `json:"type,omitempty"`
	EntryDate              string `json:"entryDate,omitempty"`
	Address                string `json:"address"`
	DisplayName            string `json:"displayName,omitempty"`
	DefaultSenderAddress   bool   `json:"defaultSenderAddress,omitempty"`
	DefaultReceiverAddress bool   `json:"defaultReceiverAddress,omitempty"`
	State                  string `json:"state,omitempty"`
	Deletable              bool   `json:"deletable,omitempty"`
}

type AliasesResponse struct {
	MailAddressList []Alias `json:"mailaddresslist"`
}

type AttachmentInput struct {
	ContentType string `json:"contentType"`
	Filename    string `json:"filename"`
	Base64Data  string `json:"base64data"`
	ContentID   string `json:"contentId,omitempty"`
	Inline      bool   `json:"inline,omitempty"`
}

type SendInput struct {
	From        string            `json:"from,omitempty"`
	To          []string          `json:"to"`
	CC          []string          `json:"cc,omitempty"`
	BCC         []string          `json:"bcc,omitempty"`
	Subject     string            `json:"subject,omitempty"`
	HTMLBody    string            `json:"htmlBody"`
	Attachments []AttachmentInput `json:"attachments,omitempty"`
	Priority    string            `json:"priority,omitempty"`
}

type ReplyInput struct {
	SendInput
	OriginalMailID string `json:"originalMailId"`
}

type ForwardInput struct {
	SendInput
	OriginalMailID string `json:"originalMailId"`
}

type SubmissionResult struct {
	MessageID    string `json:"messageId"`
	RawLocation  string `json:"rawLocation"`
}

type ListOptions struct {
	Amount               int      `json:"amount,omitempty"`
	OrderBy              string   `json:"orderBy,omitempty"`
	Condition            string   `json:"condition,omitempty"`
	TagsShowAll          *bool    `json:"tagsShowAll,omitempty"`
	ExcludeFolderTypeOrID []string `json:"excludeFolderTypeOrId,omitempty"`
	IncludeSpam          *bool    `json:"includeSpam,omitempty"`
	Query                string   `json:"query,omitempty"`
	FolderID             string   `json:"folderId,omitempty"`
}

type oauthTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type minimalMailPayload struct {
	MailHeader struct {
		From     string   `json:"from"`
		To       []string `json:"to"`
		CC       []string `json:"cc"`
		BCC      []string `json:"bcc"`
		Subject  string   `json:"subject"`
		Date     int64    `json:"date"`
		Priority string   `json:"priority"`
	} `json:"mailHeader"`
	HTMLBody    string `json:"htmlBody"`
	Attachments []struct {
		ContentType string `json:"contentType"`
		Filename    string `json:"filename"`
		Base64Data  string `json:"base64data"`
		ContentID   string `json:"contentId,omitempty"`
		Inline      bool   `json:"inline,omitempty"`
	} `json:"attachments"`
}
