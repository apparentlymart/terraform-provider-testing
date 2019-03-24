package testing

import "testing"

func TestDRTAssertions(t *testing.T) {
	t.Run("equal pass", func(t *testing.T) {
		wd := testHelper.RequireNewWorkingDir(t)
		defer wd.Close()

		wd.RequireSetConfig(t, `
data "testing_assertions" "test" {
  equal "foo" {
	got  = "a"
	want = "a"
  }
}
`)

		wd.RequireInit(t)
		wd.RequireApply(t)
	})
	t.Run("equal fail", func(t *testing.T) {
		wd := testHelper.RequireNewWorkingDir(t)
		defer wd.Close()

		wd.RequireSetConfig(t, `
data "testing_assertions" "test" {
  equal "foo" {
	got  = "a"
	want = "b"
  }
}
`)

		wd.RequireInit(t)
		err := wd.Apply()
		if err == nil {
			t.Error("succeeded; want error")
		}
	})
	t.Run("check pass", func(t *testing.T) {
		wd := testHelper.RequireNewWorkingDir(t)
		defer wd.Close()

		wd.RequireSetConfig(t, `
data "testing_assertions" "test" {
  check "foo" {
	expect = true
  }
}
`)

		wd.RequireInit(t)
		wd.RequireApply(t)
	})
	t.Run("check fail", func(t *testing.T) {
		wd := testHelper.RequireNewWorkingDir(t)
		defer wd.Close()

		wd.RequireSetConfig(t, `
data "testing_assertions" "test" {
  check "foo" {
	expect = false
  }
}
`)

		wd.RequireInit(t)
		err := wd.Apply()
		if err == nil {
			t.Error("succeeded; want error")
		}
	})
}
