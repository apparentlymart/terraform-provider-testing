package testing

import (
	"fmt"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestFormatValue(t *testing.T) {
	tests := []struct {
		Val  cty.Value
		Want string
	}{
		{
			cty.NullVal(cty.DynamicPseudoType),
			`null`,
		},
		{
			cty.UnknownVal(cty.DynamicPseudoType),
			`(unknown)`, // should never happen, but we prefer not to crash here
		},
		{
			cty.StringVal(""),
			`""`,
		},
		{
			cty.StringVal("hello"),
			`"hello"`,
		},
		{
			cty.StringVal("hello\nworld"),
			`"hello\nworld"`, // Ideally we'd use heredoc syntax here for better readability, but we don't yet
		},
		{
			cty.Zero,
			`0`,
		},
		{
			cty.NumberIntVal(5),
			`5`,
		},
		{
			cty.NumberFloatVal(5.2),
			`5.2`,
		},
		{
			cty.False,
			`false`,
		},
		{
			cty.True,
			`true`,
		},
		{
			cty.EmptyObjectVal,
			`{}`,
		},
		{
			cty.ObjectVal(map[string]cty.Value{
				"a": cty.StringVal("b"),
			}),
			`{
  "a" = "b"
}`,
		},
		{
			cty.ObjectVal(map[string]cty.Value{
				"a": cty.StringVal("b"),
				"c": cty.StringVal("d"),
			}),
			`{
  "a" = "b"
  "c" = "d"
}`,
		},
		{
			cty.MapValEmpty(cty.String),
			`tomap({})`,
		},
		{
			cty.EmptyTupleVal,
			`[]`,
		},
		{
			cty.TupleVal([]cty.Value{
				cty.StringVal("b"),
			}),
			`[
  "b",
]`,
		},
		{
			cty.TupleVal([]cty.Value{
				cty.StringVal("b"),
				cty.StringVal("d"),
			}),
			`[
  "b",
  "d",
]`,
		},
		{
			cty.ListValEmpty(cty.String),
			`tolist([])`,
		},
		{
			cty.SetValEmpty(cty.String),
			`toset([])`,
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%#v", test.Val), func(t *testing.T) {
			got := formatValue(test.Val, 0)
			if got != test.Want {
				t.Errorf("wrong result\nvalue: %#v\ngot:   %s\nwant:  %s", test.Val, got, test.Want)
			}
		})
	}
}
