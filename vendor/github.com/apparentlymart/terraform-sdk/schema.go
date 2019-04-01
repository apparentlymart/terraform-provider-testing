package tfsdk

import (
	"fmt"

	"github.com/apparentlymart/terraform-sdk/internal/dynfunc"
	"github.com/apparentlymart/terraform-sdk/tfschema"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// ValidateBlockObject checks that the given object value is suitable for the
// recieving block type, returning diagnostics if not.
//
// The given value must already have a type conforming to the schema. This
// function validates instead the attribute values and block definitions within
// the object.
func ValidateBlockObject(schema *tfschema.BlockType, val cty.Value) Diagnostics {
	var diags Diagnostics
	if !val.Type().IsObjectType() {
		diags = diags.Append(Diagnostic{
			Severity: Error,
			Summary:  "Invalid block object",
			Detail:   "An object value is required to represent this block.",
		})
		return diags
	}

	// Capacity 3 here is so that we have room for a nested block type, an
	// index, and a nested attribute name without allocating more. Each loop
	// below will mutate this backing array but not the original empty slice.
	path := make(cty.Path, 0, 3)

	for name, attrS := range schema.Attributes {
		path := path.GetAttr(name)
		av := val.GetAttr(name)
		attrDiags := ValidateAttrValue(attrS, av)
		diags = diags.Append(attrDiags.UnderPath(path))
	}

	for name, blockS := range schema.NestedBlockTypes {
		path := path.GetAttr(name)
		av := val.GetAttr(name)

		switch blockS.Nesting {
		case tfschema.NestingSingle:
			if !av.IsNull() {
				blockDiags := ValidateBlockObject(&blockS.Content, av)
				diags = diags.Append(blockDiags.UnderPath(path))
			}
		case tfschema.NestingList, tfschema.NestingMap:
			for it := av.ElementIterator(); it.Next(); {
				ek, ev := it.Element()
				path := path.Index(ek)
				blockDiags := ValidateBlockObject(&blockS.Content, ev)
				diags = diags.Append(blockDiags.UnderPath(path))
			}
		case tfschema.NestingSet:
			// We handle sets separately because we can't describe a path
			// through a set element (it has no key to use) and so any errors
			// in a set block are indicated at the set itself. Nested blocks
			// backed by sets are fraught with oddities like these, so providers
			// should avoid using them except for historical compatibilty.
			for it := av.ElementIterator(); it.Next(); {
				_, ev := it.Element()
				blockDiags := ValidateBlockObject(&blockS.Content, ev)
				diags = diags.Append(blockDiags.UnderPath(path))
			}
		default:
			diags = diags.Append(Diagnostic{
				Severity: Error,
				Summary:  "Unsupported nested block mode",
				Detail:   fmt.Sprintf("Block type %q has an unsupported nested block mode %#v. This is a bug in the provider; please report it in the provider's own issue tracker.", name, blockS.Nesting),
				Path:     path,
			})
		}
	}

	return diags
}

// ValidateAttrValue checks that the given value is a suitable value for the
// given attribute schema, returning diagnostics if not.
//
// This method is usually used only indirectly via ValidateBlockObject.
func ValidateAttrValue(schema *tfschema.Attribute, val cty.Value) Diagnostics {
	var diags Diagnostics

	if schema.Required && val.IsNull() {
		// This is a poor error message due to our lack of context here. In
		// normal use a whole-schema validation driver should detect this
		// case before calling SchemaAttribute.Validate and return a message
		// with better context.
		diags = diags.Append(Diagnostic{
			Severity: Error,
			Summary:  "Missing required argument",
			Detail:   "This argument is required.",
		})
	}

	convVal, err := convert.Convert(val, schema.Type)
	if err != nil {
		diags = diags.Append(Diagnostic{
			Severity: Error,
			Summary:  "Invalid argument value",
			Detail:   fmt.Sprintf("Incorrect value type: %s.", FormatError(err)),
		})
	}

	if diags.HasErrors() {
		// If we've already got errors then we'll skip calling the provider's
		// custom validate function, since this avoids the need for that
		// function to be resilient to already-detected problems, and avoids
		// producing duplicate error messages.
		return diags
	}

	if convVal.IsNull() {
		// Null-ness is already handled by the a.Required flag, so if an
		// optional argument is null we'll save the validation function from
		// having to also deal with it.
		return diags
	}

	if !convVal.IsKnown() {
		// If the value isn't known yet then we'll defer any further validation
		// of it until it becomes known, since custom validation functions
		// are not expected to deal with unknown values.
		return diags
	}

	// The validation function gets the already-converted value, for convenience.
	validate, err := dynfunc.WrapSimpleFunction(schema.ValidateFn, convVal)
	if err != nil {
		diags = diags.Append(Diagnostic{
			Severity: Error,
			Summary:  "Invalid provider schema",
			Detail:   fmt.Sprintf("Invalid ValidateFn: %s.\nThis is a bug in the provider that should be reported in its own issue tracker.", err),
		})
		return diags
	}

	moreDiags := validate()
	diags = diags.Append(moreDiags)
	return diags
}
