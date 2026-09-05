package workspace

import (
	"path/filepath"
)

const keystoreRel = "signing/keystore.jks"

// KeystoreFile is the only keystore location under Cache (CACHE/signing/keystore.jks).
func KeystoreFile(cache string) string {
	return filepath.Join(cache, keystoreRel)
}

// Resolve returns an absolute, cleaned path. Existing prefixes are
// symlink-evaluated so a cache or keystore that points into Repo is visible.
func Resolve(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if ev, err := filepath.EvalSymlinks(abs); err == nil {
		return ev, nil
	}
	var tail []string
	cur := abs
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		if ev, err := filepath.EvalSymlinks(parent); err == nil {
			return filepath.Join(append([]string{ev}, tail...)...), nil
		}
		cur = parent
	}
	return abs, nil
}

// Under reports whether path is root or a descendant of root after Resolve.
func Under(root, path string) bool {
	r, err := Resolve(root)
	if err != nil {
		return false
	}
	p, err := Resolve(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(r, p)
	if err != nil {
		return false
	}
	return rel == "." || filepath.IsLocal(rel)
}
