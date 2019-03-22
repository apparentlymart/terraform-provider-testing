package tap

import (
	"fmt"
	"strings"
)

// BailOut is an error type used to report when a test program intentionally
// aborts its run due to some environmental problem.
type BailOut string

func (err BailOut) Error() string {
	return fmt.Sprintf("testing aborted: %s", string(err))
}

// NoTests is an error type used to report when a test program runs no tests
// at all, which is always an error.
type NoTests struct{}

func (err NoTests) Error() string {
	return "no tests"
}

// Inconsistent is an error type used to report when a test program does not
// produce test results consistent with its plan.
type Inconsistent struct {
	Missing []int
	Extra   []int
}

func (err Inconsistent) Error() string {
	var buf strings.Builder
	if len(err.Missing) != 0 {
		buf.WriteString("no result for ")
		buf.WriteString(ranges(err.Missing))
	}
	if len(err.Missing) != 0 && len(err.Extra) != 0 {
		buf.WriteString(" and ")
	}
	if len(err.Extra) != 0 {
		buf.WriteString("unexpected extra result for ")
		buf.WriteString(ranges(err.Extra))
	}
	return buf.String()
}

func ranges(nums []int) string {
	var buf strings.Builder
	start := 0
	end := 0
	seen := false
	for _, num := range nums {
		if start == 0 {
			start = num
			end = num
		} else if num == end+1 {
			end = num
			continue // keep looking for an end
		} else {
			if seen {
				buf.WriteString(", ")
			}
			buf.WriteString(rangeStr(start, end))
			start = num
			end = num
			seen = true
		}
	}
	if start != 0 {
		if seen {
			buf.WriteString(", ")
		}
		buf.WriteString(rangeStr(start, end))
	}
	return buf.String()
}

func rangeStr(start, end int) string {
	if start == end {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}
