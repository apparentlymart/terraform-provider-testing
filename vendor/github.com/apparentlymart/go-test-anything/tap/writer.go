package tap

import (
	"bytes"
	"fmt"
	"io"
)

// Writer produces TAP-formatted output on an io.Writer.
//
// Writer is a pretty low-level interface to TAP and may not be ideal for
// use directly when implementing tests. Instead, this API is intended to be
// used as a backend for a higher-level test reporting interface.
type Writer struct {
	w io.Writer

	writtenLines bool
	writtenPlan  bool
	planPending  *Plan
	nextNumber   int
}

// NewWriter creates a new Writer that writes TAP reports to the given io.Writer.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		w: w,

		nextNumber: 1,
	}
}

// Close writes out a trailing plan if necessary. If the plan was already
// generated at the start of the run then this is a no-op.
func (w *Writer) Close() error {
	if w.planPending == nil {
		return nil
	}
	err := w.writePlan(w.planPending)
	return err
}

// Plan writes the given plan immediately if possible, or otherwise retains the
// plan to be written out when Close is called.
//
// Plan should be called exactly once for each run, ideally before any other
// calls.
func (w *Writer) Plan(plan *Plan) error {
	if !w.writtenLines {
		return w.writePlan(plan)
	}
	w.planPending = plan
	return nil
}

// Report writes the given test report.
func (w *Writer) Report(report *Report) error {
	if report.Result == Skip && report.Todo {
		return fmt.Errorf("test cannot both be SKIP and TODO at the same time")
	}
	if report.Result != Skip && report.SkipReason != "" {
		return fmt.Errorf("SkipReason must be empty if result is not Skip")
	}
	if report.Todo == false && report.TodoReason != "" {
		return fmt.Errorf("TodoReason must be empty if todo is not set")
	}
	if report.Result != Pass && report.Result != Fail && report.Result != Skip {
		return fmt.Errorf("invalid test result %#v", report.Result)
	}

	// We'll build up our line in a buffer here so that we can write it all
	// out to our underlying writer in a single call.
	var buf bytes.Buffer

	num := report.Num
	if num == 0 {
		num = w.nextNumber
	}

	for _, diag := range report.Diagnostics {
		fmt.Fprintf(&buf, "# %s\n", diag)
	}

	switch report.Result {
	case Pass, Skip:
		fmt.Fprintf(&buf, "ok %d", num)
	case Fail:
		fmt.Fprintf(&buf, "not ok %d", num)
	}

	if report.Name != "" {
		buf.WriteByte(' ')
		buf.WriteString(report.Name)
	}

	switch {
	case report.Result == Skip:
		buf.WriteString(" # SKIP")
		if report.SkipReason != "" {
			buf.WriteString(": ")
			buf.WriteString(report.SkipReason)
		}
	case report.Todo:
		buf.WriteString(" # TODO")
		if report.TodoReason != "" {
			buf.WriteString(": ")
			buf.WriteString(report.TodoReason)
		}
	}

	buf.WriteByte('\n')
	_, err := w.w.Write(buf.Bytes())
	if err == nil {
		w.writtenLines = true
		w.nextNumber = num + 1
	}
	return err
}

// BailOut produces a "Bail Out" report that indicates the test is failing in
// a severe way that implies it cannot continue further. If the given reason
// is not empty then it will be included in the bail out report.
//
// Usually a call to BailOut should be closely followed by a call to Close and
// then the test program should exit.
func (w *Writer) BailOut(reason string) error {
	var err error
	if reason != "" {
		_, err = fmt.Fprintf(w.w, "Bail out! %s\n", reason)
	} else {
		_, err = fmt.Fprintln(w.w, "Bail out!")
	}
	if err == nil {
		w.writtenLines = true
	}
	return err
}

// Diagnostic produces a diagnostic line in the output with the given content.
// As with most other strings passed to Writer, the diagnostic string must not
// contain any newlines, or the result will be broken output.
func (w *Writer) Diagnostic(msg string) error {
	_, err := fmt.Fprintf(w.w, "# %s\n", msg)
	if err == nil {
		w.writtenLines = true
	}
	return err
}

func (w *Writer) writePlan(plan *Plan) error {
	if w.writtenPlan {
		return fmt.Errorf("duplicate plan")
	}
	_, err := fmt.Fprintf(w.w, "%d..%d\n", plan.Min, plan.Max)
	if err == nil {
		w.writtenLines = true
		w.writtenPlan = true
		w.planPending = nil
	}
	return err
}
