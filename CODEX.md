# Codex Client

A fork of [ao-data/albiondata-client](https://github.com/ao-data/albiondata-client)
(MIT) that captures a player's Albion data (destiny board + market data) and uploads it
to their Codex account at <https://albioncodex.com>. The upstream `LICENSE` (MIT, The
Albion Data Project) is retained in full as required; this file documents only the
Codex-specific additions.

## What this fork adds

| Area | Change | File |
|------|--------|------|
| Destiny board capture | Decodes evFullAchievementInfo (event 150 since the 2026-10-12 game patch; 151 before) -> node id + level. | `client/decode.go`, `client/event_full_achievement_info.go` |
| Board upload | POSTs `{ character: { name, id, skills: { nodeId: level } } }` to `/api/client/ingest`. | `client/codex_upload.go` |
| **Market prices** | Uploads observed sell/buy orders (per item/city/quality) to `/api/client/market`; the tray activity feed logs each as "item - sell/buy price". | `client/codex_market.go` |
| **Market history** | Uploads the game's own price history (AuctionGetItemAverageStats) to `/api/client/market-history`. | `client/codex_history.go` |
| **Gold prices** | Uploads gold/silver exchange history (GoldMarketGetAverageInfo) to `/api/client/gold`. | `client/codex_gold.go` |
| **EMV** | Uploads the server's Estimated Market Value (evEstimatedMarketValueUpdate, event 465 since 2026-10-12; 464 in 0.1.17 and earlier) to `/api/client/emv`; activity line resolves the numeric id to a name via an embedded map. | `client/event_estimated_market_value.go`, `client/item_names.go` + `game_item_ids.json` |
| **Realized sales** | On a finished sell-order mail, uploads the realized sale (item, qty, price) to `/api/client/sales`. | `client/codex_sales.go`, `client/operation_read_mail.go` |
| Device pairing | GitHub-style device flow (`--codex-pair` or the tray "Link this PC" item) exchanges a code for a token; persisted across launches. | `client/codex_pair.go` |
| Token persistence | Saved to `%PROGRAMDATA%\CodexClient` (machine-wide, survives folder moves/elevation/OneDrive) + per-user config + exe dir. | `client/codex_pair.go` |
| Tray UI | Native system-tray menu: linked status, "seeing your game", last board upload, capture health, recent activity, issues, the market/AODP toggles and Link. No browser. | `systray/menu.go`, `client/ui.go`, `client/status.go` |
| Autostart + single instance | HKCU Run-key autostart with `-background` (refuses volatile temp paths); a named mutex keeps one instance, reopening the window on a duplicate launch. The window opens on every start, that login autostart included (`-background` is accepted and ignored since 0.1.18). | `client/autostart_win.go`, `client/single_instance_win.go`, `client/config.go` |
| Config | `CodexBaseURL` / `CodexToken` (config.yaml / `--codex-url`/`--codex-token` / env), `--codex-market` (market upload, ON by default). | `client/config.go` |

Market, history, gold, EMV, sales and purchase uploads are on by default
(`--codex-market=false` turns them off) and only happen when the client is paired.

The site that receives these uploads is <https://albioncodex.com>. It is not part of
this repository.

## Build

Requires Go. On Windows:

```powershell
# No gcc or Npcap SDK needed: nothing in the Windows build uses cgo.
./scripts/build-codex.ps1 -Version dev -NoCgo
```

Adding NSIS to the machine also builds the installer (`codex-client-setup.exe`) when
the version is numeric. Linux: `./scripts/build-codex-linux.sh` (needs `libpcap-dev`).

Releases are built by GitHub Actions from a version tag
(`.github/workflows/release.yml`). The tag's own message becomes the one-line
"what's new" note players see after updating:

```bash
git tag -a alpha-0.1.24 -m "One line for players"
```

The workflow prepares a draft release; nothing reaches players until it is published.

## Configuration

Lowest to highest precedence: built-in default -> `config.yaml` -> CLI flag -> env var.

```yaml
# config.yaml
CodexBaseURL: https://albioncodex.com
# CodexToken:  prefer the env var below instead of writing the token here
```

```sh
# preferred: keep the token out of files and shell history
CODEX_TOKEN=<device-token> ./codex-client
```

Without a token the client runs normally and simply skips the Codex upload (one debug
line), so it is safe to run unpaired.

## Capture is passive + the game caches

The client only reads what the game actually sends. Prices flow whenever a market is
browsed (order data is always fresh); market **history** only when the game re-fetches
it (it caches client-side, like the board) - so the player should click the timescale
tabs (4 weeks is deepest); **EMV** fires from the inventory tooltip, not the marketplace
screen. The board (events 151/154) is pushed once at login / zone change, not on opening
the board screen.

## Keeping up with upstream

This fork has diverged from upstream structurally, so changes are ported, not merged.
`.github/workflows/upstream-watch.yml` checks ao-data/albiondata-client once a day
against the commit recorded in `.github/upstream-synced`, opens an issue listing new
upstream commits, and opens a pull request when the game code tables differ.

## Version history

- **0.1.5 to 0.1.9** - machine-wide device token; per-item market activity feed;
  market history, gold and Estimated Market Value capture; realized sales from
  sell-order mail.
- **0.1.10** - a captured Destiny Board is bound to the character active at capture.
  The client reports its build to the site. Auto-update from a published manifest,
  verified by SHA-256.
- **0.1.11** - market browses captured before the zone is known are held and sent,
  not dropped. Admin mode (`-admin`) records the session for replay. Capture-health
  panel.
- **0.1.12** - realized-sales capture fix.
- **0.1.13** - a revoked device is detected and offered a re-link.
- **0.1.14** - upload queue with retry and backoff; a failed upload is delivered
  later instead of lost.
- **0.1.15** - realized purchases from finished buy-order mail. Market data is also
  contributed to the Albion Online Data Project, with a switch to turn that off.
- **0.1.16** - native system-tray UI and a WebView2 panel window in place of the
  browser control panel. Linux build script.
- **0.1.17** - a malformed market order no longer stops the client. Market
  heartbeat. The window opens on launch.
- **0.1.18** - ports upstream fixes up to ao-data commit c6edd94 (2026-10-12): game
  code tables, larger capture snapshot length, bounded debug slices, uploader
  connection leaks, network interface handling. The tray shows the character from
  the join response.
- **0.1.19** - purchases were recorded as sales. The game moved the city, mail type
  and time columns of the mailbox list along by one (6, 10, 11 to 7, 11, 12); the
  list failed to decode, no mail had a type, and a fallback filed every finished
  order as a sale. The columns are read from their new positions, and a mail is a
  sale or a purchase only when its type says so. Also: autostart is written through
  the registry API instead of a hidden `reg.exe`; updates are looked for on this
  repository's GitHub releases first; a per-user installer; release builds without
  cgo and with `-trimpath`.
- **0.1.20** - the panel's Codex+ row shows the owner's contribution towards earning
  Codex+. Order lists are counted by each order's own type ("38 sell orders and 12
  buy orders"); the game answers buy-order requests with the sell-offer response.
  Finished auctions are left out of uploads. A one-line "what's new" note after an
  update: the new build detects the update itself and reads the note from the
  published manifest. The User-Agent carries the running version.
- **0.1.21** - the Codex+ row shows where the owner stands among contributors, for
  the top half only.
- **0.1.22** - ledger card: what the owner sold and bought on the market over 7 and
  30 days. Hidden when the site sends no totals; no profit figure.
- **0.1.23** - a sale is recorded at what it realized (the mail's total divided by
  the quantity), not at the listed price. The ledger card refreshes as soon as a
  sale or purchase is recorded. The panel window is wide
  (1180 x 780) with its cards in three columns, falling to two and then one as it
  is narrowed; it is resizable and reopens at the size it was left at. The coverage
  list and capture totals refresh after market prices upload (at most once every
  two minutes), not only hourly. Most wanted sits under the activity feed and
  shows two groups: items with no price, and items with only an estimated price. The
  account stats are refetched when the realm becomes known or changes, so the list
  is not missing for the first hour after a start. A wanted item shows which
  enchant levels need the scan (5.0, 5.2, or 5.1-5.4 for a run). Activity lines
  use in-game item names from an embedded table (`client/itemnames.tsv.gz`), and
  name the quality instead of Q2 to Q5.

## Not yet wired

- The Go module path is still `github.com/ao-data/albiondata-client`; the binary is
  `codex-client`. A rename is cosmetic and deferred.
- Instant market trades (bought or sold without an order) produce no mail and are not
  captured yet.
- Combat, gathering and fame messages are visible to the client but not decoded.
