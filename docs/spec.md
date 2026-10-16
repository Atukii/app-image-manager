# AiM spec

What AiM must do, in the language of [GLOSSARY.md](../GLOSSARY.md). The reasons behind the big decisions are in [docs/adr/](./adr/). Requirements are numbered so tests and commits can refer to them (e.g. `ADD-4`).

## Scope

- Linux x86_64, developed against Arch/CachyOS (KDE). Other XDG desktops should work but aren't targeted.
- Per-user only. AiM never runs `sudo` and never writes outside the user's home directory.
- Two binaries: `aim` (CLI) and `aim-gui` (window + tray). `aim-gui` requires `aim` and does every action through `aim … --json`.
- **Out of scope for v1:** GitLab/Codeberg sources, scraping release pages, GPG signatures, prereleases, hourly polling, system-wide installs, other CPU architectures.

## Files and locations

| What | Where |
|---|---|
| App Folder | `~/Applications/<App ID>/` |
| AppImage | `~/Applications/<App ID>/<App ID>.AppImage` |
| Previous Version | `~/Applications/<App ID>/<App ID>.AppImage.previous` |
| Icon | `~/Applications/<App ID>/<App ID>.png` or `.svg` |
| Desktop Entry | `~/.local/share/applications/<vendor filename>.desktop`, falling back to `<App ID>.desktop` |
| Check timers | `~/.config/systemd/user/aim-check@.timer`, `aim-check@.service`, and one drop-in per app under `aim-check@<App ID>.timer.d/` |
| State | `~/.local/share/aim/apps.json` |
| Staged Updates | `~/.local/share/aim/staging/<App ID>/` |
| Config | `~/.config/aim/config.toml` |
| Tray autostart | `~/.config/autostart/aim-gui.desktop` |

XDG environment variables (`XDG_DATA_HOME`, `XDG_CONFIG_HOME`) are respected wherever they apply.

- **FS-1** Every Desktop Entry AiM writes carries `X-AiM-Managed=true` and `X-AiM-AppId=<App ID>`. Those keys are the only AiM-specific content in files outside AiM's own directories.
- **FS-2** State is written atomically (write to a temp file, then rename) and guarded by a file lock. A second `aim` process waits for the lock, with a timeout, instead of corrupting state.
- **FS-3** A failed operation leaves the Managed App in its previous working state. A half-finished install or Swap never ends up as the installed AppImage.

## Commands

Global flags: `--json` (machine-readable output; the contract the GUI uses), `--yes` (accept the default answer to every confirmation), `--dry-run` (print what would happen and change nothing).

- **CLI-1** When there's no terminal, any command that would prompt fails with a clear error naming the flag that answers the prompt. It never guesses.

### `aim add <Source URL>`

- **ADD-1** Accepts any GitHub URL (repository, releases page, tag page, direct Asset download) and resolves it to `owner/repo`. Any other `http(s)` URL is treated as a Direct URL.
- **ADD-2** From a direct GitHub Asset link, the Version comes from the tag in the path. The Asset Pattern is the filename with the Version replaced by `*` (`Obsidian-1.8.4.AppImage` → `Obsidian-*.AppImage`). A filename with no Version in it (`nvim-linux-x86_64.appimage`) is used as-is.
- **ADD-3** Otherwise AiM installs the latest Release and picks the Asset as follows:
  1. Keep Assets whose filename ends in `.AppImage` (case-insensitive).
  2. Drop Assets naming another architecture (`aarch64`, `arm64`, `armhf`, `i386`, `i686`).
  3. If exactly one is left, use it. If several are left, ask the user to choose and store an Asset Pattern derived from the choice. If none are left, fail and list what the Release contains.
- **ADD-4** Proposes an App ID: the repository name, lowercased and slugified, with common suffixes like `-releases` and `-appimage` removed. The user can change it. The prompt must say plainly that the **App ID can't be changed later without Removing and re-adding the app**. An App ID already in use is rejected.
- **ADD-5** Downloads to staging. If the Release Source publishes a Digest for the Asset, the download must match it, or nothing is installed and the user is told. If there's no Digest, the install goes ahead without one.
- **ADD-6** Makes the AppImage executable and moves it into its App Folder.
- **ADD-7** Extracts the Embedded Desktop Entry and icon without running the app (`--appimage-extract` into a temp directory, then delete the directory). Icon priority: an icon supplied with `--icon <path>` → the embedded icon → AiM's bundled default icon.
- **ADD-8** Writes the Desktop Entry from the Embedded Desktop Entry, with `Exec`, `TryExec` and `Icon` pointing at the App Folder, plus any Overrides.
- **ADD-8a** If there's no Embedded Desktop Entry, AiM drafts one (`Name` from the App ID, `Type=Application`, `Terminal=false`, `Categories=Utility;`) but **doesn't write it until the user has reviewed it**. It shows the draft and lets the user change `Name`, `Comment`, `Categories` (choosing from the freedesktop main categories: AudioVideo, Development, Education, Game, Graphics, Network, Office, Science, Settings, System, Utility), `Keywords` and `Terminal`. The values the user enters become Overrides. With no terminal, the draft is written as-is only with `--yes`; otherwise the command fails (`CLI-1`). The same review applies to `adopt`.
- **ADD-9** Before writing, checks for Shadowing: a Desktop Entry with the same filename in any system `XDG_DATA_DIRS` `applications/` directory. If found, warns and offers: continue, or use `<App ID>.desktop` instead.
- **ADD-10** Sets the Poll Rate from `--poll`, falling back to the configured default (weekly), and creates the app's timer instance unless the rate is `never`.
- **ADD-11** The first `add` or `adopt` installs and enables the timer template if it's missing, and prints one line saying so (see `SETUP-1`).
- **ADD-12** Refreshes the desktop menu (`update-desktop-database ~/.local/share/applications`, when available).

### `aim list`

- **LIST-1** Shows every Managed App with its App ID, display Name, installed Version, Poll Rate, last Check time and result, next scheduled Check, and whether a Staged Update or Skipped Release exists.

### `aim check [<App ID>…]`

The timer runs `aim check <App ID>`. Run by hand without arguments, it Checks every Managed App.

- **CHK-1** Asks the Release Source for the latest Release and ignores prereleases.
- **CHK-2** GitHub: a Release is newer if its Version is greater under semantic-version comparison (a leading `v` is ignored). If either tag isn't a valid version, a Release is newer if its tag differs from the installed one and it was published later.
- **CHK-3** Direct URL: there's a newer Release if any of `ETag`, `Last-Modified` or `Content-Length` differs from what was recorded at install.
- **CHK-4** A Skipped Release is never treated as newer.
- **CHK-5** If a newer Release exists, it's downloaded and verified (as in `ADD-5`) into staging, replacing any existing Staged Update. Then a Swap is attempted (`SWAP-1`).
- **CHK-6** A failure is retried up to 3 times within the same run, with short waits (about 10s, 30s, 90s). If all attempts fail, the user is notified straight away. A Digest mismatch is not retried. The user is notified immediately, and that Release isn't downloaded again until a newer one appears.
- **CHK-7** Records the time and result of every Check in state.

### `aim update [<App ID>…]`

- **UPD-1** Runs a Check immediately, ignoring the Poll Rate, followed by a Swap if possible. If the app is running, it reports that the update is staged and will be applied after the app exits.

### Swap

- **SWAP-1** A Managed App counts as running if any process's executable (`/proc/<pid>/exe`) is its AppImage, or any process runs from the AppImage's FUSE mount (`/tmp/.mount_*` for this file).
- **SWAP-2** If the app is running, the Staged Update is kept, and later Checks retry the Swap. A Check whose Release is unchanged still tries a pending Swap. AiM never stops or kills an app.
- **SWAP-3** If the app isn't running:
  1. The installed AppImage becomes the Previous Version, replacing any older one.
  2. The Staged Update is moved into place and made executable.
  3. The Embedded Desktop Entry and icon are extracted again, and the Desktop Entry is rebuilt from them plus the Overrides (ADR 0004). A user-supplied icon stays.
  4. State is updated.
- **SWAP-4** Notifications (each can be turned off in config): update installed, update staged because the app is running, Check failed, Digest mismatch.

### `aim rollback <App ID>`

- **RB-1** Requires a Previous Version. Swaps it back (only when the app isn't running, as in `SWAP-2`) and marks the Release being rolled back from as the Skipped Release.
- **RB-2** After a Rollback there's no Previous Version until the next Swap.
- **RB-3** `aim unskip <App ID>` clears the Skipped Release. The next Check may then install it again.

### `aim edit <App ID>` and `aim set-poll <App ID> <rate>`

- **EDIT-1** Sets or clears Overrides for: display Name, Comment, Categories, Keywords, icon (path to a file, copied into the App Folder), extra launch arguments, and environment variables (written as `Exec=env K=V … <AppImage> <args>`).
- **EDIT-2** Shows which Desktop Entry fields are overridden, and can reset a single field to its embedded value.
- **EDIT-3** Rebuilds the Desktop Entry immediately after a change.
- **EDIT-4** `set-poll` accepts `daily | weekly | monthly | never`. It rewrites only that app's timer drop-in (or removes the timer instance, for `never`) and reloads systemd.

### `aim scan`

- **SCAN-1** Looks for `*.AppImage` (case-insensitive) in `~/Applications`, `~/Downloads`, `~/.local/bin` and `~/AppImages`, skipping Managed Apps and Ignored AppImages.
- **SCAN-2** For each file found, offers: **Manage** (goes through `aim adopt`), **Leave alone** (saved as an Ignored AppImage and never offered again), or **Skip** (asked again on the next Scan).
- **SCAN-3** For Manage, suggests a Source URL from the update information embedded in the AppImage (e.g. `gh-releases-zsync|owner|repo|…`) when there is any. The user confirms or types one.

### `aim adopt <path> <Source URL>`

- **ADOPT-1** Moves the file into a new App Folder (the App ID is chosen as in `ADD-4`).
- **ADOPT-1a** Works out the installed Version, most reliable source first:
  1. The file's SHA-256 matches the Digest of an Asset in one of the Release Source's recent Releases. That Release's Version is certain.
  2. The Version embedded in the AppImage (the Embedded Desktop Entry's `X-AppImage-Version`).
  3. A version-like string in the filename.
- **ADOPT-1b** If no exact digest match is found, AiM **never assumes a Version**. It tells the user the Version is uncertain, shows the best guess and where it came from, and offers:
  - **a)** use the suggested Version;
  - **b)** enter a Version;
  - **c)** replace the file with the latest Release now.
  
  With no guess at all, only b) and c) are offered. With no terminal, it requires `--version <v>` or `--install-latest` (`CLI-1`).
- **ADOPT-2** Finds the user's existing Desktop Entries whose `Exec` or `TryExec` points at the old path.
- **ADOPT-3** If one is found, offers Consolidation. For each field that differs from the Embedded Desktop Entry, it shows the actual values:
  ```
  Name:       a) "My Obsidian"   b) "Obsidian"
  Categories: a) "Office;"       b) "Office;Utility;"   c) both: "Office;Utility;"
  ```
  - Option `a` is always the user's existing value and `b` the value from the AppImage. List fields (`Categories`, `MimeType`, `Keywords`) also offer `c`, a merged list of both.
  - There is **no default**: the user always chooses, and `--yes` can't choose for them.
  - `Exec`, `TryExec` and `Icon` always point at the App Folder. Extra arguments or `env` settings found in the user's `Exec` are carried over as Overrides.
  - Values the user takes from their own entry become Overrides.
  - The final Desktop Entry is shown before it's written.
- **ADOPT-4** Offers to delete the user's old Desktop Entries (default answer yes; always asked).
- **ADOPT-5** Otherwise behaves as `add` from step `ADD-7` onwards.

### `aim remove <App ID>`

- **RM-1** Shows a Removal Plan and waits for confirmation. Then it stops, disables and deletes the app's timer instance and drop-in, deletes the Desktop Entry, the App Folder and any Staged Update, and removes the app from state.

### `aim unmanage <App ID>`

- **UNM-1** Shows a Removal Plan and waits for confirmation. The app keeps working afterwards.
- **UNM-2** Stops and disables `aim-check@<App ID>.timer`, deletes its drop-in, and reloads systemd. If this was the last Managed App, it also deletes the timer template units.
- **UNM-3** Removes the `X-AiM-*` keys from the Desktop Entry and keeps everything else. Overrides are already written into the file.
- **UNM-4** Asks whether to delete the Previous Version (default yes).
- **UNM-5** If there's a Staged Update, asks whether to install it now (only offered when the app isn't running) or discard it (default discard).
- **UNM-6** Leaves the App Folder where it is by default. Optionally moves to the standard layout instead: the AppImage to `~/Applications/<Name>.AppImage`, the icon to `~/.local/share/icons/hicolor/<size>/apps/`, and the Desktop Entry's paths rewritten to match.
- **UNM-7** Removes the app from state.

### `aim uninstall`

- **UNI-1** Shows one combined Removal Plan and Unmanages every Managed App, asking the `UNM-4` to `UNM-6` questions per app, or once for all apps if the user prefers.
- **UNI-2** Removes the timer template units, the tray autostart entry and `~/.local/share/aim/`.
- **UNI-3** Asks whether to keep `~/.config/aim/`, with **no preselected answer**, and does what the user chose. With no terminal it requires `--keep-config` or `--delete-config` (`CLI-1`), and `--yes` doesn't answer this question.
- **UNI-4** If `aim` and `aim-gui` live in `~/.local/bin`, deletes them. If they were installed by pacman (`pacman -Qo`), prints the `sudo pacman -R …` command for the user to run.

### `aim setup` and `aim doctor`

- **SETUP-1** `setup` installs or repairs the timer template and every app's timer instance, enables them, runs `systemctl --user daemon-reload`, and rewrites any missing Desktop Entry. It also removes AiM units left behind for apps it no longer manages. It's safe to run any number of times.
- **DOC-1** `doctor` reports, with a fix for each problem:
  - `fuse2` missing (`sudo pacman -S fuse2`)
  - timer instances missing or disabled
  - Desktop Entries pointing at missing files
  - AppImages that aren't executable
  - Shadowing
  - state entries with no App Folder
  - GitHub rate-limit exhaustion (suggests setting a token)

## Config (`~/.config/aim/config.toml`)

- **CFG-1** `default_poll` (default `weekly`), `github_token` (optional; the `GITHUB_TOKEN` environment variable takes precedence), and `[notify]` toggles for `installed`, `staged`, `failed`, `digest_mismatch` (all on by default).

## Timers

- **TMR-1** `aim-check@.timer` sets `Persistent=true` and `RandomizedDelaySec=15min`. Each app's drop-in sets `OnCalendar=daily | weekly | monthly`.
- **TMR-2** `aim-check@.service` is `Type=oneshot` and runs `aim check %i`.

## Notifications

- **NOTIF-1** Sent over D-Bus (`org.freedesktop.Notifications`) without depending on `notify-send`. If no notification service is available, AiM falls back to logging only, and the Check doesn't fail.

## GUI (`aim-gui`)

- **GUI-1** Main window: the Managed App list (as in `LIST-1`), plus Add (Source URL, Asset choice, App ID with the permanence warning, Poll Rate, optional icon) and Scan (the `SCAN-2` choices as a list).
- **GUI-2** Each app has a manage section: icon, Overrides with "reset to embedded", Poll Rate, Update now, Rollback, Unskip, Remove, Stop managing. Remove and Stop managing show the Removal Plan.
- **GUI-3** Consolidation uses the same a/b/c options as `ADOPT-3`, as radio buttons, with a preview of the final Desktop Entry.
- **GUI-4** Settings: config values, the autostart toggle, and "Uninstall AiM" (with the Removal Plan, the keep-config question with no preselected answer, and the binary removal instructions).
- **GUI-5** Tray icon: shows when updates are staged or Checks have failed, with menu items Check all now, Open AiM and Quit. It starts at login through the autostart entry, which the user can turn off.
- **GUI-6** The GUI never schedules Checks and never writes AiM state or files itself. Everything goes through `aim … --json`.

## Build and release

- **CI-1** `.github/workflows/build.yaml` runs `go vet` and `go test` on every push, and on a `v*` tag builds `aim` (`CGO_ENABLED=0`) and `aim-gui` (cgo, on ubuntu with the Fyne build dependencies) for linux-x86_64, attaching both to a GitHub Release with a SHA-256 checksums file.
- **CI-2** The repository stays local until the user publishes it. The Go module path is `appimage-manager`.
