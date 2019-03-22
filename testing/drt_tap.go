package testing

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/apparentlymart/go-test-anything/tap"
	tfsdk "github.com/apparentlymart/terraform-sdk"
	"github.com/zclconf/go-cty/cty"
)

type tapDRT struct {
	Program []string `cty:"program"`
}

func tapDataResourceType() tfsdk.DataResourceType {
	return tfsdk.NewDataResourceType(&tfsdk.ResourceType{
		ConfigSchema: &tfsdk.SchemaBlockType{
			Attributes: map[string]*tfsdk.SchemaAttribute{
				"program": {
					Type:     cty.List(cty.String),
					Required: true,
					ValidateFn: func(v []string) tfsdk.Diagnostics {
						var diags tfsdk.Diagnostics
						if len(v) < 1 {
							diags = diags.Append(tfsdk.ValidationError(
								cty.Path(nil).GetAttr("program").NewErrorf("must have at least one element to specify the executable to run"),
							))
						}
						return diags
					},
				},
			},
		},

		ReadFn: func(ctx context.Context, client *Client, obj *tapDRT) (*tapDRT, tfsdk.Diagnostics) {
			var diags tfsdk.Diagnostics

			cmd := exec.CommandContext(ctx, obj.Program[0], obj.Program[1:]...)
			var outBuf, errBuf bytes.Buffer
			cmd.Stdout = &outBuf
			cmd.Stderr = &errBuf

			err := cmd.Run()

			stderrForOutput := strings.Replace(errBuf.String(), "\n", "\n  ", -1)
			if stderrForOutput != "" {
				stderrForOutput = "The test program produced the following error messages:\n" + stderrForOutput
			}

			if err != nil {
				if stderrForOutput != "" {
					stderrForOutput = "\n\n" + stderrForOutput
				}
				diags = diags.Append(tfsdk.Diagnostic{
					Severity: tfsdk.Error,
					Summary:  "Test program failed",
					Detail:   fmt.Sprintf("Error running test program: %s.%s", err, stderrForOutput),
				})
				return obj, diags
			}

			r := tap.NewReader(&outBuf)
			report, err := r.ReadAll()
			if err != nil {
				if stderrForOutput != "" {
					stderrForOutput = "\n\n" + stderrForOutput
				}
				diags = diags.Append(tfsdk.Diagnostic{
					Severity: tfsdk.Error,
					Summary:  "Test program failed",
					Detail:   fmt.Sprintf("Error during test program: %s.%s", err, stderrForOutput),
				})
				return obj, diags
			}

			for _, test := range report.Tests {
				testName := test.Name
				if testName == "" {
					testName = fmt.Sprintf("anonymous test #%d", test.Num)
				}
				switch {
				case test.Result == tap.Fail || !test.Todo:
					diags = diags.Append(tfsdk.Diagnostic{
						Severity: tfsdk.Error,
						Summary:  "Test failure",
						Detail:   fmt.Sprintf("Test failed: %s.", testName),
					})
				case test.Result == tap.Pass && test.Todo:
					diags = diags.Append(tfsdk.Diagnostic{
						Severity: tfsdk.Warning,
						Summary:  "Test passed unexpectedly",
						Detail:   fmt.Sprintf("Bonus test pass: %s.\n\nThis test is marked as a TODO test, but yet it passed. Consider removing the TODO directive from this test.", testName),
					})
				}
			}

			if stderrForOutput != "" {
				diags = diags.Append(tfsdk.Diagnostic{
					Severity: tfsdk.Error,
					Summary:  "Error messages from test program",
					Detail:   stderrForOutput,
				})
			}

			return obj, diags
		},
	})
}
