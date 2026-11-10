//go:build windows

package client

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The autostart value must be stored in the registry as exactly
// `"<exe>" -background`, quotes included, and read back unchanged. Written to a
// throwaway key under HKCU\Software, never the real Run key.
func TestAutostartValueRoundTripsThroughRegistry(t *testing.T) {
	subkey := fmt.Sprintf(`Software\CodexClientTest-%d`, time.Now().UnixNano())
	keyPath, err := syscall.UTF16PtrFromString(subkey)
	if err != nil {
		t.Fatal(err)
	}
	defer modadvapi32.NewProc("RegDeleteKeyW").Call(
		uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(keyPath)))

	if _, ok := readUserRegString(subkey, autostartName); ok {
		t.Fatal("value present before it was written")
	}
	for _, exe := range []string{
		`C:\Program Files\Codex Client\codex-client.exe`,
		`C:\Users\Player\OneDrive\Desktop\codex-client.exe`,
		`D:\games\codex.exe`,
	} {
		want := autostartValue(exe)
		if want != `"`+exe+`" -background` {
			t.Errorf("stored value = %q", want)
		}
		// Each write replaces the previous one, as when the exe is moved.
		if err := writeUserRegString(subkey, autostartName, want); err != nil {
			t.Fatalf("%s: %v", exe, err)
		}
		got, ok := readUserRegString(subkey, autostartName)
		if !ok || got != want {
			t.Errorf("%s:\n got  %q (found=%v)\n want %q", exe, got, ok, want)
		}
	}
}

// Registering autostart must not start another program. Spawning a hidden
// reg.exe to write the Run key is what antivirus heuristics flag as persistence.
func TestAutostartDoesNotSpawnProcesses(t *testing.T) {
	src, err := os.ReadFile("autostart_win.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{`"os/exec"`, "exec.Command", "HideWindow"} {
		if strings.Contains(string(src), banned) {
			t.Errorf("autostart_win.go uses %s", banned)
		}
	}
}

// What Windows runs at login (the stored value) is the exe plus -background, and
// the client's own flag parsing reads it as a background launch.
func TestAutostartValueLaunchesInBackground(t *testing.T) {
	exe := `C:\Program Files\Codex Client\codex-client.exe`
	argv, err := windows.DecomposeCommandLine(autostartValue(exe))
	if err != nil {
		t.Fatal(err)
	}
	if len(argv) != 2 || argv[0] != exe {
		t.Fatalf("login launch argv = %q", argv)
	}
	fs := flag.NewFlagSet("codex", flag.ContinueOnError)
	bg := fs.Bool(backgroundFlag, false, "")
	if err := fs.Parse(argv[1:]); err != nil || !*bg {
		t.Fatalf("login launch is not a background launch (bg=%v, err=%v)", *bg, err)
	}
}

// A second launch's signal reaches the running client and asks for the window.
func TestSecondLaunchShowsWindow(t *testing.T) {
	prev := showWindowEventName
	showWindowEventName = fmt.Sprintf("CodexClientShowWindowTest-%d", time.Now().UnixNano())
	defer func() { showWindowEventName = prev }()

	if signalShowWindow() {
		t.Fatal("signal delivered with no running client listening")
	}

	shown := make(chan struct{}, 2)
	listenForShowWindow(func() { shown <- struct{}{} })

	for i := 0; i < 2; i++ { // works every time, not just once
		if !signalShowWindow() {
			t.Fatalf("signal %d not delivered", i+1)
		}
		select {
		case <-shown:
		case <-time.After(2 * time.Second):
			t.Fatalf("signal %d did not open the window", i+1)
		}
	}
}
