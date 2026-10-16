package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"appimage-manager/internal/core"
)

// background runs work off the UI thread and hands its result back on it.
func (u *UI) background(work func(ctx context.Context) error, done func(err error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		err := work(ctx)
		fyne.Do(func() { done(err) })
	}()
}

// simple runs an aim command that asks nothing, then confirms or reports
// the error and reloads.
func (u *UI) simple(confirmation string, args ...string) {
	u.background(func(ctx context.Context) error { return u.client.Simple(ctx, args...) }, func(err error) {
		if err != nil {
			u.toast.show(err.Error())
		} else {
			u.toast.show(confirmation)
		}
		u.Reload()
	})
}

// check checks one app now and says what happened.
func (u *UI) check(id string) {
	u.toast.show("Checking " + id + "…")
	u.runCheck([]string{"update", id})
}

func (u *UI) checkAll() {
	u.toast.show("Checking every app…")
	u.runCheck([]string{"check"})
}

func (u *UI) runCheck(args []string) {
	var results []core.CheckResult
	u.background(func(ctx context.Context) error {
		reply, err := u.client.Do(ctx, args, nil, nil)
		if err == nil {
			err = json.Unmarshal(reply.Result, &results)
		}
		return err
	}, func(err error) {
		if err != nil {
			u.toast.show(err.Error())
			u.Reload()
			return
		}
		var lines []string
		for _, r := range results {
			switch {
			case r.Result == "installed" && r.Reopen:
				lines = append(lines, fmt.Sprintf("%s updated to %s; reopen it to use the new version.", r.App, r.Tag))
			case r.Result == "installed":
				lines = append(lines, fmt.Sprintf("%s updated to %s.", r.App, r.Tag))
			case r.Result == "failed":
				lines = append(lines, fmt.Sprintf("%s: the check failed.", r.App))
			}
		}
		if len(lines) == 0 {
			lines = []string{"No new versions."}
		}
		u.toast.show(strings.Join(lines, "\n"))
		u.Reload()
	})
}

func (u *UI) rollback(a core.AppStatus) {
	msg := fmt.Sprintf("%s is back on %s; %s is skipped.", a.Name, a.Previous, a.Version)
	if a.Reopen {
		msg += " Reopen it to use " + a.Previous + "."
	}
	u.simple(msg, "rollback", a.ID)
}

func (u *UI) skipRelease(a core.AppStatus) {
	tag := widget.NewEntry()
	tag.SetPlaceHolder("the latest release")
	content := container.NewVBox(
		muted("Update checks won't install the release you skip. A newer release installs as usual."),
		widget.NewForm(widget.NewFormItem("Release to skip", tag)),
		muted("Leave it empty to skip the latest release. To undo a skip, use Allow in the app's panel."),
	)
	dialog.ShowCustomConfirm("Skip a release of "+a.Name, "Skip it", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		args := []string{"skip", a.ID}
		if t := strings.TrimSpace(tag.Text); t != "" {
			args = append(args, t)
		}
		u.simple("Skipped.", args...)
	}, u.win)
}

func (u *UI) ignore(f core.Found) {
	u.simple(fmt.Sprintf("AiM won't ask about %s again. It stays where it is.", baseName(f.Path)), "ignore", f.Path)
}

func (u *UI) skipFound(f core.Found) {
	for i, g := range u.found {
		if g.Path == f.Path {
			u.found = append(u.found[:i], u.found[i+1:]...)
			break
		}
	}
	u.render()
	u.toast.show(fmt.Sprintf("Skipped %s. AiM will ask about it again next time it looks for AppImages.", baseName(f.Path)))
}

// runFlow runs an aim command that may ask Questions, showing each round of
// them in a dialog until it's done (ADR 0008). confirmLabel names the
// final confirmation's button.
func (u *UI) runFlow(title, confirmLabel string, args []string) {
	u.flow(title, confirmLabel, args, core.Fixed{}, func(out core.Outcome, _ core.Fixed) {
		u.toast.show(strings.Join(append([]string{title + ": done."}, out.Notes...), "\n"))
		if u.openID != "" {
			if _, ok := u.findApp(u.openID); !ok {
				u.closePanel()
			}
		}
		u.Reload()
	})
}

func (u *UI) flow(title, confirmLabel string, args []string, answers core.Fixed, done func(core.Outcome, core.Fixed)) {
	var reply Reply
	progress := widget.NewProgressBar()
	busy := dialog.NewCustomWithoutButtons(title, container.NewVBox(muted("Working…"), progress), u.win)
	busy.Show()
	u.background(func(ctx context.Context) (err error) {
		reply, err = u.client.Do(ctx, args, answers, func(d, t int64) {
			if t > 0 {
				fyne.Do(func() { progress.SetValue(float64(d) / float64(t)) })
			}
		})
		return err
	}, func(err error) {
		busy.Hide()
		switch {
		case err != nil:
			dialog.ShowError(err, u.win)
		case reply.Status == "questions":
			u.ask(title, confirmLabel, reply.Questions, func(given core.Fixed) {
				for id, a := range given {
					answers[id] = a
				}
				u.flow(title, confirmLabel, args, answers, done)
			})
		default:
			var out core.Outcome
			json.Unmarshal(reply.Result, &out)
			done(out, answers)
		}
	})
}

// ask shows one round of Questions and passes the answers on, unless the
// user cancels. Options a Question has no Default for start unselected.
func (u *UI) ask(title, confirmLabel string, qs []core.Question, answered func(core.Fixed)) {
	answers := core.Fixed{}
	var form []fyne.CanvasObject
	var ready []func() bool
	confirming := len(qs) == 1 && qs[0].ID == "confirm"
	consolidating := len(qs) > 1 && strings.HasPrefix(qs[0].ID, "consolidate.")
	next := primaryButton("Continue", nil)
	if confirming {
		next.SetText(confirmLabel)
		if confirmLabel == "Remove" || strings.HasPrefix(confirmLabel, "Uninstall") {
			next.Importance = widget.DangerImportance
		}
	}
	refresh := func() {
		for _, ok := range ready {
			if !ok() {
				next.Disable()
				return
			}
		}
		next.Enable()
	}
	var preview *widget.Label
	if consolidating {
		preview = widget.NewLabel("")
		preview.TextStyle = fyne.TextStyle{Monospace: true}
	}
	updatePreview := func() {
		if preview == nil {
			return
		}
		var lines []string
		for _, q := range qs {
			key := strings.TrimPrefix(q.ID, "consolidate.")
			value := "(choose)"
			for _, o := range q.Options {
				if answers[q.ID].Value == o.Key {
					value = strings.Trim(strings.TrimPrefix(o.Label, "both: "), `"`)
				}
			}
			lines = append(lines, key+"="+value)
		}
		preview.SetText("[Desktop Entry]\n" + strings.Join(lines, "\n"))
	}

	for _, q := range qs {
		q := q
		if f := q.Facts; f["file"] != "" {
			about := f["file"] + " (" + strings.Trim(f["version"]+", "+f["size"], ", ") + ")"
			if f["verified"] == "yes" {
				form = append(form, callout(about+": verified against the checksum its release publishes.", colours().good))
			} else {
				form = append(form, callout(about+": its release publishes no checksum, so it isn't verified.", colours().warn))
			}
		}
		if q.Problem != "" {
			form = append(form, callout(q.Problem, colours().bad))
		}
		if !consolidating {
			form = append(form, bold(q.Prompt))
		}
		if len(q.Plan) > 0 {
			form = append(form, planView(q.Plan))
		}
		if confirming {
			answers[q.ID] = core.Answer{Value: "yes", Basis: q.Basis}
			continue
		}
		switch {
		case len(q.Options) > 0:
			labels := make([]string, len(q.Options))
			for i, o := range q.Options {
				labels[i] = optionLabel(q, o)
			}
			typed := widget.NewEntry()
			if q.Free {
				labels = append(labels, "Enter it:")
				typed.Disable()
			}
			radio := widget.NewRadioGroup(labels, nil)
			radio.Horizontal = consolidating
			radio.OnChanged = func(label string) {
				delete(answers, q.ID)
				typed.Disable()
				for _, o := range q.Options {
					if optionLabel(q, o) == label {
						answers[q.ID] = core.Answer{Value: o.Key, Basis: q.Basis}
					}
				}
				if q.Free && label == "Enter it:" {
					typed.Enable()
					if t := strings.TrimSpace(typed.Text); t != "" {
						answers[q.ID] = core.Answer{Value: t, Basis: q.Basis}
					}
				}
				updatePreview()
				refresh()
			}
			typed.OnChanged = func(t string) {
				if radio.Selected == "Enter it:" {
					if t = strings.TrimSpace(t); t != "" {
						answers[q.ID] = core.Answer{Value: t, Basis: q.Basis}
					} else {
						delete(answers, q.ID)
					}
					refresh()
				}
			}
			if q.Default != nil {
				for _, o := range q.Options {
					if o.Key == *q.Default {
						radio.SetSelected(optionLabel(q, o))
					}
				}
			}
			if consolidating {
				form = append(form, container.NewBorder(nil, nil, container.NewGridWrap(fyne.NewSize(110, 36), bold(strings.TrimPrefix(q.ID, "consolidate."))), nil, radio))
			} else {
				form = append(form, radio)
			}
			if q.Free {
				form = append(form, typed)
			}
			ready = append(ready, func() bool { _, ok := answers[q.ID]; return ok })
		default:
			entry := widget.NewEntry()
			if q.Default != nil {
				entry.SetText(*q.Default)
				answers[q.ID] = core.Answer{Value: *q.Default, Basis: q.Basis}
			}
			entry.OnChanged = func(t string) {
				if t = strings.TrimSpace(t); t != "" || (q.Default != nil && *q.Default == "") {
					answers[q.ID] = core.Answer{Value: t, Basis: q.Basis}
				} else {
					delete(answers, q.ID)
				}
				refresh()
			}
			form = append(form, entry)
			ready = append(ready, func() bool { _, ok := answers[q.ID]; return ok })
		}
	}
	if consolidating {
		form = append([]fyne.CanvasObject{muted("Your menu entry and the AppImage's disagree on these values. Choose each one: a) yours, b) the AppImage's, c) both.")}, form...)
		form = append(form, heading("THE MENU ENTRY AIM WILL WRITE"), preview)
		updatePreview()
	}
	var d dialog.Dialog
	cancel := widget.NewButton("Cancel", func() { d.Hide() })
	next.OnTapped = func() {
		d.Hide()
		answered(answers)
	}
	refresh()
	body := container.NewVScroll(container.NewVBox(form...))
	body.SetMinSize(fyne.NewSize(600, min(float32(120+90*len(qs)), 460)))
	d = dialog.NewCustomWithoutButtons(title, container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), cancel, next), nil, nil, body), u.win)
	d.Show()
}

// optionLabel is how an option reads: Consolidation's carry their a) b)
// c) letters, which its explanation refers to.
func optionLabel(q core.Question, o core.Option) string {
	if strings.HasPrefix(q.ID, "consolidate.") {
		return o.Key + ") " + strings.TrimPrefix(o.Label, "both: ")
	}
	return o.Label
}

// planView lists a Plan's steps; a step carrying a file (a Desktop Entry
// preview) shows it as a block.
func planView(plan []string) fyne.CanvasObject {
	box := container.NewVBox()
	for _, step := range plan {
		if strings.HasPrefix(step, "(state ") {
			continue // the confirmation's fingerprint, not a step
		}
		first, rest, multiline := strings.Cut(step, "\n")
		box.Add(widget.NewLabel("• " + first))
		if multiline {
			text := widget.NewLabel(strings.TrimRight(rest, "\n"))
			text.TextStyle = fyne.TextStyle{Monospace: true}
			bg := canvas.NewRectangle(colours().sunken)
			bg.CornerRadius = 4
			box.Add(container.NewStack(bg, text))
		}
	}
	return box
}

// addApp asks where the app comes from: a link or a file (GUI-4).
func (u *UI) addApp() {
	link := widget.NewEntry()
	link.SetPlaceHolder("https://github.com/owner/repo, its releases page, or an AppImage download link")
	choice := widget.NewRadioGroup([]string{"From a link", "From a file on this computer"}, nil)
	choice.SetSelected("From a link")
	files := container.NewVBox()
	for _, f := range u.found {
		f := f
		files.Add(widget.NewButton(tildeHome(f.Path), func() { u.addFile(f.Path, f.SourceURL) }))
	}
	browse := widget.NewButton("Browse…", nil)
	files.Add(browse)
	files.Hide()
	choice.OnChanged = func(c string) {
		if c == "From a link" {
			link.Show()
			files.Hide()
		} else {
			link.Hide()
			files.Show()
		}
	}
	content := container.NewVBox(choice, link, files,
		muted("Tip: you can also drop an AppImage file onto AiM's window, or paste a link into it (Ctrl+V)."))
	var d dialog.Dialog
	browse.OnTapped = func() {
		d.Hide()
		open := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
			if err == nil && r != nil {
				r.Close()
				u.addFile(r.URI().Path(), "")
			}
		}, u.win)
		open.SetFilter(storage.NewExtensionFileFilter([]string{".AppImage", ".appimage"}))
		open.Show()
	}
	d = dialog.NewCustomConfirm("Add an app", "Continue", "Cancel", content, func(ok bool) {
		if ok && choice.Selected == "From a link" && strings.TrimSpace(link.Text) != "" {
			u.addLink(strings.TrimSpace(link.Text))
		}
	}, u.win)
	d.Resize(fyne.NewSize(640, 0))
	d.Show()
}

// addLink adds an app from a link: its Questions, then the plan from a dry
// run with the Poll Rate to choose, then the real add (aim add).
func (u *UI) addLink(link string) {
	title := "Add an app"
	u.flow(title, "", []string{"add", link, "--dry-run"}, core.Fixed{}, func(out core.Outcome, answers core.Fixed) {
		u.confirmAdd(title, link, out, answers)
	})
}

func (u *UI) confirmAdd(title, link string, planned core.Outcome, answers core.Fixed) {
	rate := widget.NewRadioGroup([]string{"daily", "weekly", "monthly", "never"}, nil)
	rate.Horizontal = true
	rate.SetSelected("weekly")
	if s, err := u.client.Config(context.Background()); err == nil && s.DefaultPoll != "" {
		rate.SetSelected(s.DefaultPoll)
	}
	content := container.NewVBox(bold("AiM will:"), planView(planned.Plan), heading("CHECK FOR UPDATES"), rate)
	d := dialog.NewCustomConfirm(title, "Add "+planned.App, "Cancel", container.NewVScroll(content), func(ok bool) {
		if !ok {
			return
		}
		u.addAnswers(title, link, rate.Selected, answers)
	}, u.win)
	d.Resize(fyne.NewSize(680, 560))
	d.Show()
}

// addAnswers repeats the add for real with the dry run's answers; a
// Question is asked again only if its facts changed since (ADR 0008).
func (u *UI) addAnswers(title, link, rate string, answers core.Fixed) {
	u.flow(title, "", []string{"add", link, "--poll", rate}, answers, func(out core.Outcome, _ core.Fixed) {
		msg := "Added " + out.App + ". "
		if rate == "never" {
			msg += "Update checks are off; turn them on in its panel."
		} else {
			msg += "AiM will check it for updates " + rate + "."
		}
		u.toast.show(strings.Join(append([]string{msg}, out.Notes...), "\n"))
		u.Reload()
	})
}

// addFile adds an AppImage already on this computer (aim adopt): its
// source first, suggested from the file itself when it can be.
func (u *UI) addFile(path, suggested string) {
	if suggested == "" {
		if f, err := u.client.Inspect(context.Background(), path); err == nil {
			suggested = f.SourceURL
		}
	}
	source := widget.NewEntry()
	source.SetText(suggested)
	source.SetPlaceHolder("https://github.com/owner/repo, or the download link it came from")
	hint := "Where does it come from? AiM checks there for new versions."
	if suggested != "" {
		hint += " Suggested from the update information inside the AppImage."
	}
	content := container.NewVBox(muted(tildeHome(path)), muted(hint), source)
	d := dialog.NewCustomConfirm("Add "+baseName(path), "Continue", "Cancel", content, func(ok bool) {
		if ok && strings.TrimSpace(source.Text) != "" {
			u.runFlow("Add "+baseName(path), "Add", []string{"adopt", path, strings.TrimSpace(source.Text)})
		}
	}, u.win)
	d.Resize(fyne.NewSize(640, 0))
	d.Show()
}

// findAppImages shows what a Scan found, with the same choices as
// Activity.
func (u *UI) findAppImages() {
	u.tabs.SelectIndex(0)
	u.Reload()
	if len(u.found) == 0 {
		u.toast.show("Looking for AppImages in ~/Applications, ~/Downloads, ~/.local/bin and ~/AppImages…")
	}
}

// settings shows AiM's settings (GUI-5).
func (u *UI) settings() {
	s, err := u.client.Config(context.Background())
	if err != nil {
		dialog.ShowError(err, u.win)
		return
	}
	token := widget.NewPasswordEntry()
	if s.GitHubTokenSet {
		token.SetPlaceHolder("A token is set; type a new one to replace it")
	} else {
		token.SetPlaceHolder("github_pat_…")
	}
	poll := widget.NewRadioGroup([]string{"daily", "weekly", "monthly", "never"}, nil)
	poll.Horizontal = true
	poll.SetSelected(s.DefaultPoll)
	installed := widget.NewCheck("An app was updated (including when it needs reopening)", nil)
	installed.SetChecked(s.Installed)
	failed := widget.NewCheck("An update check failed (after three tries)", nil)
	failed.SetChecked(s.Failed)
	mismatch := widget.NewCheck("A download didn't match its checksum (AiM never installs it)", nil)
	mismatch.SetChecked(s.DigestMismatch)
	tray := widget.NewCheck("Show the tray icon when I log in (updates run from AiM's timers either way)", nil)
	tray.SetChecked(s.TrayAutostart)
	content := container.NewVBox(
		heading("GITHUB TOKEN"), token,
		muted("Optional. Without one, GitHub allows 60 update lookups an hour; with one, 5,000. A token with no permissions is enough: create one at github.com/settings/personal-access-tokens. The GITHUB_TOKEN environment variable, if set, takes precedence."),
		heading("UPDATE CHECKS FOR NEW APPS"), poll, muted("Each app keeps its own setting; change it in its panel."),
		heading("NOTIFICATIONS"), installed, failed, mismatch,
		heading("TRAY ICON"), tray,
		heading("LEAVE AIM"), muted("Your apps stay installed and keep working. AiM removes its timers, markers and records."),
		container.NewHBox(dangerButton("Uninstall AiM…", func() { u.runFlow("Uninstall AiM", "Uninstall", []string{"uninstall"}) })),
	)
	d := dialog.NewCustomConfirm("Settings", "Save", "Cancel", container.NewVScroll(content), func(ok bool) {
		if !ok {
			return
		}
		changes := [][2]string{}
		if t := strings.TrimSpace(token.Text); t != "" {
			changes = append(changes, [2]string{"github_token", t})
		}
		if poll.Selected != s.DefaultPoll {
			changes = append(changes, [2]string{"default_poll", poll.Selected})
		}
		for _, c := range []struct {
			key       string
			was, want bool
		}{
			{"notify.installed", s.Installed, installed.Checked}, {"notify.failed", s.Failed, failed.Checked},
			{"notify.digest_mismatch", s.DigestMismatch, mismatch.Checked}, {"tray_autostart", s.TrayAutostart, tray.Checked},
		} {
			if c.was != c.want {
				changes = append(changes, [2]string{c.key, fmt.Sprint(c.want)})
			}
		}
		u.background(func(ctx context.Context) error {
			for _, c := range changes {
				if err := u.client.SetConfig(ctx, c[0], c[1]); err != nil {
					return err
				}
			}
			return nil
		}, func(err error) {
			if err != nil {
				dialog.ShowError(err, u.win)
				return
			}
			u.toast.show("Settings saved.")
		})
	}, u.win)
	d.Resize(fyne.NewSize(700, 620))
	d.Show()
}

func baseName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}
