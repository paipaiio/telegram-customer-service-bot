package config

import "testing"

func TestLoad(t *testing.T) {
	env := map[string]string{
		"BOT_TOKEN":                 "token",
		"SUPPORT_GROUP_ID":          "-1001234567890",
		"DATA_FILE":                 "/data/forumdesk.json",
		"WEB_API_KEY":               "web-secret",
		"HTTP_ADDR":                 ":9090",
		"WEB_ALLOWED_ORIGINS":       "https://one.test, https://two.test",
		"WEB_RATE_LIMIT_PER_MINUTE": "90",
	}
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BotToken != "token" || cfg.SupportGroupID != -1001234567890 || cfg.DataFile != env["DATA_FILE"] || cfg.WebAPIKey != "web-secret" || cfg.HTTPAddr != ":9090" || cfg.WebRateLimitPerMinute != 90 || len(cfg.WebAllowedOrigins) != 2 {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadRejectsMissingAndInvalidValues(t *testing.T) {
	tests := []map[string]string{
		{"SUPPORT_GROUP_ID": "-1001", "WEB_API_KEY": "key"},
		{"BOT_TOKEN": "token", "WEB_API_KEY": "key"},
		{"BOT_TOKEN": "token", "SUPPORT_GROUP_ID": "not-a-number", "WEB_API_KEY": "key"},
		{"BOT_TOKEN": "token", "SUPPORT_GROUP_ID": "-1001"},
		{"BOT_TOKEN": "token", "SUPPORT_GROUP_ID": "-1001", "WEB_API_KEY": "key", "WEB_RATE_LIMIT_PER_MINUTE": "zero"},
	}
	for _, env := range tests {
		if _, err := Load(func(key string) string { return env[key] }); err == nil {
			t.Fatalf("Load(%v) expected error", env)
		}
	}
}

func TestLoadUsesDefaultDataFile(t *testing.T) {
	env := map[string]string{"BOT_TOKEN": "token", "SUPPORT_GROUP_ID": "-1001", "WEB_API_KEY": "key"}
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataFile != "./data/forumdesk.json" {
		t.Fatalf("data file=%q", cfg.DataFile)
	}
	if cfg.HTTPAddr != ":8080" || cfg.WebRateLimitPerMinute != 60 {
		t.Fatalf("web defaults=%#v", cfg)
	}
}
