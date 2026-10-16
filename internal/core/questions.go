package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"appimage-manager/internal/state"
)

// Question is something an operation needs the user to decide.
type Question struct {
	// ID is stable across runs, e.g. "app-id"; it is part of the --json
	// contract.
	ID      string   `json:"id"`
	Prompt  string   `json:"prompt"`
	Options []Option `json:"options,omitempty"` // empty for free text
	// Default is the suggested answer; nil means there is none and the user
	// must choose.
	Default *string `json:"default,omitempty"`
	// YesOK says whether --yes may answer with the Default.
	YesOK bool `json:"yesOk"`
	// Flag is the aim flag that answers this Question.
	Flag string `json:"flag,omitempty"`
	// Plan is the Removal Plan (or other Plan) a "confirm" Question asks
	// about.
	Plan []string `json:"plan,omitempty"`
	// Problem explains why a previous answer was rejected.
	Problem string `json:"problem,omitempty"`
	// Basis fingerprints the facts the Question was built from; an answer
	// given for another Basis is stale.
	Basis string `json:"basis"`
}

// Option is one choice in a Question.
type Option struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Answer answers a Question. An empty Basis (as from a CLI flag) applies
// whatever the Question's facts are.
type Answer struct {
	Value string `json:"value"`
	Basis string `json:"basis,omitempty"`
}

// Answerer answers Questions; false means it has no answer.
type Answerer interface {
	Answer(ctx context.Context, q Question) (Answer, bool)
}

// NeedAnswers is the error an operation returns, before changing anything,
// when its Answerer cannot answer a Question.
type NeedAnswers struct {
	Questions []Question
}

func (n *NeedAnswers) Error() string {
	q := n.Questions[0]
	if q.Flag != "" {
		return fmt.Sprintf("%s (answer with %s)", q.Prompt, q.Flag)
	}
	return q.Prompt
}

// defaults answers every Question --yes may answer with its Default.
type defaults struct{}

func (defaults) Answer(_ context.Context, q Question) (Answer, bool) {
	if !q.YesOK || q.Default == nil {
		return Answer{}, false
	}
	return Answer{Value: *q.Default, Basis: q.Basis}, true
}

// Fixed answers Questions from a map of Question ID to Answer, as sent by
// aim-gui or given as flags.
type Fixed map[string]Answer

func (f Fixed) Answer(_ context.Context, q Question) (Answer, bool) {
	a, ok := f[q.ID]
	if !ok || (a.Basis != "" && a.Basis != q.Basis) {
		return Answer{}, false
	}
	return a, true
}

// Layered tries each Answerer in turn.
type Layered []Answerer

func (l Layered) Answer(ctx context.Context, q Question) (Answer, bool) {
	for _, a := range l {
		if ans, ok := a.Answer(ctx, q); ok {
			return ans, true
		}
	}
	return Answer{}, false
}

// Opt configures one operation.
type Opt func(*opts)

type opts struct {
	answerer Answerer
	dryRun   bool
	progress func(done, total int64)
	icon     string
	poll     state.PollRate
}

// With answers the operation's Questions with a.
func With(a Answerer) Opt { return func(o *opts) { o.answerer = a } }

// Defaults answers every Question --yes may answer with its suggestion.
var Defaults Answerer = defaults{}

// Yes accepts every suggested answer --yes may accept.
var Yes = With(Defaults)

// DryRun makes the operation report its Plan without changing anything.
func DryRun() Opt { return func(o *opts) { o.dryRun = true } }

// Progress reports download progress.
func Progress(fn func(done, total int64)) Opt { return func(o *opts) { o.progress = fn } }

// Poll sets a new app's Poll Rate instead of the default (ADD-10).
func Poll(rate state.PollRate) Opt { return func(o *opts) { o.poll = rate } }

// Icon makes Add use the icon file at path (PNG or SVG) instead of the
// AppImage's own (ADD-7).
func Icon(path string) Opt { return func(o *opts) { o.icon = path } }

func collect(options []Opt) opts {
	o := opts{answerer: Layered{}}
	for _, opt := range options {
		opt(&o)
	}
	return o
}

// ask fills in q's Basis and asks the Answerer, failing with NeedAnswers
// when it has no answer.
func (o opts) ask(ctx context.Context, q Question) (string, error) {
	q.Basis = basis(q)
	a, ok := o.answerer.Answer(ctx, q)
	if !ok {
		return "", &NeedAnswers{Questions: []Question{q}}
	}
	return a.Value, nil
}

func basis(q Question) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00", q.ID, q.Prompt)
	for _, o := range q.Options {
		fmt.Fprintf(h, "%s=%s\x00", o.Key, o.Label)
	}
	if q.Default != nil {
		h.Write([]byte(*q.Default))
	}
	for _, step := range q.Plan {
		fmt.Fprintf(h, "%s\x00", step)
	}
	h.Write([]byte(q.Problem))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// ErrCancelled means the user declined a Plan.
var ErrCancelled = errors.New("cancelled")

// ErrStale means state changed after the Plan was confirmed; nothing was
// changed, and the operation should be run again.
var ErrStale = errors.New("something changed since the plan was shown; run it again")

// confirm asks the user to confirm plan. generation is the state Generation
// the plan was built from, so a confirmation of an older plan is stale.
func (o opts) confirm(ctx context.Context, prompt string, plan Plan, generation int) error {
	answer, err := o.ask(ctx, Question{
		ID: "confirm", Prompt: prompt, Plan: append(plan.Descriptions(), fmt.Sprintf("(state %d)", generation)),
		Options: []Option{{Key: "yes", Label: "go ahead"}, {Key: "no", Label: "cancel"}},
		Default: ptr("yes"), YesOK: true, Flag: "--yes",
	})
	if err != nil {
		return err
	}
	if answer != "yes" {
		return ErrCancelled
	}
	return nil
}

func ptr(s string) *string { return &s }

// slug lowercases s and keeps letters, digits and single dashes.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}
