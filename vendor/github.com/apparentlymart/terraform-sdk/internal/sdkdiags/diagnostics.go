// Package sdkdiags contains the diagnostics helpers for the SDK.
//
// They are separated into this separate package so that other packages in the
// SDK module itself can use them without creating import cycles. The public
// interface to all of these symbols in the main tfsdk package.
package sdkdiags

import (
	"fmt"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

// Diagnostics is a collection type used to report zero or more problems that
// occurred during an operation.
//
// A nil Diagnostics indicates no problems. However, a non-nil Diagnostics
// may contain only warnings, so use method HasErrors to recognize when an
// error has occurred.
type Diagnostics []Diagnostic

// Diagnostic represents a single problem that occurred during an operation.
//
// Use an appropriate severity to allow the caller to properly react to the
// problem. Error severity will tend to halt further processing of downstream
// operations.
//
// If the error concerns a particular attribute within the configuration, use
// the Path field to indicate that specific attribute. This allows the caller
// to produce more specific problem reports, possibly containing direct
// references to the problematic value. General problems, such as total
// inability to reach a remote API, should be reported with a nil Path.
type Diagnostic struct {
	Severity DiagSeverity
	Summary  string
	Detail   string
	Path     cty.Path
}

func (diags Diagnostics) Append(vals ...interface{}) Diagnostics {
	for _, rawVal := range vals {
		switch val := rawVal.(type) {
		case Diagnostics:
			diags = append(diags, val...)
		case Diagnostic:
			diags = append(diags, val)
		case error:
			// We'll generate a generic error diagnostic then, to more easily
			// adapt from existing APIs that deal only in errors.
			diags = append(diags, Diagnostic{
				Severity: Error,
				Summary:  "Error from provider",
				Detail:   fmt.Sprintf("Provider error: %s", FormatError(val)),
			})
		default:
			panic(fmt.Sprintf("Diagnostics.Append does not support %T", rawVal))
		}
	}
	return diags
}

func (diags Diagnostics) HasErrors() bool {
	for _, diag := range diags {
		if diag.Severity == Error {
			return true
		}
	}
	return false
}

// UnderPath rewrites the Path fields of the receiving diagnostics to be
// relative to the given path. This can be used to gradually build up
// a full path while working backwards from leaf values, avoiding the
// need to pass full paths throughout validation and other processing
// walks.
//
// This function modifies the reciever in-place, but also returns the receiver
// for convenient use in function return statements.
func (diags Diagnostics) UnderPath(base cty.Path) Diagnostics {
	for i, diag := range diags {
		path := make(cty.Path, 0, len(base)+len(diag.Path))
		path = append(path, base...)
		path = append(path, diag.Path...)
		diags[i].Path = path
	}
	return diags
}

type DiagSeverity int

const (
	diagSeverityInvalid DiagSeverity = iota

	// Error is a diagnostic severity used to indicate that an option could
	// not be completed as requested.
	Error

	// Warning is a diagnostic severity used to indicate a problem that
	// did not block the competion of the requested operation but that the
	// user should be aware of nonetheless.
	Warning
)

// FormatError returns a string representation of the given error. For most
// error types this is equivalent to calling .Error, but will augment a
// cty.PathError by adding the indicated attribute path as a prefix.
func FormatError(err error) string {
	switch tErr := err.(type) {
	case cty.PathError:
		if len(tErr.Path) == 0 {
			// No prefix to render, then
			return tErr.Error()
		}

		return fmt.Sprintf("%s: %s", FormatPath(tErr.Path), tErr.Error())
	default:
		return err.Error()
	}
}

// FormatPath returns a string representation of the given path using a syntax
// that resembles an expression in the Terraform language.
func FormatPath(path cty.Path) string {
	var buf strings.Builder
	for _, rawStep := range path {
		switch step := rawStep.(type) {
		case cty.GetAttrStep:
			fmt.Fprintf(&buf, ".%s", step.Name)
		case cty.IndexStep:
			switch step.Key.Type() {
			case cty.String:
				// fmt's %q isn't quite the same as Terraform quoted string syntax,
				// but it's close enough for error reporting.
				fmt.Fprintf(&buf, "[%q]", step.Key.AsString())
			case cty.Number:
				fmt.Fprintf(&buf, "[%s]", step.Key.AsBigFloat())
			default:
				// A path through a set can contain a key of any type in principle,
				// but it will never be anything we can render compactly in a
				// path expression string, so we'll just use a placeholder.
				buf.WriteString("[...]")
			}
		default:
			// Should never happen because there are no other step types
			buf.WriteString(".(invalid path step)")
		}
	}
	return buf.String()
}

// ValidationError is a helper for constructing a Diagnostic to report an
// unsuitable value inside an attribute's ValidateFn.
//
// Use this function when reporting "unsuitable value" errors to ensure a
// consistent user experience across providers. The error message for the given
// error must make sense when used after a colon in a full English sentence.
//
// If the given error is a cty.PathError then it is assumed to be relative to
// the value being validated and will be reported in that context. This will
// be the case automatically if the cty.Value passed to the ValidateFn is used
// with functions from the cty "convert" and "gocty" packages.
func ValidationError(err error) Diagnostic {
	var path cty.Path
	if pErr, ok := err.(cty.PathError); ok {
		path = pErr.Path
	}

	return Diagnostic{
		Severity: Error,
		Summary:  "Unsuitable argument value",
		Detail:   fmt.Sprintf("This value cannot be used: %s.", FormatError(err)),
		Path:     path,
	}
}

// UpstreamAPIError is a helper for constructing a Diagnostic to report an
// otherwise-unhandled error response from an upstream API/SDK.
//
// Although ideally providers will handle common error types and return
// helpful, actionable error diagnostics for them, in practice there are always
// errors that the provider cannot predict and, unfortunately, some SDKs do not
// return errors in a way that allows providers to handle them carefully.
//
// In situations like these, pass the raw error value directly from the upstream
// SDK to this function to produce a consistent generic error message that
// adds the additional context about this being a problem reported by the
// upstream API, rather than by the provider or Terraform itself directly.
//
// The language used in the diagnostics returned by this function is appropriate
// only for errors returned when making calls to a remote API over a network.
// Do not use this function for errors returned from local computation functions,
// such as parsers, serializers, private key generators, etc.
func UpstreamAPIError(err error) Diagnostic {
	return Diagnostic{
		Severity: Error,
		Summary:  "Remote operation failed",
		Detail:   fmt.Sprintf("The remote API returned an error that the provider was unable to handle:\n\n%s", err),
	}
}
