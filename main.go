package main

import (
	provider "github.com/apparentlymart/terraform-provider-testing/testing"
	tfsdk "github.com/apparentlymart/terraform-sdk"
)

func main() {
	tfsdk.ServeProviderPlugin(provider.Provider())
}
