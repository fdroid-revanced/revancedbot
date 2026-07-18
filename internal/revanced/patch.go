package revanced

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/lucasew/revancedbot/internal/apksign"
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

// Patch runs revanced-cli patch (unsigned-by-us), then re-signs with operator key via apksigner.
// ReVanced's BouncyCastle cannot load modern keytool keystores reliably, so we do not pass --keystore to CLI.
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
		"-b",
	}
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
	applied := parseAppliedPatches(stdout.String() + "\n" + stderr.String())

	if opts.Blob != nil && opts.KeystorePath != "" {
		if err := apksign.Sign(opts.OutputAPK, opts.KeystorePath, opts.Blob.StorePass, opts.Blob.KeyPass, opts.Blob.Alias); err != nil {
			return applied, fmt.Errorf("re-sign with operator key: %w", err)
		}
	}
	return applied, nil
}

func parseAppliedPatches(log string) []string {
	var out []string
	for _, line := range strings.Split(log, "\n") {
		l := strings.TrimSpace(line)
		if strings.Contains(l, "succeeded") || strings.Contains(l, "Applying") {
			out = append(out, l)
		}
	}
	return out
}
