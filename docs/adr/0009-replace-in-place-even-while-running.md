# Replace AppImages in place, even while they run (supersedes ADR 0005)

When a Check finds a newer Release, AiM installs it immediately, even if the app is running, so that the next time the user opens the app it is the new version, and AiM only says "updated" once it actually is. Waiting for the app to close (ADR 0005) meant a staged update could sit for up to a whole poll interval, and "reopen to update" was false.

This is safe on Linux because a running AppImage's runtime keeps its file open and serves the app from that open file. Renaming a new file over the path leaves the running process on the old file's contents, while new launches get the new file. It depends on two invariants:

1. **Never write into the installed file.** The new AppImage is written and verified as a temporary file *inside the App Folder*, so the final step is a `rename(2)` on the same filesystem, never a copy over the live path.
2. **The path is never missing or partial.** The installed file gets a second name, `.previous`, through a hard link (no copy), and the new file is then renamed over the path in one atomic step. Rollback does the reverse in the same way.

## Consequences

- There are no Staged Updates. A running app whose file has been replaced is shown as **Reopen** (its process's `/proc/<pid>/exe` ends in ` (deleted)`) until it is restarted.
- Rollback is allowed while the app runs, and works the same way.
- If the user launches the app again while the old version still runs, both versions run side by side. A few apps don't tolerate that (for example, the new version migrating settings the old one still has open). This is the same risk as replacing the file by hand, and is accepted.
- The invariants are covered by a test: a reader that keeps opening the path during a Swap never finds it missing or partial, and a file opened before the Swap keeps its old contents. Real AppImages under FUSE are checked by hand on CachyOS.
