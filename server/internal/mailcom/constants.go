package mailcom

const (
	OAuthBaseURL = "https://oauth2.mail.com"
	MobsiBaseURL = "https://mobsi.mail.com/rest/MobSI"
	HSP2BaseURL  = "https://hsp2.mail.com/service"

	AppUserAgent = "mailcom.android.androidmail/9.8.0 Dalvik/2.1.0 (Linux; U; Android 13; SM-S908E Build/TQ2B.230505.005.A1)"
	WebViewUserAgent = "Mozilla/5.0 (Linux; Android 13; SM-S908E Build/TQ2B.230505.005.A1; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/101.0.4951.61 Mobile Safari/537.36 [APPNME/mailcom.android.androidmail;APPVS/9.8.0;APPTNME/andall]"

	AndroidClientID    = "mailcom_mailapp_android"
	AndroidRedirectURI = "com.mail.androidmail.redirect://authorization_code_grant"
	AndroidOAuthBasic  = "Basic bWFpbGNvbV9tYWlsYXBwX2FuZHJvaWQ6a2luMmxTU2tVUXRRQ0NsWG9YZklOaEp1bUc2SmQwM0taNVdMN05KOQ=="

	FullAccessScope = "mailbox_user_full_access mailbox_user_status_access hsp_user_full_access onlinestorage_user_meta_read onlinestorage_user_meta_write foo bar"

	MaxTotalAttachmentBytes = 25 * 1024 * 1024
	ListIncomingConcurrency = 5
)

var DefaultExcludedFolders = []string{"TRASH", "DRAFTS", "OUTBOX"}
var NoSpamExcludedFolders = []string{"SPAM", "TRASH", "DRAFTS", "OUTBOX"}

const (
	MIMEFolder            = "application/vnd.ui.trinity.folder-v2+json"
	MIMEFolderCreate      = "application/vnd.ui.trinity.folder.create+json; charset=utf-8"
	MIMEFolderUpdate      = "application/vnd.ui.trinity.folder.update+json"
	MIMEFolders           = "application/vnd.ui.trinity.folders-v5+json"
	MIMEMailAddresses     = "application/vnd.ui.trinity.mailaddress.list-v5+json"
	MIMEMessages          = "application/vnd.ui.trinity.messages+json"
	MIMEMailQuery         = "application/vnd.ui.trinity.mailquery+json"
	MIMEMinimalMail       = "application/vnd.ui.trinity.minimalmailmessage+json"
	MIMEMinimalMailAddr   = "application/vnd.ui.trinity.minimalmailaddress-v3+json"
	MIMEBatchUpdate       = "application/vnd.ui.trinity.message.batchupdate-v2+json"
	MIMEBatchUpdateResult = "application/vnd.ui.trinity.message.batchupdate.result-v2+json"
	MIMEValidationReq     = "application/vnd.ui.trinity.email-address-validation-request+json"
	MIMEValidationResp    = "application/vnd.ui.trinity.email-address-validation-response+json"
	MIMESettings          = "application/vnd.ui.trinity.settings-v2+json"
	MIMEQuotas            = "application/vnd.ui.trinity.quotas+json"
	MIMEBodyHTML          = "text/vnd.ui.insecure+html; removeCharsetMetaInfo=true"
	MIMEBodyPreviewSSE    = "text/event-stream; length=300; builder=html"
	MIMEEventStream       = "text/event-stream"
	MIMEURIList           = "text/uri-list"
)
