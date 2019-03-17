package tfsdk

import (
	"context"
	"fmt"

	"github.com/apparentlymart/terraform-sdk/internal/dynfunc"
	"github.com/zclconf/go-cty/cty"
)

// ResourceType is the type that provider packages should instantiate to
// implement a specific resource type.
//
// Pointers to instances of this type can be passed to the functions
// NewManagedResourceType and NewDataResourceType to provide managed and
// data resource type implementations respectively.
type ResourceType struct {
	ConfigSchema  *SchemaBlockType
	SchemaVersion int64 // Only used for managed resource types; leave as zero otherwise

	// CreateFn is a function called when creating an instance of your resource
	// type for the first time. It must be a function compatible with the
	// following signature:
	//
	//     func (ctx context.Context, planned cty.Value) (new cty.Value, diags tfsdk.Diagnostics)
	//
	// If the create was not completely successful, you may still return a
	// partially-created object alongside error diagnostics to retain the parts
	// that _were_ created.
	CreateFn interface{}

	// ReadFn is a function called to read the current upstream values for an
	// instance of your resource type. It must be a function compatible with the
	// following signature:
	//
	//     func (ctx context.Context, prior cty.Value) (new cty.Value, diags tfsdk.Diagnostics)
	//
	// If the given object appears to have been deleted upstream, return a null
	// value to indicate that. The object will then be removed from the Terraform
	// state.
	ReadFn interface{}

	// UpdateFn is a function called when performing an in-place update of an
	// instance of your resource type. It must be a function compatible with the
	// following signature:
	//
	//     func (ctx context.Context, prior cty.Value, planned cty.Value) (new cty.Value, diags tfsdk.Diagnostics)
	//
	// If the update is not completely successful, you may still return a
	// partially-updated object alongside error diagnostics to retain the
	// parts that _were_ updated. If error diagnostics are returned and the
	// returned value is null then we assume that the update failed completely
	// and retain the prior value in the Terraform state.
	UpdateFn interface{}

	// DeleteFn is a function called to delete an instance of your resource type.
	// It must be a function compatible with the following signature:
	//
	//     func (ctx context.Context, prior cty.Value) tfsdk.Diagnostics
	//
	// If error diagnostics are returned, the SDK will assume that the delete
	// failed and that the object still exists. If it actually was deleted
	// before the failure, this should be detected on the next Read call.
	DeleteFn interface{}
}

// NewManagedResourceType prepares a ManagedResourceType implementation using
// the definition from the given ResourceType instance.
//
// This function is intended to be called during startup with a valid
// ResourceType, so it will panic if the given ResourceType is not valid.
func NewManagedResourceType(def *ResourceType) ManagedResourceType {
	if def == nil {
		panic("NewManagedResourceType called with nil definition")
	}

	schema := def.ConfigSchema
	if schema == nil {
		schema = &SchemaBlockType{}
	}

	readFn := def.ReadFn
	if readFn == nil {
		readFn = defaultReadFn
	}

	// TODO: Check thoroughly to make sure def is correctly populated for a
	// managed resource type, so we can panic early.

	return managedResourceType{
		configSchema: schema,

		createFn: def.CreateFn,
		readFn:   readFn,
		updateFn: def.UpdateFn,
		deleteFn: def.DeleteFn,
	}
}

// NewDataResourceType prepares a DataResourceType implementation using the
// definition from the given ResourceType instance.
//
// This function is intended to be called during startup with a valid
// ResourceType, so it will panic if the given ResourceType is not valid.
func NewDataResourceType(def *ResourceType) DataResourceType {
	if def == nil {
		panic("NewDataResourceType called with nil definition")
	}

	schema := def.ConfigSchema
	if schema == nil {
		schema = &SchemaBlockType{}
	}
	if def.SchemaVersion != 0 {
		panic("NewDataResourceType requires def.SchemaVersion == 0")
	}

	readFn := def.ReadFn
	if readFn == nil {
		readFn = defaultReadFn
	}

	// TODO: Check thoroughly to make sure def is correctly populated for a data
	// resource type, so we can panic early.

	return dataResourceType{
		configSchema: schema,
		readFn:       readFn,
	}
}

type managedResourceType struct {
	configSchema  *SchemaBlockType
	schemaVersion int64

	createFn, readFn, updateFn, deleteFn interface{}
}

func (rt managedResourceType) getSchema() (schema *SchemaBlockType, version int64) {
	return rt.configSchema, rt.schemaVersion
}

func (rt managedResourceType) validate(obj cty.Value) Diagnostics {
	return rt.configSchema.Validate(obj)
}

func (rt managedResourceType) upgradeState(oldJSON []byte, oldVersion int) (cty.Value, Diagnostics) {
	return cty.NilVal, nil
}

func (rt managedResourceType) refresh(ctx context.Context, client interface{}, current cty.Value) (cty.Value, Diagnostics) {
	var diags Diagnostics
	wantTy := rt.configSchema.ImpliedCtyType()

	fn, err := dynfunc.WrapFunctionWithReturnValueCty(rt.readFn, wantTy, ctx, client, current)
	if err != nil {
		diags = diags.Append(Diagnostic{
			Severity: Error,
			Summary:  "Invalid provider implementation",
			Detail:   fmt.Sprintf("Invalid ReadFn: %s.\nThis is a bug in the provider that should be reported in its own issue tracker.", err),
		})
		return rt.configSchema.Null(), diags
	}

	newVal, moreDiags := fn()
	diags = diags.Append(moreDiags)

	// We'll make life easier on the provider implementer by normalizing null
	// and unknown values to the correct type automatically, so they can just
	// return dynamically-typed nulls and unknowns.
	switch {
	case newVal.IsNull():
		newVal = cty.NullVal(wantTy)
	case !newVal.IsKnown():
		newVal = cty.UnknownVal(wantTy)
	}

	return newVal, diags
}

func (rt managedResourceType) planChange(ctx context.Context, client interface{}, prior, config, proposed cty.Value) (cty.Value, Diagnostics) {
	var diags Diagnostics

	// Terraform Core has already done a lot of the work in merging prior with
	// config to produce "proposed". Our main job here is inserting any additional
	// default values called for in the provider schema.
	planned := rt.configSchema.ApplyDefaults(proposed)

	// TODO: We should also give the provider code an opportunity to make
	// further changes to the "Computed" parts of the planned value so it
	// can use its own logic, or possibly remote API calls, to produce the
	// most accurate plan.

	return planned, diags
}

func (rt managedResourceType) applyChange(ctx context.Context, client interface{}, prior, planned cty.Value) (cty.Value, Diagnostics) {
	var diags Diagnostics
	wantTy := rt.configSchema.ImpliedCtyType()

	// The planned object will contain unknown values for anything that is to
	// be determined during the apply step, but we'll replace these with nulls
	// before calling the provider's operation implementation functions so that
	// they can easily use gocty to work with the whole object and not get
	// tripped up with dealing with those unknown values.
	//
	// FIXME: This is a bit unfortunate because it means that the apply functions
	// can't easily tell the difference between something that was returned as
	// explicitly null in the plan vs. being unknown, but we're accepting that
	// for now because it seems unlikely that such a distinction would ever
	// matter in practice: the plan logic should just be consistent about whether
	// a particular attribute becomes unknown when it's unset. We might need to
	// do something better here if real-world experience indicates otherwise.
	//
	// This will also cause set values that differ only by being unknown to
	// be conflated together, but we're ignoring that here because we want to
	// phase out the idea of set-backed blocks with unknown attributes inside:
	// they cause too much ambiguity in our diffing logic.
	planned = cty.UnknownAsNull(planned)

	// We could actually be doing either a Create, an Update, or a Delete here
	// depending on the null-ness of the values we've been given. At least one
	// of them will always be non-null.
	var fn func() (cty.Value, Diagnostics)
	var err error
	var errMsg string
	switch {
	case prior.IsNull():
		fn, err = dynfunc.WrapFunctionWithReturnValueCty(rt.createFn, wantTy, ctx, client, planned)
		if err != nil {
			errMsg = fmt.Sprintf("Invalid CreateFn: %s.\nThis is a bug in the provider that should be reported in its own issue tracker.", err)
		}
	case planned.IsNull():
		fn, err = dynfunc.WrapFunctionWithReturnValueCty(rt.deleteFn, wantTy, ctx, client, prior)
		if err != nil {
			errMsg = fmt.Sprintf("Invalid DeleteFn: %s.\nThis is a bug in the provider that should be reported in its own issue tracker.", err)
		}
	default:
		fn, err = dynfunc.WrapFunctionWithReturnValueCty(rt.updateFn, wantTy, ctx, client, prior, planned)
		if err != nil {
			errMsg = fmt.Sprintf("Invalid UpdateFn: %s.\nThis is a bug in the provider that should be reported in its own issue tracker.", err)
		}
	}
	if err != nil {
		diags = diags.Append(Diagnostic{
			Severity: Error,
			Summary:  "Invalid provider implementation",
			Detail:   errMsg,
		})
		return rt.configSchema.Null(), diags
	}

	newVal, moreDiags := fn()
	diags = diags.Append(moreDiags)

	// We'll make life easier on the provider implementer by normalizing null
	// and unknown values to the correct type automatically, so they can just
	// return dynamically-typed nulls and unknowns.
	switch {
	case newVal.IsNull():
		newVal = cty.NullVal(wantTy)
	case !newVal.IsKnown():
		newVal = cty.UnknownVal(wantTy)
	}

	return newVal, diags
}

func (rt managedResourceType) importState(ctx context.Context, client interface{}, id string) (cty.Value, Diagnostics) {
	return cty.NilVal, nil
}

type dataResourceType struct {
	configSchema *SchemaBlockType

	readFn interface{}
}

func (rt dataResourceType) getSchema() *SchemaBlockType {
	return rt.configSchema
}

func (rt dataResourceType) validate(obj cty.Value) Diagnostics {
	return rt.configSchema.Validate(obj)
}

func (rt dataResourceType) read(ctx context.Context, client interface{}, config cty.Value) (cty.Value, Diagnostics) {
	var diags Diagnostics
	wantTy := rt.configSchema.ImpliedCtyType()

	fn, err := dynfunc.WrapFunctionWithReturnValueCty(rt.readFn, wantTy, ctx, client, config)
	if err != nil {
		diags = diags.Append(Diagnostic{
			Severity: Error,
			Summary:  "Invalid provider implementation",
			Detail:   fmt.Sprintf("Invalid ReadFn: %s.\nThis is a bug in the provider that should be reported in its own issue tracker.", err),
		})
		return rt.configSchema.Null(), diags
	}

	newVal, moreDiags := fn()
	diags = diags.Append(moreDiags)

	// We'll make life easier on the provider implementer by normalizing null
	// and unknown values to the correct type automatically, so they can just
	// return dynamically-typed nulls and unknowns.
	switch {
	case newVal.IsNull():
		newVal = cty.NullVal(wantTy)
	case !newVal.IsKnown():
		newVal = cty.UnknownVal(wantTy)
	}

	return newVal, diags
}

func defaultReadFn(ctx context.Context, client interface{}, v cty.Value) (cty.Value, Diagnostics) {
	return cty.UnknownAsNull(v), nil
}
