package revanced

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Job is one package with ordered preferred versions (most common first).
// Empty Version means "Any" / latest at the source.
type Job struct {
	PackageID string
	Versions  []string // preferred order; empty string element means Any/latest
}

// ListJobs runs revanced-cli list-versions and groups by package.
func ListJobs(javaBin, cliJar, patchesRVP string) ([]Job, error) {
	if javaBin == "" {
		javaBin = "java"
	}
	cmd := exec.Command(javaBin, "-jar", cliJar, "list-versions", patchesRVP)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("list-versions: %w\n%s", err, stderr.String())
	}
	return ParseListVersions(stdout.String()), nil
}

// ParseListVersions parses revanced-cli list-versions text output.
func ParseListVersions(data string) []Job {
	var jobs []Job
	var cur *Job

	flush := func() {
		if cur != nil && cur.PackageID != "" {
			jobs = append(jobs, *cur)
		}
		cur = nil
	}

	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Package name:") {
			flush()
			id := strings.TrimSpace(strings.TrimPrefix(line, "Package name:"))
			cur = &Job{PackageID: id}
			continue
		}
		if cur == nil {
			// Sometimes format is "Package name: id" split differently
			if strings.Contains(line, "Package name:") {
				flush()
				parts := strings.SplitN(line, "Package name:", 2)
				cur = &Job{PackageID: strings.TrimSpace(parts[1])}
			}
			continue
		}
		if strings.HasPrefix(line, "Most common compatible versions:") {
			continue
		}
		if line == "" || strings.HasPrefix(line, "Package name:") {
			continue
		}
		// version lines often look like: "19.25.37 (recommended)" or "Any"
		ver := strings.Fields(line)
		if len(ver) == 0 {
			continue
		}
		v := ver[0]
		if v == "Any" {
			cur.Versions = append(cur.Versions, "")
		} else if looksLikeVersion(v) {
			cur.Versions = append(cur.Versions, v)
		}
	}
	flush()

	// Deduplicate package ids preserving first block order
	seen := map[string]int{}
	var out []Job
	for _, j := range jobs {
		if i, ok := seen[j.PackageID]; ok {
			// merge versions
			out[i].Versions = appendUnique(out[i].Versions, j.Versions...)
			continue
		}
		seen[j.PackageID] = len(out)
		out = append(out, j)
	}
	return out
}

func looksLikeVersion(s string) bool {
	if s == "" {
		return false
	}
	// reject obvious non-versions
	if strings.HasPrefix(s, "http") {
		return false
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func appendUnique(dst []string, add ...string) []string {
	have := map[string]bool{}
	for _, d := range dst {
		have[d] = true
	}
	for _, a := range add {
		if !have[a] {
			dst = append(dst, a)
			have[a] = true
		}
	}
	return dst
}
