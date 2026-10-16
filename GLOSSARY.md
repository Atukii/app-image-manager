# AiM (AppImage Manager)

AiM installs AppImages from a release source, integrates them into the desktop, and keeps them up to date on a per-app schedule, without locking the user in.

## Apps and files

**AppImage**:
A single self-contained executable file distributing a Linux application.
_Avoid_: binary, package

**Managed App**:
An AppImage that AiM is responsible for: installing, integrating, checking and replacing.
_Avoid_: app (when ambiguous), package, installation

**App ID**:
The short, permanent identifier of a Managed App, fixed at install time; changing it means removing and re-adding the app.
_Avoid_: name, slug

**App Folder**:
The per-app directory holding a Managed App's AppImage, Previous Version and icon.
_Avoid_: install dir

**Previous Version**:
The single AppImage kept from before the most recent Swap, available for Rollback.
_Avoid_: backup, old version

## Sources and releases

**Source URL**:
The link the user gives when adding an app: a repository, releases page, tag page or direct download link.
_Avoid_: link, releases link

**Release Source**:
Where AiM looks for new Releases of a Managed App; either a GitHub repository or a Direct URL.
_Avoid_: provider, origin, forge

**Direct URL**:
A Release Source that is a single stable download link, outside GitHub, with no version numbers; a change is detected, not compared.
_Avoid_: generic source, static link

**Release**:
One published version of an app at its Release Source. Prereleases are never Releases for AiM's purposes.
_Avoid_: tag, build

**Version**:
The identifier of a Release, compared to decide whether a Release is newer than the installed one.
_Avoid_: tag (the tag is where a Version comes from, not the Version itself)

**Asset**:
One downloadable file attached to a Release.
_Avoid_: attachment, artifact

**Asset Pattern**:
The wildcard filename rule that picks the right Asset from every future Release of a Managed App.
_Avoid_: filter, glob, matcher

**Digest**:
The checksum a Release Source publishes for an Asset, which a download must match before it can be installed.
_Avoid_: hash, signature

## Updating

**Poll Rate**:
How often a Managed App is Checked: daily, weekly, monthly or never.
_Avoid_: frequency, interval, schedule

**Check**:
Asking a Managed App's Release Source whether a newer Release exists, and Swapping it in if so.
_Avoid_: poll, update (when only checking is meant)

**Swap**:
Atomically replacing a Managed App's installed AppImage with a newer, verified one, even while the app runs; the running app keeps the old version until it is reopened.
_Avoid_: install, upgrade, replace

**Reopen**:
The state of a Managed App that was Swapped while running: the new Version is installed, but the open app is still the old one until the user reopens it.
_Avoid_: restart required, pending, staged

**Update**:
A Check run by the user rather than by a timer; the user-facing "get me the latest" action.
_Avoid_: upgrade, refresh

**Rollback**:
Swapping a Managed App back to its Previous Version.
_Avoid_: revert, downgrade

**Skipped Release**:
A Release the user rolled back from or chose to skip, which Checks ignore until a newer Release appears or the user unskips it.
_Avoid_: blocked version, pinned version

## Desktop integration

**Desktop Entry**:
The launcher file that makes an AppImage appear in the desktop's application menu.
_Avoid_: .desktop file, shortcut, launcher

**Embedded Desktop Entry**:
The Desktop Entry shipped inside an AppImage by its developer.
_Avoid_: default desktop file, bundled entry

**Override**:
A user-chosen Desktop Entry value or launch setting that AiM reapplies on top of the Embedded Desktop Entry after every Swap.
_Avoid_: customisation, edit, setting

**Consolidation**:
Combining a user's existing Desktop Entry with the Embedded Desktop Entry, value by value, during Adoption; the user's chosen values become Overrides.
_Avoid_: merge, reconcile

**Shadowing**:
When a user-level Desktop Entry hides a system-level one with the same filename.
_Avoid_: conflict, collision

## Taking on and letting go

**Scan**:
Searching the usual folders for AppImages that AiM does not yet manage.
_Avoid_: discover, import

**Adoption**:
Turning an existing, unmanaged AppImage into a Managed App.
_Avoid_: import, migration

**Ignored AppImage**:
An AppImage the user told AiM to leave alone; Scans do not offer it again.
_Avoid_: excluded, skipped (Skip only defers the decision to the next Scan)

**Remove**:
Deleting a Managed App entirely: its App Folder, Desktop Entry and Check timer.
_Avoid_: uninstall (reserved for removing AiM itself), delete

**Unmanage**:
Ending AiM's responsibility for a Managed App while leaving the app installed, integrated and working.
_Avoid_: release, forget, eject, detach

**Uninstall**:
Unmanaging every Managed App and then removing AiM itself.
_Avoid_: remove (that is per-app and deletes the app)

**Removal Plan**:
The full list of file moves, edits, deletions and timer changes AiM shows, and waits for confirmation on, before a Remove, Unmanage or Uninstall.
_Avoid_: preview, summary
