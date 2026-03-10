# Si build commands
# INBER_ENV is injected by bus-agent (0, 1, 2) or defaults to 0

env := env_var_or_default("INBER_ENV", "0")
base_port := if env == "0" { "9020" } else if env == "1" { "9120" } else { "9220" }
bus_port := if env == "0" { "9010" } else if env == "1" { "9110" } else { "9210" }

# Build si binary
build:
  go build -o si .

# Run si on environment's port
dev: build
  SI_WS_ADDR=":{{base_port}}" \
  SI_BUS_URL="http://127.0.0.1:{{bus_port}}" \
  SI_BUS_TOKEN="inber-bus-secret-2026" \
  ./si

# Run tests
test:
  go test ./...

# Clean
clean:
  rm -f si
