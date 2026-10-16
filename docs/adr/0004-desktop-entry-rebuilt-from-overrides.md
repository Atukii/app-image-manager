# Desktop Entries are rebuilt on every Swap; user changes live as Overrides

AiM never edits a Managed App's Desktop Entry in place. On every Swap it rebuilds the entry from the new AppImage's Embedded Desktop Entry, then applies the user's Overrides from AiM's state (including values chosen during Consolidation). That way upstream changes, such as new MimeTypes or a renamed icon, flow through, while the user's choices survive every update.

## Consequences

- Hand-editing the generated Desktop Entry is overwritten on the next Swap. Changes must go through `aim edit` or the GUI manage section, which can also reset a single field to its embedded value.
- Unmanage loses nothing: the Overrides are already written into the file it leaves behind.
