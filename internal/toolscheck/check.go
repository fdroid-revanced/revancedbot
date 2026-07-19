package toolscheck

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/lucasew/revancedbot/internal/apksign"
)

// Requirement is a host tool we expect.
type Requirement struct {
	Name string
	Hint string
	// Special is an alternate check (e.g. apksigner.jar under ANDROID_HOME).
	Special func() error
}

// DefaultRun lists tools required for a full run / fdroid-update.
func DefaultRun() []Requirement {
	return []Requirement{
		{Name: "java", Hint: "install via mise (java temurin-21)"},
		{Name: "keytool", Hint: "comes with JDK"},
		{Name: "fdroid", Hint: "mise pipx:fdroidserver or host package"},
		{
			Name: "apksigner",
			Hint: "Android build-tools (mise android-sdk + sdkmanager \"build-tools;…\"); set ANDROID_HOME",
			Special: func() error {
				apksign.PrependBuildToolsPATH()
				return apksign.Available()
			},
		},
		{
			Name: "aapt",
			Hint: "Android build-tools on PATH or under ANDROID_HOME",
			Special: func() error {
				apksign.PrependBuildToolsPATH()
				if apksign.HasAapt() {
					return nil
				}
				return fmt.Errorf("aapt not found")
			},
		},
	}
}

// KeysOnly for keys generate/validate.
func KeysOnly() []Requirement {
	return []Requirement{
		{Name: "keytool", Hint: "comes with JDK"},
	}
}

// Check returns a multi-line error if any required tool is missing.
func Check(reqs []Requirement) error {
	var missing []string
	for _, r := range reqs {
		if r.Special != nil {
			if err := r.Special(); err != nil {
				msg := r.Name + ": " + err.Error()
				if r.Hint != "" {
					msg += " (" + r.Hint + ")"
				}
				missing = append(missing, msg)
			}
			continue
		}
		if _, err := exec.LookPath(r.Name); err != nil {
			msg := r.Name
			if r.Hint != "" {
				msg += " (" + r.Hint + ")"
			}
			missing = append(missing, msg)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing required tools:\n  - %s", strings.Join(missing, "\n  - "))
}
