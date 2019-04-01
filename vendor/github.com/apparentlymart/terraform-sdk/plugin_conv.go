package tfsdk

import (
	"fmt"
	"sort"

	"github.com/apparentlymart/terraform-sdk/internal/tfplugin5"
	"github.com/apparentlymart/terraform-sdk/tfschema"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"
)

func convertSchemaBlockToTFPlugin5(src *tfschema.BlockType) *tfplugin5.Schema_Block {
	ret := &tfplugin5.Schema_Block{}
	if src == nil {
		// Weird, but we'll allow it.
		return ret
	}

	for name, attrS := range src.Attributes {
		tyJSON, err := attrS.Type.MarshalJSON()
		if err != nil {
			// Should never happen, since types should always be valid
			panic(fmt.Sprintf("failed to serialize %#v as JSON: %s", attrS.Type, err))
		}
		ret.Attributes = append(ret.Attributes, &tfplugin5.Schema_Attribute{
			Name:        name,
			Type:        tyJSON,
			Description: attrS.Description,
			Required:    attrS.Required,
			Optional:    attrS.Optional,
			Computed:    attrS.Computed || attrS.Default != nil,
			Sensitive:   attrS.Sensitive,
		})
	}

	for name, blockS := range src.NestedBlockTypes {
		nested := convertSchemaBlockToTFPlugin5(&blockS.Content)
		var nesting tfplugin5.Schema_NestedBlock_NestingMode
		switch blockS.Nesting {
		case tfschema.NestingSingle:
			nesting = tfplugin5.Schema_NestedBlock_SINGLE
		case tfschema.NestingList:
			nesting = tfplugin5.Schema_NestedBlock_LIST
		case tfschema.NestingMap:
			nesting = tfplugin5.Schema_NestedBlock_MAP
		case tfschema.NestingSet:
			nesting = tfplugin5.Schema_NestedBlock_SET
		default:
			// Should never happen because the above is exhaustive.
			panic(fmt.Sprintf("unsupported block nesting mode %#v", blockS.Nesting))
		}
		ret.BlockTypes = append(ret.BlockTypes, &tfplugin5.Schema_NestedBlock{
			TypeName: name,
			Nesting:  nesting,
			Block:    nested,
			MaxItems: int64(blockS.MaxItems),
			MinItems: int64(blockS.MinItems),
		})
	}

	sort.Slice(ret.Attributes, func(i, j int) bool {
		return ret.Attributes[i].Name < ret.Attributes[j].Name
	})

	return ret
}

func decodeTFPlugin5DynamicValue(src *tfplugin5.DynamicValue, schema *tfschema.BlockType) (cty.Value, Diagnostics) {
	switch {
	case len(src.Json) > 0:
		return decodeJSONObject(src.Json, schema)
	default:
		return decodeMsgpackObject(src.Msgpack, schema)
	}
}

func encodeTFPlugin5DynamicValue(src cty.Value, schema *tfschema.BlockType) *tfplugin5.DynamicValue {
	msgpackSrc := encodeMsgpackObject(src, schema)
	return &tfplugin5.DynamicValue{
		Msgpack: msgpackSrc,
	}
}

func decodeJSONObject(src []byte, schema *tfschema.BlockType) (cty.Value, Diagnostics) {
	var diags Diagnostics
	wantTy := schema.ImpliedCtyType()
	ret, err := json.Unmarshal(src, wantTy)
	if err != nil {
		var path cty.Path
		if pErr, ok := err.(cty.PathError); ok {
			path = pErr.Path
		}
		diags = diags.Append(Diagnostic{
			Severity: Error,
			Summary:  "Invalid object from Terraform Core",
			Detail:   fmt.Sprintf("Provider recieved an object value from Terraform Core that could not be decoded: %s.\n\nThis is a bug in either Terraform Core or in the plugin SDK; please report it in Terraform Core's repository.", err),
			Path:     path,
		})
	}
	return ret, diags
}

func decodeMsgpackObject(src []byte, schema *tfschema.BlockType) (cty.Value, Diagnostics) {
	var diags Diagnostics
	wantTy := schema.ImpliedCtyType()
	ret, err := msgpack.Unmarshal(src, wantTy)
	if err != nil {
		var path cty.Path
		if pErr, ok := err.(cty.PathError); ok {
			path = pErr.Path
		}
		diags = diags.Append(Diagnostic{
			Severity: Error,
			Summary:  "Invalid object from Terraform Core",
			Detail:   fmt.Sprintf("Provider recieved an object value from Terraform Core that could not be decoded: %s.\n\nThis is a bug in either Terraform Core or in the plugin SDK; please report it in Terraform Core's repository.", err),
			Path:     path,
		})
	}
	return ret, diags
}

func encodeMsgpackObject(src cty.Value, schema *tfschema.BlockType) []byte {
	wantTy := schema.ImpliedCtyType()
	ret, err := msgpack.Marshal(src, wantTy)
	if err != nil {
		// Errors in _encoding_ always indicate programming errors in the SDK,
		// since it should be checking these things on the way out.
		panic(fmt.Sprintf("invalid object to encode: %s", err))
	}
	return ret
}

func encodeDiagnosticsToTFPlugin5(src Diagnostics) []*tfplugin5.Diagnostic {
	var ret []*tfplugin5.Diagnostic
	for _, diag := range src {
		var severity tfplugin5.Diagnostic_Severity
		switch diag.Severity {
		case Error:
			severity = tfplugin5.Diagnostic_ERROR
		case Warning:
			severity = tfplugin5.Diagnostic_WARNING
		}

		ret = append(ret, &tfplugin5.Diagnostic{
			Severity:  severity,
			Summary:   diag.Summary,
			Detail:    diag.Detail,
			Attribute: encodeAttrPathToTFPlugin5(diag.Path),
		})
	}
	return ret
}

func encodeAttrPathToTFPlugin5(path cty.Path) *tfplugin5.AttributePath {
	ret := &tfplugin5.AttributePath{}
	for _, rawStep := range path {
		switch step := rawStep.(type) {
		case cty.GetAttrStep:
			ret.Steps = append(ret.Steps, &tfplugin5.AttributePath_Step{
				Selector: &tfplugin5.AttributePath_Step_AttributeName{
					AttributeName: step.Name,
				},
			})
		case cty.IndexStep:
			switch step.Key.Type() {
			case cty.String:
				ret.Steps = append(ret.Steps, &tfplugin5.AttributePath_Step{
					Selector: &tfplugin5.AttributePath_Step_ElementKeyString{
						ElementKeyString: step.Key.AsString(),
					},
				})
			case cty.Number:
				idx, _ := step.Key.AsBigFloat().Int64()
				ret.Steps = append(ret.Steps, &tfplugin5.AttributePath_Step{
					Selector: &tfplugin5.AttributePath_Step_ElementKeyInt{
						ElementKeyInt: idx,
					},
				})
			default:
				// no other key types are valid, so we'll produce garbage in this case
				// and have Terraform Core report it as such.
				ret.Steps = append(ret.Steps, nil)
			}
		}
	}
	return ret
}
