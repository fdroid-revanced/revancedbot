package revanced

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lucasew/revancedbot/internal/drivers"
	"github.com/lucasew/workspaced/pkg/logging"
)

func TestFetchPatches_FromMirror(t *testing.T) {
	if os.Getenv("REVANCEDBOT_SKIP_NETWORK") == "1" {
		t.Skip("network disabled")
	}
	ctx, cancel := context.WithTimeout(logging.NewWriterContext(t.Output()), 3*time.Minute)
	defer cancel()
	dest := filepath.Join(t.TempDir(), "patches.rvp")
	if err := FetchPatches(ctx, os.Getenv("GITHUB_TOKEN"), dest); err != nil {
		t.Fatalf("FetchPatches: %v", err)
	}
	st, err := os.Stat(dest)
	if err != nil || st.Size() < 100_000 {
		t.Fatalf("patches file missing/small: %v size=%v", err, st)
	}
	t.Logf("fetched %d bytes", st.Size())
}
