package testing

import (
	"context"
	"fmt"

	tfsdk "github.com/apparentlymart/terraform-sdk"
	"github.com/zclconf/go-cty/cty"
)

type assertionsDRT struct {
	Subject *string `cty:"subject"`

	Check map[string]*assertionsDRTCheck `cty:"check"`
	Equal map[string]*assertionsDRTEqual `cty:"equal"`
}

type assertionsDRTEqual struct {
	Statement *string `cty:"statement"`

	Got  cty.Value `cty:"got"`
	Want cty.Value `cty:"want"`
}

type assertionsDRTCheck struct {
	Statement *string `cty:"statement"`

	Pass bool `cty:"expect"`
}

func assertionsDataResourceType() tfsdk.DataResourceType {
	return tfsdk.NewDataResourceType(&tfsdk.ResourceType{
		ConfigSchema: &tfsdk.SchemaBlockType{
			Attributes: map[string]*tfsdk.SchemaAttribute{
				"subject": {Type: cty.String, Optional: true},
			},
			NestedBlockTypes: map[string]*tfsdk.SchemaNestedBlockType{
				"check": {
					Nesting: tfsdk.SchemaNestingMap,
					Content: tfsdk.SchemaBlockType{
						Attributes: map[string]*tfsdk.SchemaAttribute{
							"statement": {Type: cty.String, Optional: true},

							"expect": {Type: cty.Bool, Required: true},
						},
					},
				},
				"equal": {
					Nesting: tfsdk.SchemaNestingMap,
					Content: tfsdk.SchemaBlockType{
						Attributes: map[string]*tfsdk.SchemaAttribute{
							"statement": {Type: cty.String, Optional: true},

							"want": {Type: cty.DynamicPseudoType, Required: true},
							"got":  {Type: cty.DynamicPseudoType, Required: true},
						},
					},
				},
			},
		},

		ReadFn: func(ctx context.Context, client *Client, obj *assertionsDRT) (*assertionsDRT, tfsdk.Diagnostics) {
			var diags tfsdk.Diagnostics

			subject := ""
			if obj.Subject != nil {
				subject = *obj.Subject
			}

			for k, chk := range obj.Check {
				if chk.Pass {
					continue
				}

				statement := ""
				if chk.Statement != nil {
					if subject != "" {
						statement = fmt.Sprintf("%s %s", subject, *chk.Statement)
					} else {
						statement = *chk.Statement
					}
				}

				msg := "Assertion failed"
				if statement != "" {
					msg = fmt.Sprintf("%s: %s", msg, statement)
				}

				diags = diags.Append(tfsdk.Diagnostic{
					Severity: tfsdk.Error,
					Summary:  "Test failure",
					Detail:   msg,
					Path:     cty.Path(nil).GetAttr("check").Index(cty.StringVal(k)).GetAttr("expect"),
				})
			}

			for k, eq := range obj.Equal {
				if eq.Got.RawEquals(eq.Want) {
					// Assertion passes!
					continue
				}

				statement := ""
				if eq.Statement != nil {
					if subject != "" {
						statement = fmt.Sprintf("%s %s", subject, *eq.Statement)
					} else {
						statement = *eq.Statement
					}
				}

				var msg string
				if statement != "" {
					msg = fmt.Sprintf("Assertion failed: %s\n    Want: %s\n    Got:  %s", statement, eq.Want, eq.Got)
				} else {
					msg = fmt.Sprintf("Assertion failed\n    Want: %s\n    Got:  %s", eq.Want, eq.Got)
				}

				diags = diags.Append(tfsdk.Diagnostic{
					Severity: tfsdk.Error,
					Summary:  "Test failure",
					Detail:   msg,
					Path:     cty.Path(nil).GetAttr("equal").Index(cty.StringVal(k)).GetAttr("got"),
				})
			}

			return obj, diags
		},
	})
}
