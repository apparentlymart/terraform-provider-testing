package tfsdk

import (
	"github.com/apparentlymart/terraform-sdk/internal/sdkdiags"
	"github.com/zclconf/go-cty/cty"
)

// Diagnostics is a collection type used to report zero or more problems that
// occurred during an operation.
//
// A nil Diagnostics indicates no problems. However, a non-nil Diagnostics
// may contain only warnings, so use method HasErrors to recognize when an
// error has occurred.
type Diagnostics = sdkdiags.Diagnostics

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
type Diagnostic = sdkdiags.Diagnostic

type DiagSeverity = sdkdiags.DiagSeverity

const (
	// Error is a diagnostic severity used to indicate that an option could
	// not be completed as requested.
	Error = sdkdiags.Error

	// Warning is a diagnostic severity used to indicate a problem that
	// did not block the competion of the requested operation but that the
	// user should be aware of nonetheless.
	Warning = sdkdiags.Warning
)

// FormatError returns a string representation of the given error. For most
// error types this is equivalent to calling .Error, but will augment a
// cty.PathError by adding the indicated attribute path as a prefix.
func FormatError(err error) string {
	return sdkdiags.FormatError(err)
}

// FormatPath returns a string representation of the given path using a syntax
// that resembles an expression in the Terraform language.
func FormatPath(path cty.Path) string {
	return sdkdiags.FormatPath(path)
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
	return sdkdiags.ValidationError(err)
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
	return sdkdiags.UpstreamAPIError(err)
}
