package fdroid

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"uuid"

	ioatomic "github.com/lewtec/lewkit/x/io/atomic"

	"github.com/lucasew/revancedbot/internal/osx"
)

// livePublishNames is the Repo triple published as one unit (INV-01).
var livePublishNames = []string{"config.yml", "repo", "metadata"}

// WriteFileAtomic writes data to path via lewkit WriteFileFunction.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := ioatomic.WriteFileFunction(path, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	}); err != nil {
		return err
	}
	return os.Chmod(path, perm)
}

// PublishArgs is the stage→live publish. LayoutOnly skips the index-artifact gate (fdroid-init).
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
		if err := publishEntry(filepath.Join(args.Stage, name), filepath.Join(args.Live, name)); err != nil {
			return fmt.Errorf("publish %s: %w", name, err)
		}
	}
	return RemovePublishLeftovers(args.Live)
}

func publishEntry(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return replaceDir(src, dst)
	}
	return copyFile(src, dst, 0o600)
}

func replaceDir(src, dst string) error {
	if err := copyDir(src, dst); err != nil {
		return err
	}
	keep := map[string]struct{}{}
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel != "." {
			keep[rel] = struct{}{}
		}
		return nil
	}); err != nil {
		return err
	}
	return filepath.Walk(dst, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dst, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if _, ok := keep[rel]; ok {
			return nil
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}

func isPublishTempName(name string) bool {
	if strings.HasPrefix(name, ".") && (strings.HasSuffix(name, ".new") || strings.HasSuffix(name, ".old")) {
		return true
	}
	for _, pub := range livePublishNames {
		rest, ok := strings.CutPrefix(name, pub+".")
		if !ok {
			continue
		}
		if _, err := uuid.Parse(rest); err == nil {
			return true
		}
	}
	return false
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
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := ioatomic.WriteFileFunction(dst, func(w io.Writer) error {
		_, err := io.Copy(w, in)
		return err
	}); err != nil {
		return err
	}
	return os.Chmod(dst, perm)
}
