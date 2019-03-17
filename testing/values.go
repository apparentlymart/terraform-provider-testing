package testing

import (
	"github.com/zclconf/go-cty/cty"
)

// formatValue formats a value in a way that resembles Terraform language syntax
// and uses the type conversion functions where necessary to indicate exactly
// what type it is given, so that equality test failures can be quickly
// understood.
func formatValue(v cty.Value) string {
	// FIXME: Write a user-friendly implementation of this
	return v.GoString()
}
