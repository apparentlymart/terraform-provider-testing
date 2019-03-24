package tftest

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// WorkingDir represents a distinct working directory that can be used for
// running tests. Each test should construct its own WorkingDir by calling
// NewWorkingDir or RequireNewWorkingDir on its package's singleton
// tftest.Helper.
type WorkingDir struct {
	h         *Helper
	baseDir   string
	configDir string
}

// Close deletes the directories and files created to represent the receiving
// working directory. After this method is called, the working directory object
// is invalid and may no longer be used.
func (wd *WorkingDir) Close() error {
	return os.RemoveAll(wd.baseDir)
}

// SetConfig sets a new configuration for the working directory.
//
// This must be called at least once before any call to Init, Plan, Apply, or
// Destroy to establish the configuration. Any previously-set configuration is
// discarded and any saved plan is cleared.
func (wd *WorkingDir) SetConfig(cfg string) error {
	// Each call to SetConfig creates a new directory under our baseDir.
	// We create them within so that our final cleanup step will delete them
	// automatically without any additional tracking.
	configDir, err := ioutil.TempDir(wd.baseDir, "config")
	if err != nil {
		return err
	}
	configFilename := filepath.Join(configDir, "test.tf")
	err = ioutil.WriteFile(configFilename, []byte(cfg), 0700)
	if err != nil {
		return err
	}
	wd.configDir = configDir

	// Changing configuration invalidates any saved plan.
	err = wd.ClearPlan()
	if err != nil {
		return err
	}
	return nil
}

// RequireSetConfig is a variant of SetConfig that will fail the test via the
// given TestControl if the configuration cannot be set.
func (wd *WorkingDir) RequireSetConfig(t TestControl, cfg string) {
	t.Helper()
	if err := wd.SetConfig(cfg); err != nil {
		t := testingT{t}
		t.Fatalf("failed to set config: %s", err)
	}
}

// ClearState deletes any Terraform state present in the working directory.
//
// Any remote objects tracked by the state are not destroyed first, so this
// will leave them dangling in the remote system.
func (wd *WorkingDir) ClearState() error {
	err := os.Remove(filepath.Join(wd.baseDir, "terraform.tfstate"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RequireClearState is a variant of ClearState that will fail the test via the
// given TestControl if the state cannot be cleared.
func (wd *WorkingDir) RequireClearState(t TestControl) {
	t.Helper()
	if err := wd.ClearState(); err != nil {
		t := testingT{t}
		t.Fatalf("failed to clear state: %s", err)
	}
}

// ClearPlan deletes any saved plan present in the working directory.
func (wd *WorkingDir) ClearPlan() error {
	err := os.Remove(wd.planFilename())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RequireClearPlan is a variant of ClearPlan that will fail the test via the
// given TestControl if the plan cannot be cleared.
func (wd *WorkingDir) RequireClearPlan(t TestControl) {
	t.Helper()
	if err := wd.ClearPlan(); err != nil {
		t := testingT{t}
		t.Fatalf("failed to clear plan: %s", err)
	}
}

func (wd *WorkingDir) init(pluginDir string) error {
	return wd.runTerraform("init", "-plugin-dir="+pluginDir, wd.configDir)
}

// Init runs "terraform init" for the given working directory, forcing Terraform
// to use the current version of the plugin under test.
func (wd *WorkingDir) Init() error {
	if wd.configDir == "" {
		return fmt.Errorf("must call SetConfig before Init")
	}
	return wd.init(wd.h.PluginDir())
}

// RequireInit is a variant of Init that will fail the test via the given
// TestControl if init fails.
func (wd *WorkingDir) RequireInit(t TestControl) {
	t.Helper()
	if err := wd.Init(); err != nil {
		t := testingT{t}
		t.Fatalf("init failed: %s", err)
	}
}

// InitPrevious runs "terraform init" for the given working directory, forcing
// Terraform to use the previous version of the plugin under test.
//
// This method will panic if no previous plugin version is available. Use
// HasPreviousVersion or RequirePreviousVersion on the test helper singleton
// to check this first.
func (wd *WorkingDir) InitPrevious() error {
	if wd.configDir == "" {
		return fmt.Errorf("must call SetConfig before InitPrevious")
	}
	return wd.init(wd.h.PreviousPluginDir())
}

// RequireInitPrevious is a variant of InitPrevious that will fail the test
// via the given TestControl if init fails.
func (wd *WorkingDir) RequireInitPrevious(t TestControl) {
	t.Helper()
	if err := wd.InitPrevious(); err != nil {
		t := testingT{t}
		t.Fatalf("init failed: %s", err)
	}
}

func (wd *WorkingDir) planFilename() string {
	return filepath.Join(wd.baseDir, "tfplan")
}

// CreatePlan runs "terraform plan" to create a saved plan file, which if successful
// will then be used for the next call to Apply.
func (wd *WorkingDir) CreatePlan() error {
	return wd.runTerraform("plan", "-out=tfplan", wd.configDir)
}

// RequireCreatePlan is a variant of CreatePlan that will fail the test via
// the given TestControl if plan creation fails.
func (wd *WorkingDir) RequireCreatePlan(t TestControl) {
	t.Helper()
	if err := wd.CreatePlan(); err != nil {
		t := testingT{t}
		t.Fatalf("failed to create plan: %s", err)
	}
}

// HasSavedPlan returns true if there is a saved plan in the working directory. If
// so, a subsequent call to Apply will apply that saved plan.
func (wd *WorkingDir) HasSavedPlan() bool {
	_, err := os.Stat(wd.planFilename())
	return err == nil
}

// Apply runs "terraform apply". If Plan has previously completed successfully
// and the saved plan has not been cleared in the meantime then ths will apply
// the saved plan. Otherwise, it will implicitly create a new plan and apply it.
func (wd *WorkingDir) Apply() error {
	args := []string{"apply"}
	if wd.HasSavedPlan() {
		args = append(args, "tfplan")
	} else {
		args = append(args, "-auto-approve")
	}
	args = append(args, wd.configDir)
	return wd.runTerraform(args...)
}

// RequireApply is a variant of Apply that will fail the test via
// the given TestControl if the apply operation fails.
func (wd *WorkingDir) RequireApply(t TestControl) {
	t.Helper()
	if err := wd.Apply(); err != nil {
		t := testingT{t}
		t.Fatalf("failed to apply: %s", err)
	}
}
