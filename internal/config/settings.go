// Package config declares every environment variable si reads, once, with
// llm-bridge's servicesettings. cmd/si reads its configuration from the
// registry; GET /settings describes the service from it; and a test holds every
// os.Getenv in the repo to it.
package config

import (
	"github.com/kayushkin/llm-bridge/msg"
	"github.com/kayushkin/llm-bridge/servicesettings"
)

// ServiceName is this service's name in its own settings description.
const ServiceName = "si"

// OwnedEnvironmentVariablePrefixes are the prefixes of the variables that are
// si's alone. A set variable carrying one that SettingDefinitions does not
// declare stops si from starting: it is a misspelling or a leftover, and
// either way someone believes it does something.
//
// Each is a full variable name, not "SI_". SI_WS_URL is how kayushkin.com finds
// si, so one shared environment file carrying it would stop si over a variable
// that was never meant for it. So SI_FEED_MODE and SI_WS_TOKENS are refused,
// and SI_WS_URL is not. NATS_URL and LOGSTACK_URL are declared and not owned:
// other services read the same names.
var OwnedEnvironmentVariablePrefixes = []string{
	"SI_FEED",
	"SI_WS_ADDR",
	"SI_WS_TOKEN",
	"SI_JWT_SECRET",
	"SI_DISCORD_TOKEN",
	"SI_DISCORD_CHANNEL",
}

// Keys of the settings, as GET /settings names them.
const (
	SettingFeedMode               = "feed_mode"
	SettingNATSURL                = "nats_url"
	SettingWebSocketListenAddress = "websocket_listen_address"
	SettingWebSocketBearerToken   = "websocket_bearer_token"
	SettingWebSocketJWTSecret     = "websocket_jwt_secret"
	SettingDiscordBotToken        = "discord_bot_token"
	SettingDiscordChannelID       = "discord_channel_id"
	SettingLogstackURL            = "logstack_url"
)

// SettingDefinitions declares every environment variable this process reads.
//
// Nothing here is Editable: every value is read once at start, and a write
// would change a description and not the running service.
func SettingDefinitions() []servicesettings.Definition {
	return []servicesettings.Definition{
		{Key: SettingFeedMode, EnvironmentVariable: "SI_FEED", Kind: msg.ServiceSettingKindBehaviour, ValueType: msg.ServiceSettingValueTypeString, Default: "nats",
			Description: "Where messages to and from agents travel: nats (the bus), bus (the same NATS feed under its old name) or echo (an in-process loop for tests, which reaches no agent). Any other value stops si at start."},
		{Key: SettingNATSURL, EnvironmentVariable: "NATS_URL", Kind: msg.ServiceSettingKindWiring, ValueType: msg.ServiceSettingValueTypeString, Default: "nats://localhost:4222",
			Description: "The NATS server the nats and bus feeds connect to. Changing it moves every agent conversation si carries to that server; si stops at start if it cannot connect. The echo feed does not read it."},
		{Key: SettingWebSocketListenAddress, EnvironmentVariable: "SI_WS_ADDR", Kind: msg.ServiceSettingKindWiring, ValueType: msg.ServiceSettingValueTypeString, Default: ":8090",
			Description: "The host:port the websocket adapter listens on for /ws, /api/status and /settings. The default binds every interface; the shipped unit binds loopback because /ws is open when neither token is set. Changing it moves si for dash, kayushkin.com and android-bridge."},
		{Key: SettingWebSocketBearerToken, EnvironmentVariable: "SI_WS_TOKEN", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "A shared bearer token websocket clients may present. With it and the JWT secret both unset, the websocket routes accept anyone. Changing it disconnects nothing but refuses the next connection that presents the old one."},
		{Key: SettingWebSocketJWTSecret, EnvironmentVariable: "SI_JWT_SECRET", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The HMAC secret that verifies JWTs with aud=si, as kayushkin.com mints them for android-bridge. It must match kayushkin.com's SI_JWT_SECRET, or every token it mints is refused."},
		{Key: SettingDiscordBotToken, EnvironmentVariable: "SI_DISCORD_TOKEN", Kind: msg.ServiceSettingKindSecret, ValueType: msg.ServiceSettingValueTypeString,
			Description: "The Discord bot token. Set, it starts the Discord adapter; unset, si carries no Discord traffic."},
		{Key: SettingDiscordChannelID, EnvironmentVariable: "SI_DISCORD_CHANNEL", Kind: msg.ServiceSettingKindWiring, ValueType: msg.ServiceSettingValueTypeString, Default: "143132977210195968",
			Description: "The Discord channel the adapter posts to, by id. The default is the Pretend server's channel. Read only when the bot token is set."},
		{Key: SettingLogstackURL, EnvironmentVariable: "LOGSTACK_URL", Kind: msg.ServiceSettingKindWiring, ValueType: msg.ServiceSettingValueTypeString, Default: "http://localhost:8088",
			Description: "The logstack every routed message is written to. si probes it once at start and writes no history for the rest of its life if it does not answer."},
	}
}

// NewSettingsRegistry reads si's settings from environment. It fails on a set
// variable under an owned prefix that nobody declared. No setting is required.
func NewSettingsRegistry(environment servicesettings.Environment) (*servicesettings.Registry, error) {
	return servicesettings.New(ServiceName, OwnedEnvironmentVariablePrefixes, SettingDefinitions(), environment)
}
