package dynfunc

import (
	"fmt"
	"reflect"

	"github.com/apparentlymart/terraform-sdk/internal/sdkdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

var diagnosticsType = reflect.TypeOf(sdkdiags.Diagnostics(nil))
var ctyValueType = reflect.TypeOf(cty.Value{})

// WrapSimpleFunction dynamically binds the given arguments to the given
// function, or returns a developer-oriented error describing why it cannot.
// The given function must return only a tfsdk.Diagnostics value.
//
// If the requested call is valid, the result is a function that takes no
// arguments, executes the requested call, and returns any diagnostics that
// result.
//
// As a convenience, if the given function is nil then a no-op function will
// be returned, for the common situation where a dynamic function is optional.
func WrapSimpleFunction(f interface{}, args ...interface{}) (func() sdkdiags.Diagnostics, error) {
	if f == nil {
		return func() sdkdiags.Diagnostics {
			return nil
		}, nil
	}

	fv := reflect.ValueOf(f)
	if fv.Kind() != reflect.Func {
		return nil, fmt.Errorf("value is %s, not Func", fv.Kind().String())
	}

	ft := fv.Type()
	if ft.NumOut() != 1 || !ft.Out(0).AssignableTo(diagnosticsType) {
		return nil, fmt.Errorf("must return Diagnostics")
	}

	convArgs, forceDiags, err := prepareDynamicCallArgs(f, args...)
	if err != nil {
		return nil, err
	}

	return func() sdkdiags.Diagnostics {
		if len(forceDiags) > 0 {
			return forceDiags
		}

		out := fv.Call(convArgs)
		return out[0].Interface().(sdkdiags.Diagnostics)
	}, nil
}

// WrapFunctionWithReturnValue is like WrapSimpleFunction but expects the
// function to return another value alongside its diagnostics. The given
// result pointer will receive the function's return value if no diagnostics
// are returned.
//
// resultPtr must be a pointer, and the return type of the function must be
// compatible with resultPtr's referent.
func WrapFunctionWithReturnValue(f interface{}, resultPtr interface{}, args ...interface{}) (func() sdkdiags.Diagnostics, error) {
	rv := reflect.ValueOf(resultPtr)
	if rv.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("resultPtr is %s, not Ptr", rv.Kind().String())
	}
	wantRT := rv.Type().Elem()

	if f == nil {
		return func() sdkdiags.Diagnostics {
			rv.Elem().Set(reflect.Zero(wantRT))
			return nil
		}, nil
	}

	fv := reflect.ValueOf(f)
	if fv.Kind() != reflect.Func {
		return nil, fmt.Errorf("value is %s, not Func", fv.Kind().String())
	}

	ft := fv.Type()
	if ft.NumOut() != 2 {
		return nil, fmt.Errorf("must have two return values")
	}
	if !ft.Out(1).AssignableTo(diagnosticsType) {
		return nil, fmt.Errorf("second return value must be diagnostics")
	}
	if gotRT := ft.Out(0); !gotRT.AssignableTo(wantRT) {
		return nil, fmt.Errorf("function return type %s cannot be assigned to result of type %s", gotRT, wantRT)
	}

	convArgs, forceDiags, err := prepareDynamicCallArgs(f, args...)
	if err != nil {
		return nil, err
	}

	return func() sdkdiags.Diagnostics {
		if len(forceDiags) > 0 {
			return forceDiags
		}

		out := fv.Call(convArgs)
		retVal := out[0]
		diags := out[1].Interface().(sdkdiags.Diagnostics)

		rv.Elem().Set(retVal)
		return diags
	}, nil
}

// WrapFunctionWithReturnValueCty is like WrapFunctionWithReturnValue but with
// the return value specified as a cty value type rather than a Go pointer.
//
// Returns a function that will call the wrapped function, convert its result
// to cty.Value using gocty, and return it.
func WrapFunctionWithReturnValueCty(f interface{}, wantTy cty.Type, args ...interface{}) (func() (cty.Value, sdkdiags.Diagnostics), error) {
	if f == nil {
		return func() (cty.Value, sdkdiags.Diagnostics) {
			return cty.NullVal(wantTy), nil
		}, nil
	}

	fv := reflect.ValueOf(f)
	if fv.Kind() != reflect.Func {
		return nil, fmt.Errorf("value is %s, not Func", fv.Kind().String())
	}

	ft := fv.Type()
	if ft.NumOut() != 2 {
		return nil, fmt.Errorf("must have two return values")
	}
	if !ft.Out(1).AssignableTo(diagnosticsType) {
		return nil, fmt.Errorf("second return value must be diagnostics")
	}
	gotRT := ft.Out(0)
	passthruResult := false
	if ctyValueType.AssignableTo(gotRT) {
		passthruResult = true
	}

	convArgs, forceDiags, err := prepareDynamicCallArgs(f, args...)
	if err != nil {
		return nil, err
	}

	return func() (cty.Value, sdkdiags.Diagnostics) {
		if len(forceDiags) > 0 {
			return cty.NullVal(wantTy), forceDiags
		}

		out := fv.Call(convArgs)
		retValRaw := out[0].Interface()
		diags := out[1].Interface().(sdkdiags.Diagnostics)
		if passthruResult {
			return retValRaw.(cty.Value), diags
		}

		// If we're not just passing through then we need to run gocty first
		// to try to derive a suitable value from whatever we've been given.

		retVal, err := gocty.ToCtyValue(retValRaw, wantTy)
		if err != nil {
			if !diags.HasErrors() { // If the result was errored anyway then we'll tolerate this conversion failure.
				diags = diags.Append(sdkdiags.Diagnostic{
					Severity: sdkdiags.Error,
					Summary:  "Invalid result from provider",
					Detail:   fmt.Sprintf("The provider produced an invalid result: %s.\n\nThis is a bug in the provider; please report it in the provider's issue tracker.", sdkdiags.FormatError(err)),
				})
			}
			retVal = cty.NullVal(wantTy)
		}
		return retVal, diags
	}, nil
}

func prepareDynamicCallArgs(f interface{}, args ...interface{}) ([]reflect.Value, sdkdiags.Diagnostics, error) {
	fv := reflect.ValueOf(f)
	if fv.Kind() != reflect.Func {
		return nil, nil, fmt.Errorf("value is %s, not Func", fv.Kind().String())
	}

	ft := fv.Type()
	if got, want := ft.NumIn(), len(args); got != want {
		// (this error assumes that "args" is defined by the SDK code and thus
		// correct, while f comes from the provider and so is wrong.)
		return nil, nil, fmt.Errorf("should have %d arguments, but has %d", want, got)
	}

	var forceDiags sdkdiags.Diagnostics

	convArgs := make([]reflect.Value, len(args))
	for i, rawArg := range args {
		wantType := ft.In(i)
		switch arg := rawArg.(type) {
		case cty.Value:
			// As a special case, we handle cty.Value arguments through gocty.
			targetVal := reflect.New(wantType)
			err := gocty.FromCtyValue(arg, targetVal.Interface())
			if err != nil {
				// While most of the errors in here are written as if the
				// f interface is wrong, for this particular case we invert
				// that to consider the f argument as a way to specify
				// constraints on the user-provided value. However, we don't
				// have much context here for what the wrapped function is for,
				// so our error message is necessarily generic. Providers should
				// generally not rely on this error form and should instead
				// ensure that all user-supplyable values can be accepted.
				forceDiags = forceDiags.Append(sdkdiags.Diagnostic{
					Severity: sdkdiags.Error,
					Summary:  "Unsuitable argument value",
					Detail:   fmt.Sprintf("This value cannot be used: %s.", sdkdiags.FormatError(err)),
				})
			}
			convArgs[i] = targetVal.Elem() // New created a pointer, but we want the referent
		default:
			// All other arguments must be directly assignable.
			argVal := reflect.ValueOf(rawArg)
			if !argVal.Type().AssignableTo(wantType) {
				return nil, nil, fmt.Errorf("argument %d must accept %T", i, rawArg)
			}
			convArgs[i] = argVal
		}
	}

	return convArgs, forceDiags, nil
}
