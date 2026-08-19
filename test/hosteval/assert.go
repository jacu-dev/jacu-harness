//go:build hosteval

package hosteval

import (
	"fmt"
	"slices"
	"strings"
)

// Expectation is one claim about the tool sequence a prompt produced. A case
// passes only when every expectation holds; there is no partial credit, and a
// case that produced no events at all fails every Contains it declared.
type Expectation interface {
	Check(tools []string) error
	String() string
}

type containsExp struct{ tool string }

// Contains asserts the tool was called at least once.
func Contains(tool string) Expectation { return containsExp{tool} }

func (e containsExp) Check(tools []string) error {
	if slices.Contains(tools, e.tool) {
		return nil
	}
	return fmt.Errorf("expected %s to be called, got %s", e.tool, render(tools))
}
func (e containsExp) String() string { return "contains " + e.tool }

type notContainsExp struct{ tool string }

// NotContains asserts the tool was never called. This is the expectation that
// catches over-routing: a skill that drags the change workflow into a
// read-only question.
func NotContains(tool string) Expectation { return notContainsExp{tool} }

func (e notContainsExp) Check(tools []string) error {
	if !slices.Contains(tools, e.tool) {
		return nil
	}
	return fmt.Errorf("expected %s never to be called, got %s", e.tool, render(tools))
}
func (e notContainsExp) String() string { return "not contains " + e.tool }

type beforeExp struct{ first, second string }

// Before asserts the first call to `first` precedes the first call to
// `second`. Both must be present: an ordering claim over an absent tool is a
// claim nobody made, and silently passing it is how a skipped step reads as
// correct.
func Before(first, second string) Expectation { return beforeExp{first, second} }

func (e beforeExp) Check(tools []string) error {
	i := slices.Index(tools, e.first)
	j := slices.Index(tools, e.second)
	switch {
	case i < 0:
		return fmt.Errorf("expected %s before %s, but %s was never called; got %s",
			e.first, e.second, e.first, render(tools))
	case j < 0:
		return fmt.Errorf("expected %s before %s, but %s was never called; got %s",
			e.first, e.second, e.second, render(tools))
	case i > j:
		return fmt.Errorf("expected %s before %s, got %s", e.first, e.second, render(tools))
	}
	return nil
}
func (e beforeExp) String() string { return e.first + " before " + e.second }

type emptyExp struct{}

// NoJacuTools asserts the host answered without touching jacu at all. It is the
// only expectation that can pass on an empty delta, and the reason it exists:
// "quanto é 2+2" reaching a governance tool is a routing bug, not a nicety.
func NoJacuTools() Expectation { return emptyExp{} }

func (emptyExp) Check(tools []string) error {
	if len(tools) == 0 {
		return nil
	}
	return fmt.Errorf("expected no jacu tool call, got %s", render(tools))
}
func (emptyExp) String() string { return "no jacu tool call" }

func render(tools []string) string {
	if len(tools) == 0 {
		return "[] (no events)"
	}
	return "[" + strings.Join(tools, " -> ") + "]"
}
