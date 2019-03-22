package testing

import (
	"context"

	tfsdk "github.com/apparentlymart/terraform-sdk"
)

func Provider() *tfsdk.Provider {
	return &tfsdk.Provider{
		ConfigSchema: &tfsdk.SchemaBlockType{
			Attributes: map[string]*tfsdk.SchemaAttribute{},
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
