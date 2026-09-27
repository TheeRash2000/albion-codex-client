//go:build !windows

package client

// The panel window is Windows-only.
//
// WebView2 is a Windows component, and the Linux client is the headless CLI
// (systray.Run is a no-op there too). The local HTTP server still starts, so a
// Linux user who wants the page can open panelURL in their own browser - but
// nothing here launches one, which is the behaviour the Windows side exists to
// avoid.

// OpenPanel is a no-op off Windows.
func OpenPanel() {}

// PanelWindowOpen always reports false off Windows.
func PanelWindowOpen() bool { return false }
