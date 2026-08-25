package api

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func New(proxyURL string, webDir string) *gin.Engine {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors())
	r.Use(func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 40<<20)
		}
		c.Next()
	})
	r.MaxMultipartMemory = 32 << 20

	h := &handlers{proxyURL: strings.TrimSpace(proxyURL)}
	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
		api.POST("/auth/login", h.login)
		api.POST("/auth/refresh", h.refresh)
		api.POST("/mail/folders", h.folders)
		api.POST("/mail/list", h.list)
		api.POST("/mail/search", h.search)
		api.POST("/mail/body", h.body)
		api.POST("/mail/preview", h.preview)
		api.POST("/mail/attachment", h.attachment)
		api.POST("/mail/send", h.send)
		api.POST("/mail/reply", h.reply)
		api.POST("/mail/forward", h.forward)
		api.POST("/actions/read", h.actionRead)
		api.POST("/actions/unread", h.actionUnread)
		api.POST("/actions/star", h.actionStar)
		api.POST("/actions/unstar", h.actionUnstar)
		api.POST("/actions/spam", h.actionSpam)
		api.POST("/actions/trash", h.actionTrash)
		api.POST("/actions/delete", h.actionDelete)
		api.POST("/actions/inbox", h.actionInbox)
		api.POST("/account/quota", h.quota)
		api.POST("/account/aliases", h.aliases)
		api.POST("/account/user", h.user)
	}

	if webDir != "" {
		r.Static("/assets", webDir+"/assets")
		r.StaticFile("/favicon.ico", webDir+"/favicon.ico")
		r.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api/") {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			c.File(webDir + "/index.html")
		})
	}
	return r
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
