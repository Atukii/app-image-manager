package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
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
}

// With answers the operation's Questions with a.
func With(a Answerer) Opt { return func(o *opts) { o.answerer = a } }

// Yes accepts every suggested answer --yes may accept.
var Yes = With(defaults{})

// DryRun makes the operation report its Plan without changing anything.
func DryRun() Opt { return func(o *opts) { o.dryRun = true } }

// Progress reports download progress.
func Progress(fn func(done, total int64)) Opt { return func(o *opts) { o.progress = fn } }

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
	return hex.EncodeToString(h.Sum(nil))[:12]
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
