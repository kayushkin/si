package config

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kayushkin/llm-bridge/msg"
	"github.com/kayushkin/llm-bridge/servicesettings"
)

// repositoryRoot is where the source scan starts: this package is
// internal/config.
const repositoryRoot = "../.."

// The registry gives cmd/si what its os.Getenv reads gave it before
// 2026-09-24: the same defaults with nothing set, and the operator's values
// when they are.
func TestTheRegistryReadsTheSameValuesTheCommandAlwaysDid(t *testing.T) {
	unset, err := NewSettingsRegistry(servicesettings.MapEnvironment(map[string]string{}))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		SettingFeedMode:               "nats",
		SettingNATSURL:                "nats://localhost:4222",
		SettingWebSocketListenAddress: ":8090",
		SettingWebSocketBearerToken:   "",
		SettingWebSocketJWTSecret:     "",
		SettingDiscordBotToken:        "",
		SettingDiscordChannelID:       "143132977210195968",
		SettingLogstackURL:            "http://localhost:8088",
	} {
		if got := unset.String(key); got != want {
			t.Errorf("%s with nothing set = %q, want %q", key, got, want)
		}
	}

	variables := map[string]string{
		"SI_FEED":            "echo",
		"NATS_URL":           "nats://bus:4223",
		"SI_WS_ADDR":         "127.0.0.1:8090",
		"SI_WS_TOKEN":        "bearer",
		"SI_JWT_SECRET":      "hmac",
		"SI_DISCORD_TOKEN":   "bot",
		"SI_DISCORD_CHANNEL": "42",
		"LOGSTACK_URL":       "http://127.0.0.1:1",
	}
	set, err := NewSettingsRegistry(servicesettings.MapEnvironment(variables))
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range SettingDefinitions() {
		if got, want := set.String(definition.Key), variables[definition.EnvironmentVariable]; got != want {
			t.Errorf("%s = %q, want %q from %s", definition.Key, got, want, definition.EnvironmentVariable)
		}
	}
	if err := set.CheckRequired(); err != nil {
		t.Errorf("CheckRequired refused: %v", err)
	}

	// A variable set to the empty string is the same as unset, as it was when
	// cmd/si compared os.Getenv to "".
	empty, err := NewSettingsRegistry(servicesettings.MapEnvironment(map[string]string{"SI_FEED": "", "SI_WS_ADDR": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if empty.String(SettingFeedMode) != "nats" || empty.String(SettingWebSocketListenAddress) != ":8090" {
		t.Errorf("empty variables: feed=%q address=%q", empty.String(SettingFeedMode), empty.String(SettingWebSocketListenAddress))
	}
}

func TestTheRegistryRefusesAMisspelledSiVariableAndNotOneMeantForAnotherService(t *testing.T) {
	for _, misspelled := range []string{"SI_FEED_MODE", "SI_WS_TOKENS", "SI_DISCORD_CHANNEL_ID"} {
		_, err := NewSettingsRegistry(servicesettings.MapEnvironment(map[string]string{misspelled: "x"}))
		if err == nil || !strings.Contains(err.Error(), misspelled+" is set and si declares no such setting") {
			t.Errorf("%s: NewSettingsRegistry = %v, want a refusal naming it", misspelled, err)
		}
	}
	// SI_WS_URL is how kayushkin.com finds si; the deploy script's own flags
	// and the other services' names are not si's either.
	notOurs := map[string]string{
		"SI_WS_URL":             "ws://127.0.0.1:8090/ws",
		"SI_BUS_URL":            "x",
		"SI_DEPLOY_ALLOW_DIRTY": "1",
		"PATH":                  "/bin",
		"HOME":                  "/root",
	}
	if _, err := NewSettingsRegistry(servicesettings.MapEnvironment(notOurs)); err != nil {
		t.Errorf("variables meant for others were refused: %v", err)
	}
}

// Every owned prefix is the start of a declared variable. An owned prefix that
// starts nothing guards nothing and reads as if it did.
func TestEveryOwnedPrefixStartsADeclaredVariable(t *testing.T) {
	for _, prefix := range OwnedEnvironmentVariablePrefixes {
		found := false
		for _, definition := range SettingDefinitions() {
			found = found || strings.HasPrefix(definition.EnvironmentVariable, prefix)
		}
		if !found {
			t.Errorf("owned prefix %s starts no declared variable", prefix)
		}
	}
}

// New quotes a value it cannot parse. A secret must be a string, which always
// parses, or its value could reach the log through its own refusal.
func TestEverySecretIsAStringWithNoDefault(t *testing.T) {
	var secrets []string
	for _, definition := range SettingDefinitions() {
		if definition.Kind != msg.ServiceSettingKindSecret {
			continue
		}
		secrets = append(secrets, definition.EnvironmentVariable)
		if definition.ValueType != msg.ServiceSettingValueTypeString || definition.Default != "" {
			t.Errorf("%s is a secret of type %s with default %q", definition.EnvironmentVariable, definition.ValueType, definition.Default)
		}
	}
	if got := strings.Join(secrets, " "); got != "SI_WS_TOKEN SI_JWT_SECRET SI_DISCORD_TOKEN" {
		t.Errorf("secrets = %s", got)
	}
}

func TestGetSettingsDescribesTheServiceAndShowsNoSecret(t *testing.T) {
	registry, err := NewSettingsRegistry(servicesettings.MapEnvironment(map[string]string{"SI_WS_ADDR": "127.0.0.1:8090", "SI_DISCORD_TOKEN": "the-bot-token"}))
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	servicesettings.Handler(registry, "/settings").ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /settings = %d: %s", recorder.Code, recorder.Body)
	}
	if strings.Contains(recorder.Body.String(), "the-bot-token") {
		t.Fatal("GET /settings served the Discord token")
	}
	var described msg.ServiceSettings
	if err := json.Unmarshal(recorder.Body.Bytes(), &described); err != nil {
		t.Fatal(err)
	}
	if described.Service != ServiceName || len(described.Settings) != len(SettingDefinitions()) {
		t.Fatalf("service=%q with %d settings, want %q with %d", described.Service, len(described.Settings), ServiceName, len(SettingDefinitions()))
	}
	for _, setting := range described.Settings {
		if setting.Editable {
			t.Errorf("%s is editable, and si reads every setting once at start", setting.Key)
		}
		if setting.Key == SettingWebSocketListenAddress && (setting.Value != "127.0.0.1:8090" || setting.Source != msg.ServiceSettingSourceEnvironment) {
			t.Errorf("listen address served as %q from %q", setting.Value, setting.Source)
		}
	}
}

// Every environment variable si's own code reads by name is declared. A read
// that is not declared is invisible on the settings page and escapes the
// startup check. The walk is scheduler's environmentReadFaults, as marginalia
// copied it.
func TestEveryEnvironmentVariableTheServiceReadsIsDeclared(t *testing.T) {
	declared := map[string]bool{}
	for _, definition := range SettingDefinitions() {
		declared[definition.EnvironmentVariable] = true
	}

	filesRead := 0
	err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == "node_modules" || entry.Name() == ".git") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		filesRead++
		for _, fault := range environmentReadFaults(file, declared) {
			t.Errorf("%s %s", path, fault)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// If this package moved, the walk would start somewhere else, read nothing
	// and pass.
	if _, err := os.Stat(filepath.Join(repositoryRoot, "cmd", "si", "main.go")); err != nil {
		t.Fatalf("the scan starts somewhere that is not the repository root: %v", err)
	}
	if filesRead < 15 {
		t.Fatalf("the scan read %d files; it is not looking at the service", filesRead)
	}
}

// The scan's own controls: each shape it exists to refuse is refused.
func TestTheSourceScanRefusesEachShapeOfUndeclaredRead(t *testing.T) {
	declared := map[string]bool{"SI_FEED": true}
	for name, source := range map[string]string{
		"an undeclared name":      `package p; import "os"; var v = os.Getenv("SI_FEEDS")`,
		"a computed name":         `package p; import "os"; var n = "X"; var v = os.Getenv(n)`,
		"the whole environment":   `package p; import "os"; var v = os.Environ()`,
		"os.Getenv as a value":    `package p; import "os"; var read = os.Getenv`,
		"an undeclared LookupEnv": `package p; import "os"; func f() { os.LookupEnv("OTHER") }`,
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "control.go", source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if faults := environmentReadFaults(file, declared); len(faults) == 0 {
			t.Errorf("%s: the scan found nothing", name)
		}
	}
	file, err := parser.ParseFile(token.NewFileSet(), "control.go", `package p; import "os"; var v = os.Getenv("SI_FEED")`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if faults := environmentReadFaults(file, declared); len(faults) != 0 {
		t.Errorf("a declared read was refused: %v", faults)
	}
}

func environmentReadFaults(file *ast.File, declared map[string]bool) []string {
	var faults []string
	called := map[*ast.SelectorExpr]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		selector, isSelector := call.Fun.(*ast.SelectorExpr)
		if !isSelector || !isOsFunction(selector, "Getenv", "LookupEnv") {
			return true
		}
		called[selector] = true
		literal, isLiteral := call.Args[0].(*ast.BasicLit)
		if !isLiteral {
			faults = append(faults, "reads an environment variable whose name is computed, which no declaration can be held to")
			return true
		}
		name, _ := strconv.Unquote(literal.Value)
		if !declared[name] {
			faults = append(faults, "reads "+name+", which SettingDefinitions does not declare")
		}
		return true
	})
	ast.Inspect(file, func(node ast.Node) bool {
		selector, isSelector := node.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		if isOsFunction(selector, "Environ") {
			faults = append(faults, "reads the whole environment, which no declaration can be held to")
		}
		if isOsFunction(selector, "Getenv", "LookupEnv") && !called[selector] {
			faults = append(faults, "hands os."+selector.Sel.Name+" on as a value, so the names it reads cannot be seen here")
		}
		return true
	})
	return faults
}

func isOsFunction(selector *ast.SelectorExpr, names ...string) bool {
	packageName, isIdentifier := selector.X.(*ast.Ident)
	if !isIdentifier || packageName.Name != "os" {
		return false
	}
	for _, name := range names {
		if selector.Sel.Name == name {
			return true
		}
	}
	return false
}
