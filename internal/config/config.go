package config

import (
	"fmt"
	"strconv"
	"strings"
)

type Config struct {
	BotToken       string
	SupportGroupID int64
	DataFile       string
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		BotToken: strings.TrimSpace(getenv("BOT_TOKEN")),
		DataFile: strings.TrimSpace(getenv("DATA_FILE")),
	}
	if cfg.BotToken == "" {
		return Config{}, fmt.Errorf("BOT_TOKEN is required")
	}
	group := strings.TrimSpace(getenv("SUPPORT_GROUP_ID"))
	if group == "" {
		return Config{}, fmt.Errorf("SUPPORT_GROUP_ID is required")
	}
	id, err := strconv.ParseInt(group, 10, 64)
	if err != nil {
		return Config{}, fmt.Errorf("parse SUPPORT_GROUP_ID: %w", err)
	}
	cfg.SupportGroupID = id
	if cfg.DataFile == "" {
		cfg.DataFile = "./data/forumdesk.json"
	}
	return cfg, nil
}
