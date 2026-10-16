// Package gui is aim-gui: a Fyne window and tray icon that does everything
// by running aim --json (ADR 0001, spec GUI-9).
package gui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"appimage-manager/internal/core"
)

// Runner runs aim with args, giving it stdin, and calls line for each line
// it prints.
type Runner interface {
	Run(ctx context.Context, args []string, stdin []byte, line func([]byte)) error
}

// Client talks to aim.
type Client struct {
	run Runner
}

// NewClient returns a Client that runs aim through run.
func NewClient(run Runner) *Client { return &Client{run: run} }

// Reply is aim's answer to one command.
type Reply struct {
	Status    string          `json:"status"` // done | questions | error
	Questions []core.Question `json:"questions"`
	Result    json.RawMessage `json:"result"`
	Error     string          `json:"error"`
}

// Do runs an aim command with answers, reporting download progress. A
// reply with Questions is not an error: answer them and call Do again.
func (c *Client) Do(ctx context.Context, args []string, answers core.Fixed, progress func(done, total int64)) (Reply, error) {
	args = append(append([]string{}, args...), "--json")
	var stdin []byte
	if len(answers) > 0 {
		args = append(args, "--answers", "-")
		var err error
		if stdin, err = json.Marshal(answers); err != nil {
			return Reply{}, err
		}
	}
	var reply Reply
	var last []byte
	runErr := c.run.Run(ctx, args, stdin, func(line []byte) {
		var event struct {
			Event       string
			Done, Total int64
		}
		if json.Unmarshal(line, &event) == nil && event.Event == "progress" {
			if progress != nil {
				progress(event.Done, event.Total)
			}
			return
		}
		last = append(last[:0], line...)
	})
	if len(last) == 0 {
		if runErr != nil {
			return Reply{}, fmt.Errorf("running aim: %w", runErr)
		}
		return Reply{}, errors.New("aim gave no reply")
	}
	if err := json.Unmarshal(last, &reply); err != nil {
		return Reply{}, fmt.Errorf("reading aim's reply: %w", err)
	}
	if reply.Status == "error" {
		return reply, errors.New(reply.Error)
	}
	return reply, nil
}

// result runs a command that asks no Questions and decodes its result.
func (c *Client) result(ctx context.Context, into any, args ...string) error {
	reply, err := c.Do(ctx, args, nil, nil)
	if err != nil {
		return err
	}
	if reply.Status != "done" {
		return fmt.Errorf("aim %s asked a question it shouldn't have", args[0])
	}
	if into == nil || len(reply.Result) == 0 {
		return nil
	}
	return json.Unmarshal(reply.Result, into)
}

// List returns every Managed App.
func (c *Client) List(ctx context.Context) ([]core.AppStatus, error) {
	var apps []core.AppStatus
	return apps, c.result(ctx, &apps, "list")
}

// Exec runs the aim binary at Path.
type Exec struct {
	Path string
}

// Run runs aim. Exit code 3 (Questions) is part of the protocol, so only
// failures to run aim at all are errors; the reply says the rest.
func (e Exec) Run(ctx context.Context, args []string, stdin []byte, line func([]byte)) error {
	cmd := exec.CommandContext(ctx, e.Path, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line(scanner.Bytes())
	}
	err = cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, bytes.TrimSpace(stderr.Bytes()))
		}
		return nil
	}
	return err
}

// Scan returns AppImages AiM doesn't manage yet.
func (c *Client) Scan(ctx context.Context) ([]core.Found, error) {
	var found []core.Found
	return found, c.result(ctx, &found, "scan")
}

// Inspect suggests a Release Source for one AppImage.
func (c *Client) Inspect(ctx context.Context, path string) (core.Found, error) {
	var found core.Found
	return found, c.result(ctx, &found, "inspect", path)
}

// Settings are aim config's values.
type Settings struct {
	DefaultPoll    string `json:"default_poll"`
	GitHubTokenSet bool   `json:"github_token_set"`
	Installed      bool   `json:"notify.installed"`
	Failed         bool   `json:"notify.failed"`
	DigestMismatch bool   `json:"notify.digest_mismatch"`
	TrayAutostart  bool   `json:"tray_autostart"`
}

// Config returns AiM's settings.
func (c *Client) Config(ctx context.Context) (Settings, error) {
	var s Settings
	return s, c.result(ctx, &s, "config")
}

// SetConfig changes one setting.
func (c *Client) SetConfig(ctx context.Context, key, value string) error {
	return c.result(ctx, nil, "config", "set", key, value)
}

// Simple runs an aim command that needs no answers, such as dismiss,
// ignore, skip, unskip, rollback, set-poll, edit or check.
func (c *Client) Simple(ctx context.Context, args ...string) error {
	return c.result(ctx, nil, args...)
}
