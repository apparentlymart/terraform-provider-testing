package tap

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var planPattern = regexp.MustCompile(`^(\d+)\.\.(\d+)$`)
var reportPattern = regexp.MustCompile(`^(?i)(ok|not ok|Bail out!)(?:\s+((\d*)\s*(.*?)(?:\s+# (todo|skip|)\S*\s*(.*))?))?$`)

// Read is a convenience wrapper around constructing a Reader, reading all of
// its results, and constructing a report. A caller that doesn't need streaming
// access to results should use this for simplicity.
func Read(r io.Reader) (*RunReport, error) {
	tr := NewReader(r)
	return tr.ReadAll()
}

// Reader consumes TAP output from an io.Reader and provides a pull-based API
// to access that output.
type Reader struct {
	r  io.Reader
	sc *bufio.Scanner

	plan    *Plan
	nextNum int
	results map[int]*Report
	bail    *BailOut
	err     error
}

// NewReader creates a new Reader that parses TAP output from the given
// io.Reader.
func NewReader(r io.Reader) *Reader {
	sc := bufio.NewScanner(r)
	return &Reader{
		r:  r,
		sc: sc,

		nextNum: 1,
		results: make(map[int]*Report),
	}
}

// Read will block until either a new test report is available or until there
// are no more reports to read (either due to successful end of file or via an
// error). The result is non-nil if a new test report was found, or nil if there
// are no more reports to read.
func (r *Reader) Read() *Report {
	if r.err != nil {
		return nil // stop if we've reported an error
	}
	for r.sc.Scan() {
		line := r.sc.Bytes()
		if match := reportPattern.FindSubmatch(line); match != nil {
			prefix := strings.ToLower(string(match[1]))
			switch prefix {
			case "ok", "not ok":
				num := r.nextNum
				if len(match[3]) > 0 {
					num64, _ := strconv.ParseInt(string(match[3]), 10, 0)
					num = int(num64)
				}
				r.nextNum = num + 1

				report := &Report{
					Num:  num,
					Name: string(match[4]),
				}

				report.Result = Fail
				if prefix == "ok" {
					report.Result = Pass
				}
				switch strings.ToLower(string(match[5])) {
				case "skip":
					report.Result = Skip
					report.SkipReason = string(match[6])
				case "todo":
					report.Todo = true
					report.TodoReason = string(match[6])
				}

				r.results[num] = report
				return report
			case "bail out!":
				err := BailOut(match[2])
				r.err = err
				return nil
			}
		} else if match := planPattern.FindSubmatch(line); match != nil {
			min64, _ := strconv.ParseInt(string(match[1]), 10, 0)
			max64, _ := strconv.ParseInt(string(match[2]), 10, 0)
			r.plan = &Plan{
				Min: int(min64),
				Max: int(max64),
			}
		}
	}
	if len(r.results) == 0 {
		r.err = NoTests{}
	}
	return nil
}

// ReadAll is a convenience wrapper around calling Read in a loop for callers
// that don't need streaming TAP output. It will consume all of the results,
// update any other status, and then return the error from the reader if there
// is one.
func (r *Reader) ReadAll() (*RunReport, error) {
	for {
		report := r.Read()
		if report == nil {
			break
		}
	}
	return r.Report(), r.Err()
}

// Report creates and returns a RunReport object describing the overall outcome
// of a test run. The returned object will be incomplete if this method is called
// before the test run has finished.
func (r *Reader) Report() *RunReport {
	var ret RunReport
	plan := r.plan
	ret.Plan = plan

	// If we got no explicit plan then we'll create a synthetic one just to
	// get this done.
	if plan == nil {
		plan = &Plan{
			Min: 1,
			Max: 0,
		}
		for num := range r.results {
			if num > plan.Max {
				plan.Max = num
			}
		}
	}

	if plan.Valid() {
		count := plan.Max - plan.Min + 1
		tests := make([]*Report, count)
		i := 0
		for num := plan.Min; num <= plan.Max; num++ {
			if report, exists := r.results[num]; exists {
				tests[i] = report
			}
			i++
		}
		ret.Tests = tests
	}
	return &ret
}

// Err returns an error that was encountered during reading, if any. Call this
// after Read stops returning true to learn if the reason was due to the end
// being reached (in which case Err returns nil) or some other problem.
func (r *Reader) Err() error {
	if r.err != nil {
		// Our own errors take precedence over the scanner's errors
		return r.err
	}
	if err := r.sc.Err(); err != nil {
		// If there was an error in scanning then better to report that than
		// missing reports, since this error is probably the cause of those.
		return err
	}
	if plan := r.plan; plan != nil {
		inconsistent := plan.check(r.results)
		if inconsistent != nil {
			return *inconsistent
		}
	}
	return nil
}
