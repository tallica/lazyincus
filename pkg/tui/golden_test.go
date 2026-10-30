package tui

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "rewrite the screen tests' golden files")

// Dates render in local time; the golden screens were taken in UTC.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}

// goldenScreen is m's screen the way the goldens hold it: no trailing
// blanks, a newline at the end.
func goldenScreen(m *Model) string {
	lines := screen(m)
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}

	return strings.Join(lines, "\n") + "\n"
}

// assertGolden compares a screen with testdata/screens/<name>.txt, or
// rewrites that file under -update.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", "screens", name+".txt")

	if *updateGolden {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o600))
		return
	}

	want, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no golden screen at %s - run the test with -update to write it:\n%s", path, got)
	}
	require.NoError(t, err)

	assert.Equal(t, string(want), got, "screen differs from %s", path)
}
