package utils

import (
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/assert"
)

func TestTruncate(t *testing.T) {
	assert.EqualValues(t, "short", Truncate("short", 10))
	assert.EqualValues(t, "exactly10!", Truncate("exactly10!", 10))
	assert.EqualValues(t, "Alpine ed…", Truncate("Alpine edge arm64 (20260911_13:02)", 10))
	assert.EqualValues(t, "untouched", Truncate("untouched", 1))
}

func TestTruncateColored(t *testing.T) {
	green := "\x1b[32m"
	reset := "\x1b[0m"

	t.Run("leaves a line that fits", func(t *testing.T) {
		assert.EqualValues(t, "short", TruncateColored("short", 10))
	})

	t.Run("measures display width, not escapes", func(t *testing.T) {
		// Eight columns of text wearing 9 bytes of escapes either side.
		line := green + "web-1" + reset + " up"
		assert.EqualValues(t, line, TruncateColored(line, 8))
	})

	t.Run("cuts uncoloured text like Truncate", func(t *testing.T) {
		assert.EqualValues(t, "192.0.2.1…", TruncateColored("192.0.2.144", 10))
	})

	t.Run("carries escapes over and closes the colour", func(t *testing.T) {
		assert.EqualValues(t, green+"runn"+"…"+reset, TruncateColored(green+"running"+reset, 5))
	})

	t.Run("never cuts inside an escape sequence", func(t *testing.T) {
		truncated := TruncateColored("ab"+green+"cdef"+reset, 4)
		assert.EqualValues(t, "ab"+green+"c"+"…"+reset, truncated)
		assert.EqualValues(t, 4, runewidth.StringWidth(Decolorise(truncated)))
	})

	t.Run("steps over 256-colour escapes", func(t *testing.T) {
		orange := "\x1b[38;5;208m"
		truncated := TruncateColored(orange+"unhealthy"+reset, 5)
		assert.EqualValues(t, orange+"unhe…"+reset, truncated)
		assert.EqualValues(t, 5, runewidth.StringWidth(Decolorise(truncated)))
	})

	t.Run("leaves a line whose overflow is only padding", func(t *testing.T) {
		line := "web-1 " + green + "" + reset + "     "
		assert.EqualValues(t, line, TruncateColored(line, 6))
	})

	t.Run("a width with no room to mark the cut leaves the line alone", func(t *testing.T) {
		assert.EqualValues(t, "untouched", TruncateColored("untouched", 1))
	})
}

func TestMarshalIntoYamlKeepsFieldOrder(t *testing.T) {
	type inner struct {
		Zulu  int `json:"zulu"`
		Alpha int `json:"alpha"`
	}

	data, err := MarshalIntoYaml(struct {
		Name   string            `json:"name"`
		Nested inner             `json:"nested"`
		Config map[string]string `json:"config"`
	}{
		Name:   "web",
		Nested: inner{Zulu: 1, Alpha: 2},
		Config: map[string]string{"b": "2", "a": "1"},
	})

	assert.NoError(t, err)
	assert.EqualValues(t, "name: web\nnested:\n  zulu: 1\n  alpha: 2\nconfig:\n  a: \"1\"\n  b: \"2\"\n", string(data))
}
