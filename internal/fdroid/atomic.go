package fdroid

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lucasew/revancedbot/internal/osx"
)

// livePublishNames is the Repo triple swapped as one unit (INV-01).
var livePublishNames = []string{"config.yml", "repo", "metadata"}

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
		osx.Remove(tmp)
		return err
	}
	return nil
}

// PublishArgs is the stage→live swap. LayoutOnly skips the index-artifact gate (fdroid-init).
type PublishArgs struct {
	Stage      string
	Live       string
	LayoutOnly bool
}

// Publish replaces live REPO {config.yml,repo,metadata} from stage as one unit.
// Stage must contain those paths. LayoutOnly skips the index-artifact gate;
// run / smoke / fdroid-update leave it false.
// revancedbot.yaml in REPO is never touched.
func Publish(args PublishArgs) error {
	check := ValidateStageAfterUpdate
	if args.LayoutOnly {
		check = ValidateStageLayout
	}
	if err := check(args.Stage); err != nil {
		return fmt.Errorf("publish aborted: %w", err)
	}
	if err := RemovePublishLeftovers(args.Live); err != nil {
		return err
	}
	if err := os.MkdirAll(args.Live, 0o755); err != nil {
		return err
	}

	for _, name := range livePublishNames {
		if err := stagePublishEntry(filepath.Join(args.Stage, name), publishTmp(args.Live, name)); err != nil {
			removePublishTemps(args.Live)
			return fmt.Errorf("publish %s: %w", name, err)
		}
	}

	var swapped []string
	for _, name := range livePublishNames {
		if err := swapPublishEntry(args.Live, name); err != nil {
			rollbackPublish(args.Live, swapped)
			removePublishTemps(args.Live)
			return fmt.Errorf("publish %s: %w", name, err)
		}
		swapped = append(swapped, name)
	}
	for _, name := range livePublishNames {
		osx.RemoveAll(publishOld(args.Live, name))
	}
	return nil
}

func publishTmp(root, name string) string {
	return filepath.Join(root, "."+name+".new")
}

func publishOld(root, name string) string {
	return filepath.Join(root, "."+name+".old")
}

func stagePublishEntry(src, tmp string) error {
	osx.RemoveAll(tmp)
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return copyDir(src, tmp)
	}
	return copyFile(src, tmp, 0o600)
}

func swapPublishEntry(root, name string) error {
	dst := filepath.Join(root, name)
	tmp := publishTmp(root, name)
	old := publishOld(root, name)
	osx.RemoveAll(old)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		osx.Rename(old, dst)
		osx.RemoveAll(tmp)
		return err
	}
	return nil
}

func rollbackPublish(root string, names []string) {
	for i := len(names) - 1; i >= 0; i-- {
		name := names[i]
		old := publishOld(root, name)
		if _, err := os.Stat(old); err != nil {
			continue
		}
		dst := filepath.Join(root, name)
		osx.RemoveAll(dst)
		osx.Rename(old, dst)
	}
}

func removePublishTemps(root string) {
	for _, name := range livePublishNames {
		osx.RemoveAll(publishTmp(root, name))
	}
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
			osx.Remove(target)
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
