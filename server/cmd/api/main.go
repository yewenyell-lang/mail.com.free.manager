package main

import (
	"log"
	"os"
	"path/filepath"

	"mailcom/manager/internal/api"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8787"
	}
	proxy := os.Getenv("MAILCOM_PROXY")
	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		candidates := []string{"web/dist", "../web/dist", "./dist"}
		for _, candidate := range candidates {
			if info, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil && !info.IsDir() {
				webDir = candidate
				break
			}
		}
	}
	engine := api.New(proxy, webDir)
	log.Printf("listening on :%s proxy=%q web=%q", port, proxy, webDir)
	if err := engine.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
