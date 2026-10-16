package core

import "fmt"

// Step is one change an operation makes. A Plan's Steps are what --dry-run
// and Removal Plans show, and exactly what a real run executes.
type Step struct {
	Describe string `json:"describe"`
	do       func() error
	undo     func()
}

// Plan is the list of Steps an operation makes, in order.
type Plan []Step

// run executes the Plan. If a Step fails, the completed Steps are undone in
// reverse, so the operation leaves things as they were (FS-3).
func (p Plan) run() error {
	for i, s := range p {
		if err := s.do(); err != nil {
			for j := i - 1; j >= 0; j-- {
				if p[j].undo != nil {
					p[j].undo()
				}
			}
			return fmt.Errorf("%s: %w", s.Describe, err)
		}
	}
	return nil
}

// Descriptions lists what the Plan does.
func (p Plan) Descriptions() []string {
	d := make([]string, len(p))
	for i, s := range p {
		d[i] = s.Describe
	}
	return d
}
