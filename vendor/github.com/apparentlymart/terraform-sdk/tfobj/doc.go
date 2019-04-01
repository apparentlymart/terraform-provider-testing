// Package tfobj contains helper types for working with Terraform object values
// (resource configurations, etc) in a higher-level way while still retaining
// the Terraform schema concepts.
//
// This provides a middle-ground between working directly with the object value
// representation (cty.Value) and directly decoding into a custom struct using
// gocty.
package tfobj
