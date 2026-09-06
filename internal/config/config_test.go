package config

import "testing"

func TestLoad(t *testing.T) {
	env := map[string]string{
		"BOT_TOKEN":        "token",
		"SUPPORT_GROUP_ID": "-1001234567890",
		"DATA_FILE":        "/data/forumdesk.json",
	}
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.BotToken != "token" || cfg.SupportGroupID != -1001234567890 || cfg.DataFile != env["DATA_FILE"] {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadRejectsMissingAndInvalidValues(t *testing.T) {
	tests := []map[string]string{
		{"SUPPORT_GROUP_ID": "-1001"},
		{"BOT_TOKEN": "token"},
		{"BOT_TOKEN": "token", "SUPPORT_GROUP_ID": "not-a-number"},
	}
	for _, env := range tests {
		if _, err := Load(func(key string) string { return env[key] }); err == nil {
			t.Fatalf("Load(%v) expected error", env)
		}
	}
}

func TestLoadUsesDefaultDataFile(t *testing.T) {
	env := map[string]string{"BOT_TOKEN": "token", "SUPPORT_GROUP_ID": "-1001"}
	cfg, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataFile != "./data/forumdesk.json" {
		t.Fatalf("data file=%q", cfg.DataFile)
	}
}
