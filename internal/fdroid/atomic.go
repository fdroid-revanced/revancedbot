package fdroid

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path via a same-dir temp file + rename.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Publish replaces live REPO publishable paths with the stage tree atomically
// (per entry: write under REPO/.publish-*, rename swap).
// Stage must contain config.yml, repo/, metadata/ after a successful fdroid update.
// revancedbot.yaml in REPO is never touched.
func Publish(stageRoot, liveRepo string) error {
	if err := ValidateStageAfterUpdate(stageRoot); err != nil {
		return fmt.Errorf("publish aborted: %w", err)
	}
	if err := RemovePublishLeftovers(liveRepo); err != nil {
		return err
	}
	if err := os.MkdirAll(liveRepo, 0o755); err != nil {
		return err
	}
	// Files
	if err := publishFile(filepath.Join(stageRoot, "config.yml"), filepath.Join(liveRepo, "config.yml")); err != nil {
		return fmt.Errorf("publish config.yml: %w", err)
	}
	// Directories
	for _, name := range []string{"repo", "metadata"} {
		src := filepath.Join(stageRoot, name)
		dst := filepath.Join(liveRepo, name)
		if err := publishDir(src, dst); err != nil {
			return fmt.Errorf("publish %s: %w", name, err)
		}
	}
	return nil
}

func publishFile(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("%s is a directory", src)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return WriteFileAtomic(dst, data, 0o600)
}

func publishDir(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	parent := filepath.Dir(dst)
	base := filepath.Base(dst)
	tmp := filepath.Join(parent, "."+base+".new")
	old := filepath.Join(parent, "."+base+".old")

	_ = os.RemoveAll(tmp)
	_ = os.RemoveAll(old)

	if err := copyDir(src, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}

	// Swap: dst -> old, tmp -> dst
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			_ = os.RemoveAll(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		// best-effort restore
		_ = os.Rename(old, dst)
		_ = os.RemoveAll(tmp)
		return err
	}
	_ = os.RemoveAll(old)
	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_ = os.Remove(target)
			return os.Symlink(link, target)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
