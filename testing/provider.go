package testing

import (
	"context"

	tfsdk "github.com/apparentlymart/terraform-sdk"
	"github.com/apparentlymart/terraform-sdk/tfschema"
)

func Provider() *tfsdk.Provider {
	return &tfsdk.Provider{
		ConfigSchema: &tfschema.BlockType{
			Attributes: map[string]*tfschema.Attribute{},
		},
		ConfigureFn: func(ctx context.Context, config *Config) (*Client, tfsdk.Diagnostics) {
			return &Client{}, nil
		},

		DataResourceTypes: map[string]tfsdk.DataResourceType{
			"testing_assertions": assertionsDataResourceType(),
			"testing_tap":        tapDataResourceType(),
		},
	}
}

type Config struct {
}

type Client struct {
}
