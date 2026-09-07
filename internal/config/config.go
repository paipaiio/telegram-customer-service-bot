package config

import (
	"fmt"
	"strconv"
	"strings"
)

type Config struct {
	BotToken              string
	SupportGroupID        int64
	DataFile              string
	HTTPAddr              string
	WebAPIKey             string
	WebAllowedOrigins     []string
	WebRateLimitPerMinute int
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		BotToken:  strings.TrimSpace(getenv("BOT_TOKEN")),
		DataFile:  strings.TrimSpace(getenv("DATA_FILE")),
		HTTPAddr:  strings.TrimSpace(getenv("HTTP_ADDR")),
		WebAPIKey: strings.TrimSpace(getenv("WEB_API_KEY")),
	}
	if cfg.BotToken == "" {
		return Config{}, fmt.Errorf("BOT_TOKEN is required")
	}
	if cfg.WebAPIKey == "" {
		return Config{}, fmt.Errorf("WEB_API_KEY is required")
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
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	for _, origin := range strings.Split(getenv("WEB_ALLOWED_ORIGINS"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			cfg.WebAllowedOrigins = append(cfg.WebAllowedOrigins, origin)
		}
	}
	limitValue := strings.TrimSpace(getenv("WEB_RATE_LIMIT_PER_MINUTE"))
	if limitValue == "" {
		cfg.WebRateLimitPerMinute = 60
	} else {
		limit, err := strconv.Atoi(limitValue)
		if err != nil || limit < 1 || limit > 10000 {
			return Config{}, fmt.Errorf("WEB_RATE_LIMIT_PER_MINUTE must be between 1 and 10000")
		}
		cfg.WebRateLimitPerMinute = limit
	}
	return cfg, nil
}
