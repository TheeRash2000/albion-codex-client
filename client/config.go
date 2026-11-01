package client

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ao-data/albiondata-client/log"

	"github.com/mattn/go-colorable"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const maxLogFiles = 1

// logFileName is where the client writes its log. It lives in the per-user config
// dir (%APPDATA%\CodexClient on Windows) named codex-log.txt, so it does not
// clutter the folder the exe runs from. Just one rotated backup is kept.
var logFileName = resolveLogPath()

func resolveLogPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return "codex-log.txt"
	}
	d := filepath.Join(dir, "CodexClient")
	_ = os.MkdirAll(d, 0o700)
	return filepath.Join(d, "codex-log.txt")
}

// ansiStripWriter wraps an io.Writer and strips ANSI escape codes before writing
type ansiStripWriter struct {
	writer io.Writer
	regex  *regexp.Regexp
}

// newAnsiStripWriter creates a writer that strips ANSI escape codes
func newAnsiStripWriter(w io.Writer) *ansiStripWriter {
	return &ansiStripWriter{
		writer: w,
		// Matches ANSI escape sequences like \x1b[0m, \x1b[36m, etc.
		regex: regexp.MustCompile(`\x1b\[[0-9;]*m`),
	}
}

func (w *ansiStripWriter) Write(p []byte) (n int, err error) {
	stripped := w.regex.ReplaceAll(p, []byte{})
	_, err = w.writer.Write(stripped)
	// Return original length to satisfy io.Writer contract
	return len(p), err
}

type config struct {
	AllowedWSHosts                 []string
	Debug                          bool
	Trace                          bool
	DebugEvents                    map[int]bool
	DebugEventsString              string
	DebugEventsBlacklistString     string
	DebugOperations                map[int]bool
	DebugOperationsString          string
	DebugOperationsBlacklistString string
	DebugIgnoreDecodingErrors      bool
	DisableUpload                  bool
	EnableWebsockets               bool
	ListenDevices                  string
	LogLevel                       string
	Minimize                       bool
	Offline                        bool
	OfflinePath                    string
	RecordPath                     string
	PrivateIngestBaseUrls          string
	PublicIngestBaseUrls           string
	NoCPULimit                     bool
	PrintVersion                   bool
	UpdateGithubOwner              string
	UpdateGithubRepo               string
	// Codex: destiny-board upload to the player's Codex account.
	CodexBaseURL      string
	CodexToken        string
	CodexPair         bool
	CodexMarketUpload bool
	CodexOpenPanel    bool
	CodexBackground   bool
	AdminMode         bool
}

// config global config data
var ConfigGlobal = &config{
	LogLevel:          "INFO",
	UpdateGithubOwner: "ao-data",
	UpdateGithubRepo:  "albiondata-client",
	CodexBaseURL:      "https://albioncodex.com"}

func (config *config) SetupFlags() {
	config.setupWebsocketFlags()
	config.setupDebugFlags()
	config.setupCommonFlags()
	config.setupCodexFlags()
	config.setupPanelFlag()

	flag.Parse()

	config.applyCodexEnv()

	// The tray checkboxes, persisted across restarts. Applied after flags so the
	// AODP one composes with an explicit -i rather than replacing it: the stored
	// setting only ever ADDS our placeholder to whatever targets are configured.
	// The market one instead defers to an explicitly passed -codex-market, since
	// a bool cannot compose. See codex_settings.go for both.
	applyStoredAODPSetting()
	applyStoredMarketSetting()

	if config.OfflinePath != "" {
		config.Offline = true
		config.DisableUpload = true

		if config.PublicIngestBaseUrls == "http+pow://west.aodp.local:3000" {
			config.DisableUpload = false
		}

		log.Infof("config.PublicIngestBaseUrls: %v", config.PublicIngestBaseUrls)
		log.Infof("config.DisableUpload: %v", config.DisableUpload)
	}

	if config.DisableUpload {
		log.Info("Upload is disabled.")
	}

	config.setupLogs()
}

func (config *config) setupWebsocketFlags() {
	// Setup the config file and parse values
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()

	// if we cannot find the configuration file, set Websockets to false
	if err != nil {
		viper.Set("EnableWebsockets", false)
	}

	config.EnableWebsockets = viper.GetBool("EnableWebsockets")
	config.AllowedWSHosts = viper.GetStringSlice("AllowedWebsocketHosts")

	// // Keeping for local development, but commenting out so it's not live.
	// // Read update configuration (use defaults if not specified)
	// if viper.IsSet("UpdateGithubOwner") {
	// 	config.UpdateGithubOwner = viper.GetString("UpdateGithubOwner")
	// }
	// if viper.IsSet("UpdateGithubRepo") {
	// 	config.UpdateGithubRepo = viper.GetString("UpdateGithubRepo")
	// }
}

func (config *config) setupDebugFlags() {
	flag.BoolVar(
		&config.PrintVersion,
		"version",
		false,
		"Print version, then close.",
	)

	flag.BoolVar(
		&config.Debug,
		"debug",
		false,
		"Enable debug logging.",
	)

	flag.BoolVar(
		&config.Trace,
		"trace",
		false,
		"Enable trace logging. Even more verbose than debug.",
	)

	flag.StringVar(
		&config.DebugEventsString,
		"events",
		"",
		"Whitelist of event IDs to output messages when debugging. Comma separated.",
	)

	flag.StringVar(
		&config.DebugEventsBlacklistString,
		"events-ignore",
		"",
		"Blacklist of event IDs to hide messages when debugging. Comma separated.",
	)

	flag.StringVar(
		&config.DebugOperationsString,
		"operations",
		"",
		"Whitelist of operation IDs to output messages when debugging. Comma separated.",
	)

	flag.StringVar(
		&config.DebugOperationsBlacklistString,
		"operations-ignore",
		"",
		"Blacklist of operation IDs to hide messages when debugging. Comma separated.",
	)

	flag.BoolVar(
		&config.DebugIgnoreDecodingErrors,
		"ignore-decode-errors",
		false,
		"Ignore the decoding errors when debugging",
	)

	flag.BoolVar(
		&config.NoCPULimit,
		"no-limit",
		false,
		"Use all available CPU cores",
	)

}

func (config *config) setupCommonFlags() {
	flag.BoolVar(
		&config.DisableUpload,
		"d",
		false,
		"If specified no attempts will be made to upload data to remote server.",
	)

	flag.StringVar(
		&config.ListenDevices,
		"l",
		"",
		"Listen on this comma separated devices instead of all available. (Windows: Use MAC-Address, Linux: Use interface name)",
	)

	flag.StringVar(
		&config.OfflinePath,
		"o",
		"",
		"Parses a local file instead of checking albion ports.",
	)

	flag.BoolVar(
		&config.Minimize,
		"minimize",
		false,
		"Automatically minimize the window.",
	)

	flag.StringVar(
		&config.PublicIngestBaseUrls,
		"i",
		"",
		"Base URL to send PUBLIC market data to (third-party, e.g. AODP), can be 'nats://', 'http://' or 'https://', comma separated. Empty by default - Codex does NOT upload to third parties. Pass an AODP URL here to opt in. (Do not use 'noop'; createUploaders panics on short tokens.)",
	)

	flag.StringVar(
		&config.PrivateIngestBaseUrls,
		"p",
		"",
		"Base URL to send PRIVATE data to, can be 'nats://', 'http://', 'https://' or 'noop' and can have multiple uploaders. Comma separated.",
	)

	flag.StringVar(
		&config.RecordPath,
		"record",
		"",
		"Enable recording commands to a file for debugging later.",
	)

	flag.BoolVar(
		&config.AdminMode,
		"admin",
		false,
		"Admin/diagnostic mode: always record this session to a rotating .gob (under ProgramData\\CodexClient\\recordings) so any issue can be replayed and diagnosed later. Normal capture and upload are unchanged.",
	)
}

// setupCodexFlags wires the Codex destiny-board upload config. Precedence (low to
// high): built-in default -> config.yaml (CodexBaseURL / CodexToken) -> CLI flag ->
// environment (CODEX_BASE_URL / CODEX_TOKEN, applied after flag.Parse). viper has
// already read config.yaml in setupWebsocketFlags.
func (config *config) setupCodexFlags() {
	if viper.IsSet("CodexBaseURL") {
		config.CodexBaseURL = viper.GetString("CodexBaseURL")
	}
	if viper.IsSet("CodexToken") {
		config.CodexToken = viper.GetString("CodexToken")
	}

	flag.StringVar(
		&config.CodexBaseURL,
		"codex-url",
		config.CodexBaseURL,
		"Codex base URL the destiny board is uploaded to.",
	)
	flag.StringVar(
		&config.CodexToken,
		"codex-token",
		config.CodexToken,
		"Codex device token from pairing. Prefer the CODEX_TOKEN env var so it stays out of shell history.",
	)
	flag.BoolVar(
		&config.CodexPair,
		"codex-pair",
		false,
		"Link this PC to your Codex account (interactive), save the token, then exit.",
	)
	flag.BoolVar(
		&config.CodexMarketUpload,
		"codex-market",
		true,
		"Contribute market prices you scan in-game to your Codex network (ON by default; pass -codex-market=false to disable).",
	)
}

// applyCodexEnv lets environment variables override config.yaml and flags, and
// falls back to the token saved by `--codex-pair`. Useful for keeping the device
// token out of config files and command lines.
func (config *config) applyCodexEnv() {
	if v := strings.TrimSpace(os.Getenv("CODEX_BASE_URL")); v != "" {
		config.CodexBaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("CODEX_TOKEN")); v != "" {
		config.CodexToken = v
	}
	// Lowest priority: the token persisted by the pairing flow (stable per-user
	// path, with migration from the legacy cwd-relative file).
	if config.CodexToken == "" {
		config.CodexToken = loadCodexToken()
	}
}

func (config *config) setupLogs() {
	if config.Debug {
		config.LogLevel = "DEBUG"
	}
	if config.Trace {
		config.LogLevel = "TRACE"
	}

	level, err := logrus.ParseLevel(strings.ToLower(config.LogLevel))
	if err != nil {
		log.Errorf("Error getting level: %v", err)
	}

	log.SetLevel(level)

	// Rotate existing log files before creating new one
	rotateLogFiles()

	// Always log to both file and terminal
	// Use colors for terminal, strip ANSI codes for file
	log.SetFormatter(&logrus.TextFormatter{FullTimestamp: true, DisableSorting: true, ForceColors: true})
	f, err := os.OpenFile(logFileName, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err == nil {
		// Wrap file writer to strip ANSI codes
		strippedFileWriter := newAnsiStripWriter(f)
		multiWriter := io.MultiWriter(colorable.NewColorableStdout(), strippedFileWriter)
		log.SetOutput(multiWriter)
	} else {
		log.SetOutput(colorable.NewColorableStdout())
		log.Warnf("Could not create log file: %v", err)
	}
}

// rotateLogFiles moves the current log file to a numbered backup and removes old backups
func rotateLogFiles() {
	// Check if current log file exists
	if _, err := os.Stat(logFileName); os.IsNotExist(err) {
		return // No log file to rotate
	}

	// Remove the oldest log file if we're at the limit
	oldestLog := fmt.Sprintf("%s.%d", logFileName, maxLogFiles)
	_ = os.Remove(oldestLog)

	// Shift all existing log files up by one number
	for i := maxLogFiles - 1; i >= 1; i-- {
		oldName := fmt.Sprintf("%s.%d", logFileName, i)
		newName := fmt.Sprintf("%s.%d", logFileName, i+1)
		_ = os.Rename(oldName, newName)
	}

	// Rename current log file to .1
	_ = os.Rename(logFileName, fmt.Sprintf("%s.1", logFileName))
}

// GetLogFilePath returns the full path to the current log file
func GetLogFilePath() string {
	absPath, err := filepath.Abs(logFileName)
	if err != nil {
		return logFileName
	}
	return absPath
}

func (config *config) setupDebugEvents() {
	config.DebugEvents = make(map[int]bool)
	if config.DebugEventsString != "" {
		for _, event := range strings.Split(config.DebugEventsString, ",") {
			number, err := strconv.Atoi(event)
			if err == nil {
				config.DebugEvents[number] = true
			}
		}
	}
	if config.DebugEventsBlacklistString != "" {
		for _, event := range strings.Split(config.DebugEventsBlacklistString, ",") {
			number, err := strconv.Atoi(event)
			if err == nil {
				config.DebugEvents[number] = false
			}
		}
	}

	// Looping through map keys is purposefully random by design in Go
	for number, shouldDebug := range config.DebugEvents {
		verb := "Ignoring"
		if shouldDebug {
			verb = "Showing"
		}
		log.Debugf("[%v] event: [%v]%v", verb, number, EventType(number))
	}

}

func (config *config) setupDebugOperations() {
	config.DebugOperations = make(map[int]bool)
	if config.DebugOperationsString != "" {
		for _, operation := range strings.Split(config.DebugOperationsString, ",") {
			number, err := strconv.Atoi(operation)
			if err == nil {
				config.DebugOperations[number] = true
			}
		}
	}

	if config.DebugOperationsBlacklistString != "" {
		for _, operation := range strings.Split(config.DebugOperationsBlacklistString, ",") {
			number, err := strconv.Atoi(operation)
			if err == nil {
				config.DebugOperations[number] = false
			}
		}
	}

	// Looping through map keys is purposefully random by design in Go
	for number, shouldDebug := range config.DebugOperations {
		verb := "Ignoring"
		if shouldDebug {
			verb = "Showing"
		}
		log.Debugf("[%v] operation: [%v]%v", verb, number, OperationType(number))
	}

}

// setupPanelFlag adds the one flag that is purely a UI convenience: open the
// panel window at startup instead of waiting for the tray. Useful for support
// ("run it with -panel and tell me what the window says") and for anyone who
// wants the window up the moment the client launches.
func (config *config) setupPanelFlag() {
	flag.BoolVar(
		&config.CodexOpenPanel,
		"panel",
		false,
		"Open the Codex Client panel window on start (normally opened from the tray).",
	)
	flag.BoolVar(
		&config.CodexBackground,
		backgroundFlag,
		false,
		"Accepted for older autostart entries; it no longer keeps the window closed. Every launch opens the window.",
	)
}

// backgroundFlag is still parsed because the Windows autostart entry written
// by earlier builds passes it, and an unknown flag would make that login
// launch exit. It no longer changes anything: since 0.1.18 every launch opens
// the window, the login autostart and the restart after a self-update
// included. A start that only adds a tray icon looks like nothing happened,
// and a player who never sees the window never finds "Link this PC".
const backgroundFlag = "background"

// shouldOpenPanelOnStart decides whether startup opens the window. It always
// does. The parameters stay so the launch kinds remain named in the test and
// at the call site, and so a future exception has somewhere to live.
func shouldOpenPanelOnStart(openPanel, background, linked bool) bool {
	return true
}
