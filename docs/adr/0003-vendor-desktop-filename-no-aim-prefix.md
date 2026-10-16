# Desktop Entries use the vendor's filename, with no AiM prefix

A Managed App's Desktop Entry is written as `~/.local/share/applications/<vendor filename>.desktop`, taken from the Embedded Desktop Entry (falling back to `<App ID>.desktop`). AiM ownership is marked inside the file with `X-AiM-*` keys rather than an `aim-` filename prefix. There are two reasons. Unmanaging then only means stripping those keys, so nothing has to be renamed or migrated. And Wayland compositors match windows to launcher icons by the Desktop Entry filename (app_id), so the vendor's name gives correct taskbar icons and grouping.

## Consequences

- A user-level Desktop Entry hides a system one with the same filename (Shadowing). AiM must detect this and warn before writing.
- Finding AiM's files means reading the `X-AiM-*` keys or the state file, not globbing for a prefix.
