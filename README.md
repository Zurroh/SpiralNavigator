# Spiral Navigator

A small launcher for private Wizard101 servers. One exe, no installer.

- Keep a list of servers (name, address, port, school colour).
- Each server has a **patch notes file**, a plain Markdown file on the web, shown on the page when you pick that server.
- Optionally pulls game files from a **patch files folder** before you play.
- Optional **Report a bug** button per server: it opens a web page you choose (for example your GitHub Issues "new issue" page).
- Share a server with friends as a short code (`w101srv:...`); they paste it in.

## Using it

Run `SpiralNavigator.exe`. Pick a server on the left, read the notes, press **Play**.
The first time, it looks for your game folder (the one containing `Bin\WizardGraphicalClient.exe`) and asks if it can't find it (Settings).
Play starts the client with `-L <address> <port>`.

It refuses to start the game against KingsIsle's live servers.

Settings are in `%APPDATA%\SpiralNavigator\launcher.json`.

## Pasting into the game's chat

Wizard101's chat box takes no Ctrl+V. While Spiral Navigator is open (Windows), press **Ctrl+Shift+V** with the game as the active window:

- one line on the clipboard is typed into the chat box you already have open (press Enter to send);
- several lines are each sent as their own message (close the chat first; it opens and sends each one itself).

It acts only when the active window is Wizard101, so Ctrl+Shift+V works as usual everywhere else. Untick "Close the launcher after the game starts" in Settings, or the helper closes with the launcher.

## Patch notes file

Host any text or Markdown file. Example: `https://example.com/patchnotes.md`.

Supported: `#` headings, bold, italic, `code`, lists, block quotes, `---`, links, images (http/https only). Raw HTML is shown as text, never run.

```
# New patch!
Fixed bugs.
Updated to new Imlight version.
```

The file is fetched every time you pick the server (12 s timeout, 1 MB limit). If the site is down, the last copy is shown with an "offline" note.
Serve it as plain text. A web page that returns HTML is rejected.

## Patch files (optional)

Put game files in a folder laid out like the game folder:

```
patch/
  Data/GameData/MyServer.wad
  Data/GameData/Another.wad
```

Then run:

```
SpiralNavigator.exe make-manifest patch 5
```

This writes `patch/manifest.json` (version, size and sha256 of each file). Upload the whole folder to a web path, e.g. `https://example.com/patch/`, and put that address in the server's **Patch files folder** box.

The launcher compares hashes, shows what would change, and downloads only the differences (`.part` then rename, sha256-checked). With "Update game files when I press Play" ticked it does this automatically.

Safety limits: only files under `Data/` or `Bin/`, only known game data types such as `.wad`, `.xml`, `.dds`, `.mp3` (never `.exe` or `.dll`), no `..` or absolute paths.

## Share codes

**Share** on a server gives a `w101srv:` code. **Paste a shared server** on the left imports it. Codes carry name, address, port, notes and patch URLs, school colour; never your game folder.

## Seeding profiles for friends

Drop a `profiles.json` next to the exe (see `profiles.example.json`). It's used only the first time Spiral Navigator runs on a PC, so you can hand friends a zip with your server already in the list. `defaults.json` is the built-in list; it ships empty.

## Building

```
go build -ldflags "-H=windowsgui -s -w" -o SpiralNavigator.exe .
```

Needs Go 1.24+. The window uses WebView2, which is included in Windows 10/11. If it can't open, the launcher uses your default browser instead (and closes itself about 20 seconds after you close the tab).
`SpiralNavigator.exe --serve` prints a local URL for testing in a browser.

## Not affiliated

Spiral Navigator is an unofficial fan project. It is not made by, endorsed by or connected to KingsIsle Entertainment. Wizard101 is a trademark of its owner. It does not contain game files and refuses to connect to the official servers.

## Licence

MIT, see `LICENSE`. The bundled fonts have their own licences in `ui/fonts`.
