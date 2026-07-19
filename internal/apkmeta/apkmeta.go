// Package apkmeta reads version metadata from APKs (via aapt/aapt2).
package apkmeta

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lucasew/revancedbot/internal/apksign"
)

// Info is package identity from an APK.
type Info struct {
	PackageID   string
	VersionName string
	VersionCode string
}

var (
	rePackage     = regexp.MustCompile(`name='([^']+)'`)
	reVersionName = regexp.MustCompile(`versionName='([^']*)'`)
	reVersionCode = regexp.MustCompile(`versionCode='([^']*)'`)
)

// Inspect runs aapt dump badging and parses package/version fields.
func Inspect(apkPath string) (Info, error) {
	out, err := dumpBadging(apkPath)
	if err != nil {
		return Info{}, err
	}
	return ParseBadging(out)
}

// VersionName returns versionName from the APK, or an error if unavailable.
func VersionName(apkPath string) (string, error) {
	info, err := Inspect(apkPath)
	if err != nil {
		return "", err
	}
	if info.VersionName == "" {
		return "", fmt.Errorf("empty versionName in %s", apkPath)
	}
	return info.VersionName, nil
}

// ParseBadging parses aapt dump badging output.
func ParseBadging(out string) (Info, error) {
	var info Info
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "package:") {
			continue
		}
		if m := rePackage.FindStringSubmatch(line); len(m) == 2 {
			info.PackageID = m[1]
		}
		if m := reVersionName.FindStringSubmatch(line); len(m) == 2 {
			info.VersionName = m[1]
		}
		if m := reVersionCode.FindStringSubmatch(line); len(m) == 2 {
			info.VersionCode = m[1]
		}
		break
	}
	if info.VersionName == "" && info.VersionCode == "" {
		return Info{}, fmt.Errorf("no package: line with version in aapt output")
	}
	return info, nil
}

func dumpBadging(apkPath string) (string, error) {
	apksign.PrependBuildToolsPATH()
	bin, err := findAapt()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(bin, "dump", "badging", apkPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// aapt2 uses the same subcommand shape for dump badging on modern build-tools.
		return "", fmt.Errorf("%s dump badging: %w\n%s", bin, err, out)
	}
	return string(out), nil
}

func findAapt() (string, error) {
	if p, err := exec.LookPath("aapt"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("aapt2"); err == nil {
		return p, nil
	}
	for _, root := range []string{os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT")} {
		if root == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, "build-tools", "*", "aapt"))
		if len(matches) > 0 {
			// last match tends to be newer version string
			return matches[len(matches)-1], nil
		}
		matches, _ = filepath.Glob(filepath.Join(root, "build-tools", "*", "aapt2"))
		if len(matches) > 0 {
			return matches[len(matches)-1], nil
		}
	}
	return "", fmt.Errorf("aapt/aapt2 not found (set ANDROID_HOME or put build-tools on PATH)")
}
