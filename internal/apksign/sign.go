package apksign

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Sign re-signs an APK with apksigner (operator keystore).
// On NixOS the apksigner shell wrapper may have a broken #!/bin/bash;
// prefer java -jar .../lib/apksigner.jar when found.
func Sign(apkPath, keystorePath, storePass, keyPass, alias string) error {
	jar, err := findApksignerJar()
	if err != nil {
		// fall back to apksigner on PATH
		return runApksignerBinary(apkPath, keystorePath, storePass, keyPass, alias)
	}
	args := []string{
		"-jar", jar,
		"sign",
		"--ks", keystorePath,
		"--ks-pass", "pass:" + storePass,
		"--key-pass", "pass:" + keyPass,
		"--ks-key-alias", alias,
		apkPath,
	}
	cmd := exec.Command("java", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("apksigner: %w\n%s", err, out)
	}
	return nil
}

func runApksignerBinary(apkPath, keystorePath, storePass, keyPass, alias string) error {
	cmd := exec.Command("apksigner",
		"sign",
		"--ks", keystorePath,
		"--ks-pass", "pass:"+storePass,
		"--key-pass", "pass:"+keyPass,
		"--ks-key-alias", alias,
		apkPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("apksigner: %w\n%s", err, out)
	}
	return nil
}

func findApksignerJar() (string, error) {
	// ANDROID_HOME/build-tools/*/lib/apksigner.jar
	for _, root := range []string{os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT")} {
		if root == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, "build-tools", "*", "lib", "apksigner.jar"))
		if len(matches) > 0 {
			// pick last (often newest version string)
			return matches[len(matches)-1], nil
		}
	}
	// next to apksigner on PATH
	if p, err := exec.LookPath("apksigner"); err == nil {
		// resolve and look for lib/apksigner.jar sibling
		dir := filepath.Dir(p)
		cand := filepath.Join(dir, "lib", "apksigner.jar")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand, nil
		}
		// follow symlink
		if real, err := filepath.EvalSymlinks(p); err == nil {
			cand = filepath.Join(filepath.Dir(real), "lib", "apksigner.jar")
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				return cand, nil
			}
		}
	}
	return "", fmt.Errorf("apksigner.jar not found (set ANDROID_HOME or put build-tools on PATH)")
}

// Available reports whether signing is possible.
func Available() error {
	if _, err := findApksignerJar(); err == nil {
		if _, e := exec.LookPath("java"); e != nil {
			return fmt.Errorf("java required to run apksigner.jar")
		}
		return nil
	}
	if _, err := exec.LookPath("apksigner"); err != nil {
		return fmt.Errorf("apksigner not found on PATH and apksigner.jar not under ANDROID_HOME")
	}
	return nil
}

// Ensure aapt is on PATH (simple check used by preflight elsewhere).
func HasAapt() bool {
	_, err := exec.LookPath("aapt")
	if err == nil {
		return true
	}
	// also accept aapt under ANDROID_HOME build-tools
	for _, root := range []string{os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT")} {
		if root == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, "build-tools", "*", "aapt"))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// PrependBuildToolsPATH adds ANDROID_HOME/build-tools/<ver> to PATH if needed.
func PrependBuildToolsPATH() {
	for _, root := range []string{os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT")} {
		if root == "" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(root, "build-tools", "*"))
		for i := len(matches) - 1; i >= 0; i-- {
			d := matches[i]
			if st, err := os.Stat(d); err == nil && st.IsDir() {
				os.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
				return
			}
		}
	}
}

// unused import guard
