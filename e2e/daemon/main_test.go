// Package daemon checks, against a real daemon, the facts docs/Incus.md
// states about it. A failure on a new Incus release means a doc, and
// probably the code that relies on it, needs changing.
package daemon

import (
	"os"
	"testing"

	"github.com/tallica/lazyincus/e2e/internal/harness"
)

func TestMain(m *testing.M) {
	os.Exit(harness.Main(m))
}
