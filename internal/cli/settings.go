package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"appimage-manager/internal/config"
	"appimage-manager/internal/state"
)

func init() {
	commands["config"] = command{
		usage: "config | config set <key> <value>", maxArgs: 3,
		run: func(_ context.Context, inv *invocation, args []string) (any, error) {
			switch {
			case len(args) == 0:
				return inv.showConfig()
			case len(args) == 3 && args[0] == "set":
				if err := inv.setConfig(args[1], args[2]); err != nil {
					return nil, err
				}
				return inv.showConfig()
			}
			return nil, usageError("aim config | aim config set <key> <value>")
		},
		print: func(inv *invocation, result any) {
			for _, kv := range result.(configView).lines() {
				fmt.Fprintln(inv.env.Stdout, kv)
			}
		},
	}
	commands["dismiss"] = command{
		usage: "dismiss <app-id>", minArgs: 1, maxArgs: 1,
		run: func(ctx context.Context, inv *invocation, args []string) (any, error) {
			return nil, inv.env.Core.Dismiss(ctx, args[0])
		},
	}
}

// configView is what aim config shows: the settings, never the token.
type configView struct {
	DefaultPoll    state.PollRate `json:"default_poll"`
	GitHubTokenSet bool           `json:"github_token_set"`
	Installed      bool           `json:"notify.installed"`
	Failed         bool           `json:"notify.failed"`
	DigestMismatch bool           `json:"notify.digest_mismatch"`
	TrayAutostart  bool           `json:"tray_autostart"`
}

func (v configView) lines() []string {
	token := "not set"
	if v.GitHubTokenSet {
		token = "set"
	}
	return []string{
		"default_poll           " + string(v.DefaultPoll),
		"github_token           " + token,
		"notify.installed       " + strconv.FormatBool(v.Installed),
		"notify.failed          " + strconv.FormatBool(v.Failed),
		"notify.digest_mismatch " + strconv.FormatBool(v.DigestMismatch),
		"tray_autostart         " + strconv.FormatBool(v.TrayAutostart),
	}
}

func (inv *invocation) showConfig() (configView, error) {
	c, err := config.Load(inv.env.Settings.File, os.Getenv)
	if err != nil {
		return configView{}, err
	}
	_, autostartErr := os.Stat(inv.env.Settings.Autostart)
	return configView{
		DefaultPoll: c.DefaultPoll, GitHubTokenSet: c.GitHubToken != "",
		Installed: c.Notify.Installed, Failed: c.Notify.Failed, DigestMismatch: c.Notify.DigestMismatch,
		TrayAutostart: autostartErr == nil,
	}, nil
}

func (inv *invocation) setConfig(key, value string) error {
	if key == "tray_autostart" {
		on, err := strconv.ParseBool(value)
		if err != nil {
			return usageError("tray_autostart is true or false")
		}
		return inv.setAutostart(on)
	}
	c, err := config.LoadFile(inv.env.Settings.File)
	if err != nil {
		return err
	}
	flag := func(target *bool) error {
		v, err := strconv.ParseBool(value)
		if err != nil {
			return usageError(key + " is true or false")
		}
		*target = v
		return nil
	}
	switch key {
	case "default_poll":
		if !config.ValidPoll(state.PollRate(value)) {
			return usageError("default_poll is daily, weekly, monthly or never")
		}
		c.DefaultPoll = state.PollRate(value)
	case "github_token":
		c.GitHubToken = value // "" removes it
	case "notify.installed":
		err = flag(&c.Notify.Installed)
	case "notify.failed":
		err = flag(&c.Notify.Failed)
	case "notify.digest_mismatch":
		err = flag(&c.Notify.DigestMismatch)
	default:
		return usageError(fmt.Sprintf("unknown setting %q", key))
	}
	if err != nil {
		return err
	}
	return config.Save(inv.env.Settings.File, c)
}

// setAutostart writes or removes the tray's login entry.
func (inv *invocation) setAutostart(on bool) error {
	path := inv.env.Settings.Autostart
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	entry := "[Desktop Entry]\nType=Application\nName=AiM tray\nComment=Shows AppImage updates that need you\n" +
		"Exec=" + inv.env.Settings.GUI + " --tray\nX-GNOME-Autostart-enabled=true\nX-AiM-Managed=true\n"
	return os.WriteFile(path, []byte(entry), 0o644)
}

// usageError is a mistake in how aim was called; it exits 2.
type usageError string

func (e usageError) Error() string { return string(e) }
