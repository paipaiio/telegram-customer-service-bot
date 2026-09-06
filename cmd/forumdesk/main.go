package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"forumdesk/internal/app"
	"forumdesk/internal/bot"
	"forumdesk/internal/config"
	"forumdesk/internal/store"
	"forumdesk/internal/telegram"
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

	log.Printf("ForumDesk started; support group=%d data=%s", cfg.SupportGroupID, cfg.DataFile)
	app.Run(ctx, log.Default(), api, handler)
	log.Print("ForumDesk stopped")
}
