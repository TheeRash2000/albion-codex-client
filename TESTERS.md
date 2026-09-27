# Testing the Codex Client

Thanks for helping test. This little app reads your Albion **destiny board** and
fills it in on [albioncodex.com](https://albioncodex.com) automatically, so you
never have to type your spec levels by hand. It only reads your board - nothing
else is uploaded.

## One-time setup (about 5 minutes)

1. **Install Npcap** (lets the app read game traffic):
   <https://npcap.com/#download> - download the installer, run it, keep the
   default options.
2. **Make a Codex account:** go to <https://albioncodex.com> and sign in with
   Discord or Google.
3. **Get the app:** put `codex-client.exe` in its own folder (e.g.
   `Documents\CodexClient`). It writes a small token + log file next to itself.

## Link your PC and run it

4. Double-click **`codex-client.exe`**. It starts in the background and puts a
   **Codex icon in your system tray** (bottom-right, by the clock - you may need
   to click the small arrow to see hidden icons). There is no window; everything
   is on the icon's menu.
   - Windows may say it's an unrecognized app - click **More info → Run anyway**.
     (It's unsigned; that warning is expected for a small tool.)
5. Click the tray icon and choose **Link this PC**. The code is copied to your
   clipboard and the link page opens in your browser - paste it there and
   **Confirm**. The menu's top line flips to "Status: linked to Codex".
   - If the code did not copy, the notification shows it so you can type it. It
     is also on the menu item itself while linking, and clicking that item again
     copies it.
6. With the app **still running**, log in to Albion (or, if you're already in,
   log out to character select and back in). Your board uploads - the menu shows
   "Destiny Board: … nodes, just now". Open your Destiny Board on the site to
   see it.

## Good to know

- It must be running **before** you log in - the game only sends the full board
  once, at login.
- It lives in your **system tray** (bottom-right, by the clock). The icon's menu
  is the whole app: status, whether it can see your game, your last board upload,
  what has been captured (**Capture health** / **Recent activity** / **Issues**),
  the market and AODP switches, **Link this PC**, and **Quit**.
- Launching it a second time will not open anything - one copy runs at a time,
  and it tells you to look at the tray icon.
- If it ever seems to be using too much memory, click **Save memory report** on
  the menu and send me the file it names.
- It does **not** modify the game. It only reads network packets, which Albion's
  developers have said is fine.

## Linux (alpha)

The Linux build is a command-line alpha: same capture and upload pipeline, no
tray, no autostart, no auto-update (you download new builds yourself).

From 0.1.16 there is **no control panel on Linux either** - it was a local web
page, and the Windows client replaced it with a native tray menu that Linux
cannot show without desktop libraries the build deliberately does not require.
Status comes from the terminal output. Pair with `--codex-pair`.

1. **No Npcap needed** - Linux uses libpcap, which your distro already has.
   (Only building from source needs `libpcap-dev`.)
2. Download `codex-client-linux` into its own folder and make it runnable:

   ```bash
   chmod +x codex-client-linux
   ```

3. **Allow packet capture** (one-time per downloaded build; capture needs a
   network capability that normal programs don't have):

   ```bash
   sudo setcap cap_net_raw,cap_net_admin=eip ./codex-client-linux
   ```

   Don't run it with `sudo` instead - that works, but then your pairing token
   lands in root's home and you'll be asked to re-link later.

4. Run `./codex-client-linux`, then pair and play exactly like on Windows
   (link page opens in the browser; board uploads on login/zone change).
   Your token and log live in `~/.config/CodexClient/`.

Works with Albion under Proton/Steam - the capture watches network traffic on
the game's port, not the game process.

Found a bug or something weird? Tell me - that's what this is for.
