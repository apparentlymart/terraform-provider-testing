package tfschema

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

type BlockType struct {
	Attributes       map[string]*Attribute
	NestedBlockTypes map[string]*NestedBlockType
}

type Attribute struct {
	// Type defines the Terraform Language type that is required for values of
	// this attribute. Set Type to cty.DynamicPseudoType to indicate that any
	// type is allowed. The ValidateFunc field can be used to provide more
	// specific constraints on acceptable values.
	Type cty.Type

	// Required, Optional, and Computed together define how this attribute
	// behaves in configuration and during change actions.
	//
	// Required and Optional are mutually exclusive. If Required is set then
	// a value for the attribute must always be provided as an argument in
	// the configuration. If Optional is set then the configuration may omit
	// definition of the attribute, causing it to be set to a null value.
	// Optional can also be used in conjunction with computed, as described
	// below.
	//
	// Set Computed to indicate that the provider itself decides the value for
	// the attribute. When Computed is used in isolation, the attribute may not
	// be used as an argument in configuration at all. When Computed is combined
	// with Optional, the attribute may optionally be defined in configuration
	// but the provider supplies a default value when it is not set.
	//
	// Required may not be used in combination with either Optional or Computed.
	Required, Optional, Computed bool

	// Sensitive is a request to protect values of this attribute from casual
	// display in the default Terraform UI. It may also be used in future for
	// more complex propagation of derived sensitive values. Set this flag
	// for any attribute that may contain passwords, private keys, etc.
	Sensitive bool

	// Description is an English language description of the meaning of values
	// of this attribute, written as at least one full sentence with a leading
	// capital letter and trailing period. Use multiple full sentences if any
	// clarifying remarks are needed, but try to keep descriptions consise.
	Description string

	// ValidateFn, if non-nil, must be set to a function that takes a single
	// argument and returns Diagnostics. The function will be called during
	// validation and passed a representation of the attribute value converted
	// to the type of the function argument using package gocty.
	//
	// If a given value cannot be converted to the first argument type, the
	// function will not be called and instead a generic type-related error
	// will be returned automatically to the user. If the given function has
	// the wrong number of arguments or an incorrect return value, validation
	// will fail with an error indicating a bug in the provider.
	//
	// Diagnostics returned from the function must have Path values relative
	// to the given value, which will be appended to the base path by the
	// caller during a full validation walk. For primitive values (which have
	// no elements or attributes), set Path to nil.
	ValidateFn interface{}

	// Default, if non-nil, must be set to a value that can be converted to
	// the attribute's value type to be used as a default value for the
	// (presumably optional) attribute.
	//
	// For attributes whose "default" values cannot be assigned statically,
	// leave Default as nil and mark the attribute instead as Computed, allowing
	// the value to be assigned either during planning or during apply.
	Default interface{}
}

type NestedBlockType struct {
	Nesting NestingMode
	Content BlockType

	MaxItems, MinItems int
}

type NestingMode int

const (
	nestingInvalid NestingMode = iota
	NestingSingle
	NestingList
	NestingMap
	NestingSet
)

//go:generate stringer -type=NestingMode

// DefaultValue returns the cty.Value representation of the receiving attribute's
// default, as specified in the Default field.
//
// Will panic if the configured default cannot be converted to the attribute's
// value type.
func (a *Attribute) DefaultValue() cty.Value {
	if a.Default == nil {
		return cty.NullVal(a.Type)
	}

	v, err := gocty.ToCtyValue(a.Default, a.Type)
	if err != nil {
		panic(fmt.Sprintf("invalid default value %#v for %#v: %s", a.Default, a.Type, err))
	}
	return v
}

// Null returns a null value of the type implied by the receiving schema.
func (b *BlockType) Null() cty.Value {
	return cty.NullVal(b.ImpliedCtyType())
}

// Unknown returns an unknown value of the type implied by the receiving schema.
func (b *BlockType) Unknown() cty.Value {
	return cty.UnknownVal(b.ImpliedCtyType())
}

// ImpliedCtyType derives a cty.Type value to represent values conforming to
// the receiving schema. The returned type is always an object type, with its
// attributes derived from the attributes and nested block types defined in
// the schema.
//
// This corresponds with similar logic in Terraform itself, and so must be
// compatible enough with that logic to communicate with Terraform's own
// object serializer/deserializer.
//
// This function produces reasonable results only for a valid schema. Use
// InternalValidate on the schema in provider tests to check that it is correct.
// When called on an invalid schema, the result may be incorrect or incomplete.
func (b *BlockType) ImpliedCtyType() cty.Type {
	atys := make(map[string]cty.Type)
	for name, attrS := range b.Attributes {
		atys[name] = attrS.Type
	}
	for name, blockS := range b.NestedBlockTypes {
		atys[name] = blockS.impliedCtyType()
	}
	return cty.Object(atys)
}

func (b *NestedBlockType) impliedCtyType() cty.Type {
	nested := b.Content.ImpliedCtyType()
	if b.Nesting == NestingSingle {
		return nested // easy case
	}

	if nested.HasDynamicTypes() {
		// If a multi-nesting block contains any dynamic-typed attributes then
		// it'll be passed in as either a tuple or an object type with full
		// type information in the payload, so for the purposes of our static
		// type constraint, the whole block type attribute is itself
		// dynamically-typed.
		return cty.DynamicPseudoType
	}

	switch b.Nesting {
	case NestingList:
		return cty.List(nested)
	case NestingSet:
		return cty.Set(nested)
	case NestingMap:
		return cty.Map(nested)
	default:
		// Invalid, so what we return here is undefined as far as our godoc is
		// concerned.
		return cty.DynamicPseudoType
	}
}

// ApplyDefaults takes an object value (that must conform to the receiving
// schema) and returns a new object value where any null attribute values in
// the given object are replaced with their default values from the schema.
//
// The result is guaranteed to also conform to the schema. This function may
// panic if the schema is incorrectly specified.
func (b *BlockType) ApplyDefaults(given cty.Value) cty.Value {
	vals := make(map[string]cty.Value)

	for name, attrS := range b.Attributes {
		gv := given.GetAttr(name)
		rv := gv
		if gv.IsNull() {
			switch {
			case attrS.Computed:
				rv = cty.UnknownVal(attrS.Type)
			default:
				rv = attrS.DefaultValue()
			}
		}
		vals[name] = rv
	}

	for name, blockS := range b.NestedBlockTypes {
		gv := given.GetAttr(name)
		vals[name] = blockS.ApplyDefaults(gv)
	}

	return cty.ObjectVal(vals)
}

// ApplyDefaults takes a value conforming to the type that represents blocks of
// the recieving nested block type and returns a new value, also conforming
// to that type, with the result of SchemaBlockType.ApplyDefaults applied to
// each element.
//
// This function expects that the given value will meet the guarantees offered
// by Terraform Core for values representing nested block types: they will always
// be known, and (aside from SchemaNestedSingle) never be null. If these
// guarantees don't hold then this function will panic.
func (b *NestedBlockType) ApplyDefaults(given cty.Value) cty.Value {
	wantTy := b.impliedCtyType()
	switch b.Nesting {
	case NestingSingle:
		if given.IsNull() {
			return given
		}
		return b.Content.ApplyDefaults(given)
	case NestingList:
		vals := make([]cty.Value, 0, given.LengthInt())
		for it := given.ElementIterator(); it.Next(); {
			_, gv := it.Element()
			vals = append(vals, b.Content.ApplyDefaults(gv))
		}
		if !wantTy.IsListType() {
			// Schema must contain dynamically-typed attributes then, so we'll
			// return a tuple to properly capture the possibly-inconsistent
			// element object types.
			return cty.TupleVal(vals)
		}
		if len(vals) == 0 {
			return cty.ListValEmpty(wantTy.ElementType())
		}
		return cty.ListVal(vals)
	case NestingMap:
		vals := make(map[string]cty.Value, given.LengthInt())
		for it := given.ElementIterator(); it.Next(); {
			k, gv := it.Element()
			vals[k.AsString()] = b.Content.ApplyDefaults(gv)
		}
		if !wantTy.IsMapType() {
			// Schema must contain dynamically-typed attributes then, so we'll
			// return an object to properly capture the possibly-inconsistent
			// element object types.
			return cty.ObjectVal(vals)
		}
		if len(vals) == 0 {
			return cty.MapValEmpty(wantTy.ElementType())
		}
		return cty.MapVal(vals)
	case NestingSet:
		vals := make([]cty.Value, 0, given.LengthInt())
		for it := given.ElementIterator(); it.Next(); {
			_, gv := it.Element()
			vals = append(vals, b.Content.ApplyDefaults(gv))
		}
		// Dynamically-typed attributes are not supported with SchemaNestingSet,
		// so we just always return a set value for these.
		if len(vals) == 0 {
			return cty.SetValEmpty(wantTy.ElementType())
		}
		return cty.SetVal(vals)
	default:
		panic(fmt.Sprintf("invalid block nesting mode %#v", b.Nesting))
	}
}
