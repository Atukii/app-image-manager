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
| New version being installed | `~/Applications/<App ID>/.<App ID>.AppImage.new` (same filesystem, so the Swap is one rename) |
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
- **ADD-5** Downloads to the download cache. If the Release Source publishes a Digest for the Asset, the download must match it, or nothing is installed and the user is told. If there's no Digest, the install goes ahead without one. Either way the user is told which: the App ID Question carries the download's facts (file, Version, size, verified or not), and the outcome repeats it.
- **ADD-6** Makes the AppImage executable and moves it into its App Folder.
- **ADD-7** Extracts the Embedded Desktop Entry and icon without running the app (`--appimage-extract` into a temp directory, then delete the directory). Icon priority: an icon supplied with `--icon <path>` → the embedded icon → AiM's bundled default icon.
- **ADD-8** Writes the Desktop Entry from the Embedded Desktop Entry, with `Exec`, `TryExec` and `Icon` pointing at the App Folder, plus any Overrides.
- **ADD-8a** If there's no Embedded Desktop Entry, AiM drafts one (`Name` from the App ID, `Type=Application`, `Terminal=false`, `Categories=Utility;`) but **doesn't write it until the user has reviewed it**. It shows the draft and lets the user change `Name`, `Comment`, `Categories` (choosing from the freedesktop main categories: AudioVideo, Development, Education, Game, Graphics, Network, Office, Science, Settings, System, Utility), `Keywords` and `Terminal`. The values the user enters become Overrides. With no terminal, the draft is written as-is only with `--yes`; otherwise the command fails (`CLI-1`). The same review applies to `adopt`.
- **ADD-9** Before writing, checks for Shadowing: a Desktop Entry with the same filename in any system `XDG_DATA_DIRS` `applications/` directory. If found, warns and offers: continue, or use `<App ID>.desktop` instead.
- **ADD-10** Sets the Poll Rate from `--poll`, falling back to the configured default (weekly), and creates the app's timer instance unless the rate is `never`.
- **ADD-11** The first `add` or `adopt` installs and enables the timer template if it's missing, and prints one line saying so (see `SETUP-1`).
- **ADD-12** Refreshes the desktop menu (`update-desktop-database ~/.local/share/applications`, when available).

### `aim list`

- **LIST-1** Shows every Managed App with its App ID, display Name, installed Version, Previous Version, Poll Rate, last Check time and result, Skipped Release, Overrides, Release Source, Asset Pattern and Desktop Entry filename, and whether it needs reopening (Reopen).

### `aim check [<App ID>…]`

The timer runs `aim check <App ID>`. Run by hand without arguments, it Checks every Managed App.

- **CHK-1** Asks the Release Source for the latest Release and ignores prereleases.
- **CHK-2** GitHub: a Release is newer if its Version is greater under semantic-version comparison (a leading `v` is ignored). If either tag isn't a valid version, a Release is newer if its tag differs from the installed one and it was published later.
- **CHK-3** Direct URL: there's a newer Release if any of `ETag`, `Last-Modified` or `Content-Length` differs from what was recorded at install.
- **CHK-4** A Skipped Release is never treated as newer.
- **CHK-5** If a newer Release exists, it's downloaded, verified (as in `ADD-5`) and Swapped in straight away, whether or not the app is running (ADR 0009).
- **CHK-6** A failure is retried up to 3 times within the same run, with short waits (about 10s, 30s, 90s). If all attempts fail, the user is notified straight away. A Digest mismatch is not retried. The user is notified immediately, and that Release isn't downloaded again until a newer one appears.
- **CHK-7** Records the time and result of every Check in state.

### `aim update [<App ID>…]`

- **UPD-1** Runs a Check immediately, ignoring the Poll Rate. If the app is running, it reports that the update is installed and the app needs reopening.

### Swap

- **SWAP-1** A Swap never writes into the installed AppImage. The new AppImage is placed in the App Folder as a temporary file, made executable, the installed file is hard-linked as the Previous Version (replacing any older one), and the new file is renamed over the installed path in one step. The path is never missing or partial, and a running app keeps its old file (ADR 0009).
- **SWAP-2** A Managed App needs **Reopen** when a process still runs its replaced file (`/proc/<pid>/exe` is the AppImage's path followed by ` (deleted)`). AiM never stops or kills an app.
- **SWAP-3** After the rename, the Embedded Desktop Entry and icon are read from the new AppImage and the Desktop Entry is rebuilt from them plus the Overrides (ADR 0004). A user-supplied icon stays. If that fails, the Previous Version is renamed back.
- **SWAP-4** Notifications (each can be turned off in config): updated (saying when the app needs reopening), Check failed, Digest mismatch.

### `aim rollback <App ID>`

- **RB-1** Requires a Previous Version. Swaps it back in the same atomic way as `SWAP-1` (also while the app runs) and marks the Release being rolled back from as the Skipped Release.
- **RB-2** After a Rollback there's no Previous Version until the next Swap.
- **RB-3** `aim unskip <App ID>` clears the Skipped Release. The next Check may then install it again.
- **RB-4** `aim skip <App ID> [<version>]` makes a Release, the latest one by default, the Skipped Release so Checks don't install it. Skipping the installed Version is refused; that is a Rollback.

### `aim edit <App ID>` and `aim set-poll <App ID> <rate>`

- **EDIT-1** Sets or clears Overrides for: display Name, Comment, Categories, Keywords, icon (path to a file, copied into the App Folder), extra launch arguments, and environment variables (written as `Exec=env K=V … <AppImage> <args>`).
- **EDIT-2** Shows which Desktop Entry fields are overridden, and can reset a single field to its embedded value.
- **EDIT-3** Rebuilds the Desktop Entry immediately after a change.
- **EDIT-4** `set-poll` accepts `daily | weekly | monthly | never`. It rewrites only that app's timer drop-in (or removes the timer instance, for `never`) and reloads systemd.

### `aim scan`

- **SCAN-1** Looks for `*.AppImage` (case-insensitive) in `~/Applications`, `~/Downloads`, `~/.local/bin` and `~/AppImages`, skipping Managed Apps and Ignored AppImages.
- **SCAN-2** For each file found, offers: **Manage** (goes through `aim adopt`), **Leave alone** (saved as an Ignored AppImage and never offered again), or **Skip** (asked again on the next Scan).
- **SCAN-4** `aim inspect <path>` reports the same suggestion for any one AppImage, for adding a file chosen outside the scanned folders.
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

- **RM-1** Shows a Removal Plan and waits for confirmation. Then it stops, disables and deletes the app's timer instance and drop-in, deletes the Desktop Entry and the App Folder, and removes the app from state.

### `aim unmanage <App ID>`

- **UNM-1** Shows a Removal Plan and waits for confirmation. The app keeps working afterwards.
- **UNM-2** Stops and disables `aim-check@<App ID>.timer`, deletes its drop-in, and reloads systemd. If this was the last Managed App, it also deletes the timer template units.
- **UNM-3** Removes the `X-AiM-*` keys from the Desktop Entry and keeps everything else. Overrides are already written into the file.
- **UNM-4** Asks whether to delete the Previous Version (default yes).
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

- **CFG-1** `default_poll` (default `weekly`), `github_token` (optional; the `GITHUB_TOKEN` environment variable takes precedence), and `[notify]` toggles for `installed`, `failed`, `digest_mismatch` (all on by default).

## Timers

- **TMR-1** `aim-check@.timer` sets `Persistent=true` and `RandomizedDelaySec=15min`. Each app's drop-in sets `OnCalendar=daily | weekly | monthly`.
- **TMR-2** `aim-check@.service` is `Type=oneshot` and runs `aim check %i`.

## Notifications

- **NOTIF-1** Sent over D-Bus (`org.freedesktop.Notifications`) without depending on `notify-send`. If no notification service is available, AiM falls back to logging only, and the Check doesn't fail.

## GUI (`aim-gui`)

The layout, wording and colours were settled with a clickable prototype (layout "D"), kept on the branch `prototype/aim-gui-layout` (`prototype/aim-gui.html`). The prototype is the reference for anything below that a sentence can't pin down.

- **GUI-1 Window.** A toolbar (**+ Add app**, **Find AppImages…**, **Check all**, **Settings**) over two tabs:
  - **Activity** (the default; its tab shows a count) lists only what needs a decision, each as a card with its choices: an app updated while open (**Roll back to <previous>**, **Got it**), an app whose Check failed (**Add token** when GitHub's limit was the cause, **Try again**), and AppImages found on this computer (**Manage**, **Leave alone**, **Skip**). Clicking an app card's body opens that app in **Apps**. With nothing to decide it says "Nothing needs your attention."
  - **Apps** shows every Managed App as a tile (icon, name, Version, status chip) and a final **Add an app** tile.
- **GUI-2 App panel.** Selecting an app splits the window: the list narrows to a column, and the app's panel opens on the right with a ✕ (or Esc) to close it. It fades and slides in (about 0.2 s, only when it opens; none with reduced motion), casts a shadow over the list, and the list dims slightly. The panel fits on one screen without scrolling, in two columns:
  - left: Check for updates, Roll back to <previous>, Skip a release… or Allow <skipped>; update checks (daily / weekly / monthly / never, and the last Check); details (source, file pattern, files, menu entry);
  - right: application-menu fields (each marked as yours or from the AppImage, with a reset), icon, launch arguments and environment, and Stop managing… / Remove app… (both through their Removal Plan).
  A note at the top explains the app's state (Reopen, Check failed, Skipped).
- **GUI-3 Status chips** are exactly: **Reopen**, **Check failed**, **Skipped**, **Up to date**, **Checks off**.
- **GUI-4 Adding.** **+ Add app** asks first: **From a link** (then the `aim add` Questions) or **From a file on this computer** (choose or Browse…, then the source, the Version Question when uncertain, the App ID and the plan, as `aim adopt`). Dropping an AppImage file onto the window starts the file route; pasting a link (Ctrl+V) starts the link route. Links can't be dropped: Fyne only receives dropped files. Consolidation shows every differing field at once (a / b / c, nothing preselected) with a live preview of the final entry. Each confirmation states the Poll Rate chosen.
- **GUI-5 Settings:** GitHub token (with what it changes and how to get one), the default Poll Rate for new apps, notifications (updated, Check failed, Digest mismatch), the tray at login, and **Uninstall AiM…** (its Removal Plan and the keep-config choice with nothing preselected).
- **GUI-6 Tray icon** with a badge counting apps that need reopening or failed a Check; its menu lists those, then Check all now, Open AiM and Quit the tray icon (updates still run from the timers).
- **GUI-7 Pop-ups** confirm what happened in one line and disappear after about 7 seconds; they have no Undo. Skipping a found AppImage says AiM will ask about it again next time it looks.
- **GUI-8 Look.** The user's violet scheme in both themes, following the system setting: Neutral `#0F081D`, Primary `#A78BFA`, Secondary `#8B5CF6`, Tertiary `#C4B5FD`; dark is the reference, light is derived from the same hues (a deeper Secondary where the accent is text). JetBrains Mono everywhere. Square buttons, inputs and chips on rounded panels and cards; salmon for destructive actions.
- **GUI-9** The GUI never schedules Checks and never writes AiM state or files itself. Everything goes through `aim … --json`, and every sentence it shows describes behaviour that exists.

## Build and release

- **CI-1** `.github/workflows/build.yaml` runs `gofmt`, `go vet` and `go test -race` on every push and pull request. On a push to `main`, it works out the next semantic version from the Conventional Commits since the last `v*` tag and, if there is one, tags the commit, builds `aim` (`CGO_ENABLED=0`) and `aim-gui` (cgo, with Fyne's build dependencies) for linux-x86_64 with the version embedded, and publishes a GitHub Release with both and a `SHA256SUMS` file (ADR 0010). A `v*` tag pushed by hand is built and released the same way.
- **CI-2** The repository stays local until the user publishes it. The Go module path is `appimage-manager`.
