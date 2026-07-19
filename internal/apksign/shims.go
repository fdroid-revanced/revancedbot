package apksign

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// EnsureShims writes executable wrappers under binDir for apksigner so
// fdroidserver can exec them (works around broken #!/bin/bash on NixOS SDK scripts).
func EnsureShims(binDir string) (string, error) {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	jar, err := findApksignerJar()
	if err != nil {
		return "", err
	}
	java := resolveJava()
	apksignerPath := filepath.Join(binDir, "apksigner")
	script := fmt.Sprintf("#!/usr/bin/env bash\nexec %s -jar %s \"$@\"\n",
		strconv.Quote(java), strconv.Quote(jar))
	if err := os.WriteFile(apksignerPath, []byte(script), 0o755); err != nil {
		return "", err
	}
	// symlink aapt from ANDROID_HOME if missing on PATH
	if _, err := exec.LookPath("aapt"); err != nil {
		for _, root := range []string{os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT")} {
			if root == "" {
				continue
			}
			matches, _ := filepath.Glob(filepath.Join(root, "build-tools", "*", "aapt"))
			if len(matches) > 0 {
				link := filepath.Join(binDir, "aapt")
				_ = os.Remove(link)
				_ = os.Symlink(matches[len(matches)-1], link)
				break
			}
		}
	}
	return binDir, nil
}

func resolveJava() string {
	if h := os.Getenv("JAVA_HOME"); h != "" {
		p := filepath.Join(h, "bin", "java")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if out, err := exec.Command("mise", "which", "java").Output(); err == nil {
		p := strings.TrimSpace(string(out))
		if p != "" {
			if real, err := filepath.EvalSymlinks(p); err == nil {
				return real
			}
			return p
		}
	}
	if p, err := exec.LookPath("java"); err == nil {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return real
		}
		return p
	}
	return "java"
}
