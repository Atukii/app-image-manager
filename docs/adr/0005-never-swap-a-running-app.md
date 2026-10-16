---
status: superseded by ADR 0009
---

# Never Swap a running app; stage and wait instead

Linux would allow renaming a new AppImage over a running one, because the process keeps the old inode. AiM deliberately doesn't: if the Managed App is running, the verified download is kept as a Staged Update and swapped in by a later Check after the app has exited. AiM never kills a running app. This avoids a confusing half-updated state, such as an app that relaunches helper processes from the new file while the old one is still running.

## Consequences

- A Managed App has at most one Staged Update. A newer Release found while one is waiting replaces it.
- An app left open for weeks stays on the old version. The user is notified that an update is waiting.
