package testing

import (
	"context"
	"fmt"

	tfsdk "github.com/apparentlymart/terraform-sdk"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

type assertionsDRT struct {
	Subject *string `cty:"subject"`

	Checks cty.Value `cty:"check"`
	Equals cty.Value `cty:"equal"`
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

			for it := obj.Checks.ElementIterator(); it.Next(); {
				k, v := it.Element()
				var chk assertionsDRTCheck
				err := gocty.FromCtyValue(v, &chk)
				if err != nil {
					// Should never happen; indicates that our struct is wrong.
					diags = diags.Append(tfsdk.Diagnostic{
						Severity: tfsdk.Error,
						Summary:  "Bug in 'testing' provider",
						Detail:   fmt.Sprintf("The provider encountered a problem while decoding the check %q block: %s.\n\nThis is a bug in the provider; please report it in the provider's issue tracker.", k.AsString(), err),
					})
					continue
				}

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
					msg = fmt.Sprintf("%s: %s.", msg, statement)
				} else {
					msg = msg + "."
				}

				diags = diags.Append(tfsdk.Diagnostic{
					Severity: tfsdk.Error,
					Summary:  "Test failure",
					Detail:   msg,
					Path:     cty.Path(nil).GetAttr("check").Index(k).GetAttr("expect"),
				})
			}

			for it := obj.Equals.ElementIterator(); it.Next(); {
				k, v := it.Element()
				var eq assertionsDRTEqual
				err := gocty.FromCtyValue(v, &eq)
				if err != nil {
					// Should never happen; indicates that our struct is wrong.
					diags = diags.Append(tfsdk.Diagnostic{
						Severity: tfsdk.Error,
						Summary:  "Bug in 'testing' provider",
						Detail:   fmt.Sprintf("The provider encountered a problem while decoding the equal %q block: %s.\n\nThis is a bug in the provider; please report it in the provider's issue tracker.", k.AsString(), err),
					})
					continue
				}

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
					msg = fmt.Sprintf(
						"Assertion failed: %s.\n  Want: %s\n  Got:  %s",
						statement,
						formatValue(eq.Want, 2),
						formatValue(eq.Got, 2),
					)
				} else {
					msg = fmt.Sprintf("Assertion failed.\n  Want: %s\n  Got:  %s", eq.Want, eq.Got)
				}

				diags = diags.Append(tfsdk.Diagnostic{
					Severity: tfsdk.Error,
					Summary:  "Test failure",
					Detail:   msg,
					Path:     cty.Path(nil).GetAttr("equal").Index(k).GetAttr("got"),
				})
			}

			return obj, diags
		},
	})
}
