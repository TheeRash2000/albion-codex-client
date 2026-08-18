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

4. Double-click **`codex-client.exe`**. A small "Codex Client" window opens.
   - Windows may say it's an unrecognized app - click **More info → Run anyway**.
     (It's unsigned; that warning is expected for a small tool.)
5. Click **Link this PC** → **Copy code** → the link page opens in your browser →
   paste the code → **Confirm**. The window flips to "Linked".
6. With the app **still running**, log in to Albion (or, if you're already in,
   log out to character select and back in). Your board uploads - the window
   shows "… nodes, just now". Open your Destiny Board on the site to see it.

## Good to know

- It must be running **before** you log in - the game only sends the full board
  once, at login.
- It lives in your **system tray** (bottom-right, by the clock). Right-click the
  icon to reopen the window or quit.
- It does **not** modify the game. It only reads network packets, which Albion's
  developers have said is fine.

## Linux (alpha)

The Linux build is a command-line alpha: same capture and upload pipeline, no
tray window, no autostart, no auto-update (you download new builds yourself).
The control panel still works - it opens in your normal browser.

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
