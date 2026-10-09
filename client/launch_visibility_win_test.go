//go:build windows

package client

import (
	"flag"
	"fmt"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The Run-key value must come out of reg.exe as exactly `"<exe>" -background`.
// Decoded with the Windows command-line parser (CommandLineToArgvW), which is how
// reg.exe reads its arguments.
func TestAutostartRegCommandLine(t *testing.T) {
	for _, exe := range []string{
		`C:\Program Files\Codex Client\codex-client.exe`,
		`C:\Users\Player\OneDrive\Desktop\codex-client.exe`,
		`D:\games\codex.exe`,
	} {
		args, err := windows.DecomposeCommandLine(autostartRegCmdLine(exe))
		if err != nil {
			t.Fatalf("%s: %v", exe, err)
		}
		want := []string{"reg", "add", autostartRunKey, "/v", autostartName, "/t", "REG_SZ", "/d", autostartValue(exe), "/f"}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("%s:\n got  %q\n want %q", exe, args, want)
		}
		if v := autostartValue(exe); v != `"`+exe+`" -background` {
			t.Errorf("stored value = %q", v)
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
