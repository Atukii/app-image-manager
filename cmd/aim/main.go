// Command aim manages AppImages: it installs them from GitHub releases or
// download links, integrates them into the desktop and keeps them updated.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"appimage-manager/internal/cli"
	"appimage-manager/internal/config"
	"appimage-manager/internal/core"
	"appimage-manager/internal/notify"
	"appimage-manager/internal/source"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) == 2 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println("aim", version)
		return 0
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "aim:", err)
		return 1
	}
	paths := core.XDGPaths(home, os.Getenv)
	cfg, err := config.Load(filepath.Join(paths.Config, "config.toml"), os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aim:", err)
		return 1
	}
	exe, err := os.Executable()
	if err == nil {
		exe, _ = filepath.EvalSymlinks(exe)
	}

	client := &http.Client{}
	c := core.New(core.Deps{
		Paths: paths,
		HTTP:  client,
		Sources: func(ref source.Ref) (source.ReleaseSource, error) {
			if ref.Kind == source.Direct {
				return source.NewDirect(ref, client, nil), nil
			}
			return source.NewGitHub(ref, source.GitHubOptions{
				Client: client, Token: cfg.GitHubToken, CacheDir: filepath.Join(paths.Cache, "github"),
			}), nil
		},
		Systemctl:    systemctl{},
		Aim:          exe,
		DefaultPoll:  cfg.DefaultPoll,
		Notifier:     notify.Notifier{Wants: cfg.Notify.Wants, Fallback: os.Stderr},
		Binaries:     []string{exe, filepath.Join(filepath.Dir(exe), "aim-gui")},
		PackageOwner: pacmanOwner,
		RefreshMenu:  refreshMenu,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return cli.Run(ctx, os.Args[1:], cli.Env{
		Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Interactive: isTerminal(os.Stdin) && isTerminal(os.Stderr),
		Core:        c,
		Settings: cli.Settings{
			File:      filepath.Join(paths.Config, "config.toml"),
			Autostart: filepath.Join(paths.Autostart, "aim-gui.desktop"),
			GUI:       filepath.Join(filepath.Dir(exe), "aim-gui"),
		},
	})
}

// systemctl runs the real systemctl.
type systemctl struct{}

func (systemctl) Run(ctx context.Context, args ...string) error {
	out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// pacmanOwner names the package that installed path, or "" if none did
// (or pacman isn't available).
func pacmanOwner(path string) string {
	out, err := exec.Command("pacman", "-Qqo", path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// refreshMenu updates the desktop's application cache, when the tool for
// it is installed.
func refreshMenu(dir string) {
	if _, err := exec.LookPath("update-desktop-database"); err == nil {
		exec.Command("update-desktop-database", dir).Run()
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
