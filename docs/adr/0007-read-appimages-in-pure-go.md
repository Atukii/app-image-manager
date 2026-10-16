# Read AppImages in pure Go; never execute them to inspect them

To get an AppImage's Embedded Desktop Entry, icon, update information and version, AiM opens the squashfs filesystem appended to the AppImage's ELF runtime and reads it in Go (`github.com/diskfs/go-diskfs`, which also writes the squashfs images used as test fixtures). It does not run `<file> --appimage-extract`. Extracting that way executes a freshly downloaded, unverified program just to read metadata, and it can't run on macOS, where AiM is developed and tested.

## Consequences

- Legacy type-1 AppImages (ISO9660, pre-2017) can't be read. They are still installed, with a drafted Desktop Entry for the user to review (spec `ADD-8a`) and the default icon.
- The squashfs offset is found by parsing the ELF header (the end of the section header table), not by searching for magic bytes.
