package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"appimage-manager/internal/core"
)

// prompter answers Questions by asking in the terminal.
type prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func newPrompter(in io.Reader, out io.Writer) *prompter {
	return &prompter{in: bufio.NewReader(in), out: out}
}

// Answer shows q and reads an answer, asking again until it is one q
// accepts. It has no answer once input ends.
func (p *prompter) Answer(_ context.Context, q core.Question) (core.Answer, bool) {
	fmt.Fprintln(p.out)
	if q.Problem != "" {
		fmt.Fprintf(p.out, "! %s\n", q.Problem)
	}
	fmt.Fprintln(p.out, q.Prompt)
	for _, step := range q.Plan {
		fmt.Fprintf(p.out, "  • %s\n", strings.ReplaceAll(step, "\n", "\n    "))
	}
	for _, o := range q.Options {
		fmt.Fprintf(p.out, "  %s) %s\n", o.Key, o.Label)
	}
	for {
		if q.Default != nil {
			fmt.Fprintf(p.out, "[%s] > ", *q.Default)
		} else {
			fmt.Fprint(p.out, "> ")
		}
		line, err := p.in.ReadString('\n')
		answer := strings.TrimSpace(line)
		if err != nil && answer == "" {
			fmt.Fprintln(p.out)
			return core.Answer{}, false
		}
		if answer == "" && q.Default != nil {
			answer = *q.Default
		}
		if accepts(q, answer) {
			return core.Answer{Value: answer, Basis: q.Basis}, true
		}
		if len(q.Options) > 0 {
			fmt.Fprintf(p.out, "Please choose %s.\n", keys(q))
		}
	}
}

// accepts reports whether answer is one q takes.
func accepts(q core.Question, answer string) bool {
	if answer == "" {
		return false
	}
	if len(q.Options) == 0 || q.Free {
		return true
	}
	for _, o := range q.Options {
		if o.Key == answer {
			return true
		}
	}
	return false
}

func keys(q core.Question) string {
	ks := make([]string, len(q.Options))
	for i, o := range q.Options {
		ks[i] = o.Key
	}
	return strings.Join(ks, ", ")
}
