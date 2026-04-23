module github.com/kayushkin/si

go 1.25.0

replace github.com/kayushkin/bus => ../bus

replace github.com/kayushkin/llm-bridge => ../llm-bridge

require (
	github.com/bwmarrin/discordgo v0.29.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/kayushkin/bus v0.0.0-00010101000000-000000000000
)

require (
	github.com/kayushkin/llm-bridge v0.0.0-00010101000000-000000000000 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/nats-io/nats.go v1.49.0 // indirect
	github.com/nats-io/nkeys v0.4.12 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
)
