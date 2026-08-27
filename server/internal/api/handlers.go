package api

import (
	"errors"
	"net/http"

	"mailcom/manager/internal/mailcom"
	"mailcom/manager/internal/stats"

	"github.com/gin-gonic/gin"
)

type handlers struct {
	proxyURL string
	stats    *stats.Store
}

type sessionIn struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

type loginReq struct {
	sessionIn
}

type listReq struct {
	sessionIn
	mailcom.ListOptions
}

type bodyReq struct {
	sessionIn
	MailID   string `json:"mailId"`
	Format   string `json:"format"`
	MarkRead *bool  `json:"markRead"`
}

type previewReq struct {
	sessionIn
	MailIDs []string `json:"mailIds"`
}

type attachReq struct {
	sessionIn
	MailID       string `json:"mailId"`
	AttachmentID string `json:"attachmentId"`
}

type sendReq struct {
	sessionIn
	mailcom.SendInput
}

type replyReq struct {
	sessionIn
	mailcom.ReplyInput
}

type forwardReq struct {
	sessionIn
	mailcom.ForwardInput
}

type actionReq struct {
	sessionIn
	MailIDs []string `json:"mailIds"`
}

func (h *handlers) client(in sessionIn) (*mailcom.Client, error) {
	return mailcom.New(mailcom.Options{
		Email:        in.Email,
		Password:     in.Password,
		AccessToken:  in.AccessToken,
		RefreshToken: in.RefreshToken,
		ProxyURL:     h.proxyURL,
	})
}

func writeErr(c *gin.Context, err error) {
	status := http.StatusBadGateway
	var mc *mailcom.Error
	if errors.As(err, &mc) {
		if mc.Kind == "auth" {
			status = http.StatusUnauthorized
		} else if mc.Kind == "validation" {
			status = http.StatusBadRequest
		} else if mc.Status > 0 {
			status = mc.Status
		}
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func sessionOut(client *mailcom.Client) gin.H {
	s := client.Session()
	return gin.H{
		"accessToken":  s.AccessToken,
		"refreshToken": s.RefreshToken,
		"expiresAt":    s.ExpiresAt,
		"accountEmail": s.AccountEmail,
	}
}

func (h *handlers) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req.sessionIn)
	if err != nil {
		h.stats.MarkLogin(false)
		writeErr(c, err)
		return
	}
	defer client.Close()
	if _, err := client.Login(); err != nil {
		h.stats.MarkLogin(false)
		writeErr(c, err)
		return
	}
	h.stats.MarkLogin(true)
	user, _ := client.UserData()
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client), "user": user})
}

func (h *handlers) refresh(c *gin.Context) {
	var req sessionIn
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	if _, err := client.Refresh(req.RefreshToken); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client)})
}

func (h *handlers) list(c *gin.Context) {
	var req listReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req.sessionIn)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	var data any
	if req.FolderID != "" {
		data, err = client.ListByFolder(req.FolderID, req.ListOptions)
	} else {
		data, err = client.ListIncoming(req.ListOptions)
	}
	if err != nil {
		writeErr(c, err)
		return
	}
	switch result := data.(type) {
	case mailcom.IncomingResponse:
		h.stats.AddMailListed(int64(len(result.Mail)))
	case mailcom.MessagesResponse:
		h.stats.AddMailListed(int64(len(result.Mail)))
	}
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client), "data": data})
}

func (h *handlers) search(c *gin.Context) {
	var req listReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req.sessionIn)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	data, err := client.Search(req.Query, req.ListOptions)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client), "data": data})
}

func (h *handlers) body(c *gin.Context) {
	var req bodyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req.sessionIn)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	markRead := true
	if req.MarkRead != nil {
		markRead = *req.MarkRead
	}
	html, err := client.GetBody(req.MailID, req.Format, markRead)
	if err != nil {
		writeErr(c, err)
		return
	}
	h.stats.MarkMailOpened()
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client), "html": html})
}

func (h *handlers) preview(c *gin.Context) {
	var req previewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req.sessionIn)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	data, err := client.GetPreview(req.MailIDs)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client), "data": data})
}

func (h *handlers) attachment(c *gin.Context) {
	var req attachReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req.sessionIn)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	file, err := client.DownloadAttachment(req.MailID, req.AttachmentID)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"session":     sessionOut(client),
		"filename":    file.Filename,
		"contentType": file.ContentType,
		"base64data":  file.Base64Data,
	})
}

func (h *handlers) send(c *gin.Context) {
	var req sendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req.sessionIn)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	data, err := client.Send(req.SendInput)
	if err != nil {
		writeErr(c, err)
		return
	}
	h.stats.MarkSend("send")
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client), "data": data})
}

func (h *handlers) reply(c *gin.Context) {
	var req replyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req.sessionIn)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	data, err := client.Reply(req.ReplyInput)
	if err != nil {
		writeErr(c, err)
		return
	}
	h.stats.MarkSend("reply")
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client), "data": data})
}

func (h *handlers) forward(c *gin.Context) {
	var req forwardReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req.sessionIn)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	data, err := client.Forward(req.ForwardInput)
	if err != nil {
		writeErr(c, err)
		return
	}
	h.stats.MarkSend("forward")
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client), "data": data})
}

func (h *handlers) runAction(c *gin.Context, fn func(*mailcom.Client, []string) error) {
	var req actionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req.sessionIn)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	if err := fn(client, req.MailIDs); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client), "ok": true})
}

func (h *handlers) actionRead(c *gin.Context) {
	h.runAction(c, func(client *mailcom.Client, ids []string) error {
		return client.BatchUpdate(ids, map[string]any{"read": true})
	})
}
func (h *handlers) actionUnread(c *gin.Context) {
	h.runAction(c, func(client *mailcom.Client, ids []string) error {
		return client.BatchUpdate(ids, map[string]any{"read": false})
	})
}
func (h *handlers) actionStar(c *gin.Context) {
	h.runAction(c, func(client *mailcom.Client, ids []string) error {
		return client.BatchUpdate(ids, map[string]any{"flagged": true})
	})
}
func (h *handlers) actionUnstar(c *gin.Context) {
	h.runAction(c, func(client *mailcom.Client, ids []string) error {
		return client.BatchUpdate(ids, map[string]any{"flagged": false})
	})
}
func (h *handlers) actionSpam(c *gin.Context) {
	h.runAction(c, func(client *mailcom.Client, ids []string) error {
		return client.MoveToFolderType(ids, "SPAM")
	})
}
func (h *handlers) actionTrash(c *gin.Context) {
	h.runAction(c, func(client *mailcom.Client, ids []string) error {
		return client.MoveToFolderType(ids, "TRASH")
	})
}
func (h *handlers) actionDelete(c *gin.Context) {
	h.runAction(c, func(client *mailcom.Client, ids []string) error {
		return client.DeletePermanent(ids)
	})
}
func (h *handlers) actionInbox(c *gin.Context) {
	h.runAction(c, func(client *mailcom.Client, ids []string) error {
		return client.MoveToFolderType(ids, "INBOX")
	})
}
func (h *handlers) folders(c *gin.Context) {
	h.accountJSON(c, func(client *mailcom.Client) (any, error) {
		list, err := client.ListFolders()
		if err != nil {
			return nil, err
		}
		return gin.H{"folders": list}, nil
	})
}

func (h *handlers) quota(c *gin.Context) {
	h.accountJSON(c, func(client *mailcom.Client) (any, error) { return client.Quota() })
}
func (h *handlers) aliases(c *gin.Context) {
	h.accountJSON(c, func(client *mailcom.Client) (any, error) { return client.Aliases() })
}
func (h *handlers) user(c *gin.Context) {
	h.accountJSON(c, func(client *mailcom.Client) (any, error) { return client.UserData() })
}

func (h *handlers) accountJSON(c *gin.Context, fn func(*mailcom.Client) (any, error)) {
	var req sessionIn
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := h.client(req)
	if err != nil {
		writeErr(c, err)
		return
	}
	defer client.Close()
	data, err := fn(client)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": sessionOut(client), "data": data})
}
