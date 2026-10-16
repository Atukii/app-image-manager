# Go, two binaries, GUI drives the CLI

AiM is written in Go and ships as two binaries: `aim`, a static CLI with no runtime dependencies, and `aim-gui`, a Fyne window and tray icon. The GUI performs every action by running `aim <command> --json`, not by importing the core library. Users can install only the CLI. There is one code path for every action, and no two processes write state concurrently except through the CLI's own lock.

## Considered Options

- **TypeScript on Bun**: cross-compiles easily from macOS, but a 50 MB+ binary and less fun.
- **One binary with an `aim gui` subcommand**: simpler to install, but forces a libGL/cgo link on CLI-only users.
- **GUI imports the core library**: faster, but two writers to the state file need locking, and the logic risks drifting apart.
- **Wails / GTK4 / Qt**: Wails needs webkit2gtk at runtime and has a weak tray. GTK4 looks foreign on KDE (the CachyOS default), and Qt brings heavy dependencies. Fyne has the fewest runtime dependencies and a built-in tray.

## Consequences

- `aim`'s `--json` output is a public contract that the GUI depends on; changing it is a breaking change.
- `aim-gui` needs cgo, so release builds run on Linux in CI rather than being cross-compiled from macOS.
