package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"forumdesk/internal/app"
	"forumdesk/internal/bot"
	"forumdesk/internal/config"
	"forumdesk/internal/store"
	"forumdesk/internal/telegram"
	"forumdesk/internal/webapi"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	s, err := store.OpenFile(cfg.DataFile)
	if err != nil {
		log.Fatal(err)
	}
	api := telegram.NewClient(cfg.BotToken)
	handler := bot.NewHandler(cfg.SupportGroupID, s, api)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := api.SetAdminCommands(ctx, cfg.SupportGroupID); err != nil {
		log.Printf("register admin commands: %v", err)
	}
	if err := api.SetPrivateCommands(ctx); err != nil {
		log.Printf("register private commands: %v", err)
	}
	httpServer := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: webapi.New(webapi.Config{
			APIKey: cfg.WebAPIKey, SupportGroupID: cfg.SupportGroupID,
			AllowedOrigins: cfg.WebAllowedOrigins, RateLimitPerMinute: cfg.WebRateLimitPerMinute,
		}, s, api),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("ForumDesk web API listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("web API: %v", err)
			stop()
		}
	}()

	log.Printf("ForumDesk started; support group=%d data=%s", cfg.SupportGroupID, cfg.DataFile)
	app.Run(ctx, log.Default(), api, handler)
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		log.Printf("web API shutdown: %v", err)
	}
	log.Print("ForumDesk stopped")
}
