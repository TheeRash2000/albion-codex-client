//go:build darwin

// Codex Client tray icon (macOS).

package icon

import (
	_ "embed"
)

//go:embed codex-client.png
var Data []byte
