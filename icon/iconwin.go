//go:build windows

// Codex Client tray icon (Windows). The systray needs ICO bytes here.
package icon

import _ "embed"

//go:embed codex-client.ico
var Data []byte
