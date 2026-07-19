package fdroid

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Required layout for a simple-binary F-Droid root (stage or live).
// Happy path only — anything else is an error (caller may regen empty stage).

// ValidateLayout checks repo/ + metadata/ directories exist under root.
func ValidateLayout(root string) error {
	for _, name := range []string{"repo", "metadata"} {
		p := filepath.Join(root, name)
		st, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf("repo structure: missing %s: %w", name, err)
		}
		if !st.IsDir() {
			return fmt.Errorf("repo structure: %s is not a directory", name)
		}
	}
	return nil
}

// ValidateJSONTree walks root and ensures every *.json file is valid JSON.
// Returns the first path that fails.
func ValidateJSONTree(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Leftover atomic publish dirs are not happy path.
			base := info.Name()
			if strings.HasPrefix(base, ".") && (strings.HasSuffix(base, ".new") || strings.HasSuffix(base, ".old")) {
				return fmt.Errorf("repo structure: leftover publish dir %s", path)
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("repo structure: read %s: %w", path, err)
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			return fmt.Errorf("repo structure: empty JSON %s", path)
		}
		if !json.Valid(data) {
			return fmt.Errorf("repo structure: invalid JSON %s", path)
		}
		return nil
	})
}

// SanitizeJSONTree removes invalid/empty *.json files under root so fdroid can regen.
// Returns count removed.
func SanitizeJSONTree(root string) (int, error) {
	n := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(string(data))) == 0 || !json.Valid(data) {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove corrupt JSON %s: %w", path, err)
			}
			n++
		}
		return nil
	})
	return n, err
}

// RemovePublishLeftovers deletes .repo.new / .repo.old style dirs under live root.
func RemovePublishLeftovers(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") && (strings.HasSuffix(name, ".new") || strings.HasSuffix(name, ".old")) {
			if err := os.RemoveAll(filepath.Join(root, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateStageAfterUpdate checks the stage after a successful `fdroid update`.
// Aborts outside happy path: layout, config.yml, valid indexes, all JSON valid.
func ValidateStageAfterUpdate(stageRoot string) error {
	if err := ValidateLayout(stageRoot); err != nil {
		return err
	}
	cfg := filepath.Join(stageRoot, "config.yml")
	if st, err := os.Stat(cfg); err != nil || st.IsDir() {
		return fmt.Errorf("repo structure: stage missing config.yml")
	}
	repoDir := filepath.Join(stageRoot, "repo")
	// Need at least one F-Droid index artifact.
	hasIndex := false
	for _, name := range []string{"index-v1.json", "index-v2.json", "index.xml", "entry.json"} {
		p := filepath.Join(repoDir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Size() > 0 {
			hasIndex = true
			break
		}
	}
	if !hasIndex {
		return fmt.Errorf("repo structure: stage/repo missing index artifacts after fdroid update")
	}
	if err := ValidateJSONTree(repoDir); err != nil {
		return err
	}
	// status/ if present must be valid JSON too (covered by tree walk if under repo/).
	return nil
}

// ValidateLiveForSeed checks live REPO before seeding. Missing repo/metadata is OK (empty seed).
// Corrupt JSON or leftover publish dirs is not OK (caller should regen without seeding).
func ValidateLiveForSeed(liveRepo string) error {
	if err := RemovePublishLeftovers(liveRepo); err != nil {
		return err
	}
	// If neither repo nor metadata exist, empty is fine.
	repoDir := filepath.Join(liveRepo, "repo")
	metaDir := filepath.Join(liveRepo, "metadata")
	_, errR := os.Stat(repoDir)
	_, errM := os.Stat(metaDir)
	if os.IsNotExist(errR) && os.IsNotExist(errM) {
		return nil
	}
	if err := ValidateLayout(liveRepo); err != nil {
		return err
	}
	if errR == nil {
		if err := ValidateJSONTree(repoDir); err != nil {
			return err
		}
	}
	return nil
}
