package tap

import (
	"sort"
)

// RunReport is a description of the overall result of a test program.
//
// If the reader that produced a report returned an error then the report for
// the run may be incomplete.
type RunReport struct {
	Plan  *Plan
	Tests []*Report
}

// Plan describes the plan line from a test run. A *Plan might be nil if the
// test program didn't produce a plan yet.
type Plan struct {
	// Min and Max give the lowest and highest test numbers in the range of test
	// numbers expected by this test program. After the test program completes,
	// it should've produced one result for every test number in the range from
	// Min to Max, both inclusive.
	//
	// In current versions of TAP, Min is generally expected to always be 1 and
	// so callers might choose to just assume that. The minimum is included just
	// for completeness.
	Min, Max int
}

func (p *Plan) Valid() bool {
	if p == nil {
		return false
	}
	return p.Min > 0 && p.Max >= p.Min
}

func (p *Plan) check(results map[int]*Report) *Inconsistent {
	var ret Inconsistent
	for _, report := range results {
		if report.Num < p.Min || report.Num > p.Max {
			ret.Extra = append(ret.Extra, report.Num)
		}
	}
	if p.Max >= p.Min {
		for num := p.Min; num <= p.Max; num++ {
			if _, exists := results[num]; !exists {
				ret.Missing = append(ret.Missing, num)
			}
		}
	}

	sort.Ints(ret.Missing)
	sort.Ints(ret.Extra)
	if len(ret.Missing) > 0 || len(ret.Extra) > 0 {
		return &ret
	}
	return nil
}

// Report describes the outcome for one test.
type Report struct {
	// Num is the test number this result belongs to.
	Num int

	// Result describes the passing status for the test.
	Result Result

	// Name is the name given for the test, if any.
	Name string

	// Todo is set if the test program marked this particular test as a Todo
	// test, meaning that failures are expected. If Todo is set then the Result
	// is expected to be Fail for a successful test, and a Pass is considered to
	// be a "bonus" that ought to be reported to the user in a prominent way to
	// let them know that the test is now passing.
	Todo bool

	// If Result is Skip then SkipReason might contain a reason string for the
	// skip, if provided by the test program.
	SkipReason string

	// If Todo is set then TodoReason might contain a reason string for the
	// TODO, if provided by the test program.
	TodoReason string

	// Diagnostics captures any consecutive diagnostic lines that appeared
	// immediately before this test report in the test program output. These
	// will have had their leading "# " markers removed already.
	Diagnostics []string
}

// Result describes the passing status for a test.
type Result int

const (
	resultInvalid Result = iota

	// Pass signals that the test succeeded.
	Pass

	// Fail signals that the test failed.
	Fail

	// Skip signals that the test was skipped for some reason.
	Skip
)
