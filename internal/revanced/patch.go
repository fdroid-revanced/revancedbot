package revanced

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lucasew/revancedbot/internal/signing"
)

// PatchOptions controls a single patch invocation.
type PatchOptions struct {
	JavaBin      string
	CLIJar       string
	PatchesRVP   string
	InputAPK     string
	OutputAPK    string
	KeystorePath string
	Blob         *signing.Blob
	// EnableChangePackageName forces the package rename (append .revanced default).
	EnableChangePackageName bool
}

// Patch runs revanced-cli patch with defaults/recommended and operator keystore.
func Patch(opts PatchOptions) (appliedPatches []string, err error) {
	java := opts.JavaBin
	if java == "" {
		java = "java"
	}
	args := []string{
		"-jar", opts.CLIJar,
		"patch",
		opts.InputAPK,
		"-o", opts.OutputAPK,
		"-p", opts.PatchesRVP,
	}
	if opts.Blob != nil && opts.KeystorePath != "" {
		// Common ReVanced CLI keystore flags (v4+/v5+ style).
		args = append(args,
			"--keystore", opts.KeystorePath,
			"--keystore-password", opts.Blob.StorePass,
			"--keystore-entry-alias", opts.Blob.Alias,
			"--keystore-entry-password", opts.Blob.KeyPass,
		)
	}
	// Package rename: enable the patch if the CLI supports --enable.
	// Different CLI versions use different flag names; try the common form.
	if opts.EnableChangePackageName {
		args = append(args, "--enable", "Change package name")
	}

	cmd := exec.Command(java, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("patch %s: %w\n%s%s", opts.InputAPK, err, stdout.String(), stderr.String())
	}
	return parseAppliedPatches(stdout.String() + "\n" + stderr.String()), nil
}

func parseAppliedPatches(log string) []string {
	var out []string
	for _, line := range strings.Split(log, "\n") {
		// Heuristic: lines mentioning "Applying" or similar.
		l := strings.TrimSpace(line)
		if strings.Contains(l, "Applying") || strings.Contains(l, "applied") {
			out = append(out, l)
		}
	}
	return out
}
