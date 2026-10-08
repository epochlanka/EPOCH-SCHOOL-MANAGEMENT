package main

import (
	"bufio"
	"os"
	"strings"
)

type Config struct {
	Addr         string
	DBPath       string
	TZ           string
	CountryCode  string
	SecureCookie bool
	SeedDemo     bool

	AdminEmail    string
	AdminPassword string

	// SMS: "log" (simulate), "notifylk", "twilio" or "webhook"
	SMSProvider     string
	NotifyLKUserID  string
	NotifyLKAPIKey  string
	NotifyLKSender  string
	TwilioSID       string
	TwilioToken     string
	TwilioSMSFrom   string
	TwilioWAFrom    string
	SMSWebhookURL   string
	SMSWebhookToken string

	// WhatsApp: "log" (simulate), "meta" (WhatsApp Cloud API) or "twilio"
	WAProvider     string
	WAToken        string
	WAPhoneID      string
	WAAPIVersion   string
	WATemplate     string
	WATemplateLang string

	AIModel string
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func loadConfig() Config {
	return Config{
		Addr:         env("ADDR", ":8080"),
		DBPath:       env("DB_PATH", "data/epoch.db"),
		TZ:           env("TZ", "Asia/Colombo"),
		CountryCode:  env("DEFAULT_COUNTRY_CODE", "94"),
		SecureCookie: env("SECURE_COOKIE", "false") == "true",
		SeedDemo:     env("SEED_DEMO", "true") == "true",

		AdminEmail:    env("ADMIN_EMAIL", "admin@epoch.lk"),
		AdminPassword: env("ADMIN_PASSWORD", "epoch123"),

		SMSProvider:     env("SMS_PROVIDER", "log"),
		NotifyLKUserID:  env("NOTIFYLK_USER_ID", ""),
		NotifyLKAPIKey:  env("NOTIFYLK_API_KEY", ""),
		NotifyLKSender:  env("NOTIFYLK_SENDER_ID", "NotifyDEMO"),
		TwilioSID:       env("TWILIO_ACCOUNT_SID", ""),
		TwilioToken:     env("TWILIO_AUTH_TOKEN", ""),
		TwilioSMSFrom:   env("TWILIO_SMS_FROM", ""),
		TwilioWAFrom:    env("TWILIO_WHATSAPP_FROM", ""),
		SMSWebhookURL:   env("SMS_WEBHOOK_URL", ""),
		SMSWebhookToken: env("SMS_WEBHOOK_TOKEN", ""),

		WAProvider:     env("WHATSAPP_PROVIDER", "log"),
		WAToken:        env("WHATSAPP_TOKEN", ""),
		WAPhoneID:      env("WHATSAPP_PHONE_NUMBER_ID", ""),
		WAAPIVersion:   env("WHATSAPP_API_VERSION", "v21.0"),
		WATemplate:     env("WHATSAPP_TEMPLATE", ""),
		WATemplateLang: env("WHATSAPP_TEMPLATE_LANG", "en"),

		AIModel: env("AI_MODEL", "claude-opus-5-5"),
	}
}

// loadDotEnv reads KEY=VALUE lines from a .env file without overriding variables
// that are already set in the environment.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(strings.TrimPrefix(k, "export "))
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}
