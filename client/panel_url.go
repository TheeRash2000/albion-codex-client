package client

import (
	"os"
	"path/filepath"
	"strings"
)

// The running instance records its control-panel URL here so a second launch
// (blocked by the single-instance guard) can reopen the SAME instance's window
// instead of starting a duplicate.
func panelURLFile() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}
	d := filepath.Join(dir, "CodexClient")
	_ = os.MkdirAll(d, 0o700)
	return filepath.Join(d, "panel-url")
}

func writePanelURL(u string) {
	if p := panelURLFile(); p != "" {
		_ = os.WriteFile(p, []byte(u), 0o600)
	}
}

func readPanelURL() string {
	p := panelURLFile()
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
