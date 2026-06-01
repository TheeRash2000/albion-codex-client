# Codex Client

The Codex Client is a small desktop agent for [Albion Codex](https://albioncodex.com).
It reads your destiny board straight from the game's network traffic and uploads it to
your Codex account, so you never have to type your spec levels in by hand.

It is a fork of the MIT-licensed
[ao-data/albiondata-client](https://github.com/ao-data/albiondata-client). See
[CODEX.md](CODEX.md) for the technical specifics of what this fork changes.

## Is this allowed?

Yes - it only *reads* network packets, it never modifies the game or interferes with it.
Sandbox Interactive's stated position on packet analysis:

> As long as you just look and analyze we are ok with it. The moment you modify or
> manipulate something [...] we will react.
>
> ~ MadDave, Technical Lead for Albion Online
> ([source](https://forum.albiononline.com/index.php/Thread/51604-Is-it-allowed-to-scan-your-internet-trafic-and-pick-up-logs/?postID=512670#post512670))

This is a third-party application and is in no way affiliated with Sandbox Interactive
or Albion Online.

## What it captures

- **Your destiny board** - every mastery and specialization level. That's it for now.

Market scanning (the upstream client's job) is **off by default** in the Codex Client.
You can opt back in with `-i <url>` if you want to contribute to a market project.

## Setup (Windows)

1. **Install [Npcap](https://npcap.com/#download)** (the packet-capture driver). Use the
   default options. This is the only prerequisite.
2. **Download `codex-client.exe`** and put it in a folder of its own (it writes a small
   `codex-token` and log file next to itself).
3. **Link it to your account** - run once:
   ```
   codex-client.exe --codex-pair
   ```
   It shows a short code and opens your Codex link page. Sign in, enter the code, done.
   The device token is saved to `codex-token` so you only pair once.
4. **Run it and log in to Albion.** Start `codex-client.exe`, then log in to the game
   (or relog if you're already in). Your board is pushed by the game at login, the client
   catches it and uploads it. Open your destiny board on Codex - your specs are there.

The client lives in the system tray (look for the Codex icon). Right-click it to link a
device, open your Codex account, show/hide the console, or quit.

> The game only sends the full board **once per session, at login.** Start the client
> *before* you log in (or relog with it running) so it catches the push.

## Configuration

Precedence, lowest to highest: built-in default -> `config.yaml` -> command-line flag ->
environment variable.

| Setting | Flag | Env | Default |
|---------|------|-----|---------|
| Codex URL | `--codex-url` | `CODEX_BASE_URL` | `https://albioncodex.com` |
| Device token | `--codex-token` | `CODEX_TOKEN` | (from `codex-token` after pairing) |
| Market upload | `-i <url>` | - | `noop` (off) |

Keep your token in the `codex-token` file or `CODEX_TOKEN`; avoid passing it on the
command line where it can land in your shell history.

## Privacy

- The only thing uploaded is your destiny board (mastery/spec levels) and your character
  name, tied to your Codex account by a device token.
- The token is stored on Codex only as a SHA-256 hash; the raw token lives on your PC.
- Revoke a device any time from your Codex account page.

## Build from source

Requires Go, a mingw-w64 gcc, and the [Npcap SDK](https://npcap.com/#download) (the
capture layer uses cgo).

```powershell
# auto-detects the toolchain; pass a version if you like
pwsh scripts/build-codex.ps1 -Version 0.1.0
```

## License

MIT. The upstream copyright (The Albion Data Project) is retained in [LICENSE](LICENSE).
