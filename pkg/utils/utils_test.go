package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	assert.EqualValues(t, "short", Truncate("short", 10))
	assert.EqualValues(t, "exactly10!", Truncate("exactly10!", 10))
	assert.EqualValues(t, "Alpine ed…", Truncate("Alpine edge arm64 (20260911_13:02)", 10))
	assert.EqualValues(t, "untouched", Truncate("untouched", 1))
}
