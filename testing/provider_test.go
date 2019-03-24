package testing

import (
	"os"
	"testing"

	"github.com/apparentlymart/terraform-sdk/tftest"
)

var testHelper *tftest.Helper

func TestMain(m *testing.M) {
	testHelper = tftest.InitProvider("testing", Provider())
	status := m.Run()
	testHelper.Close()
	os.Exit(status)
}
