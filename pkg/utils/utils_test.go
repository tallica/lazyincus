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

func TestTruncateMarksCuts(t *testing.T) {
	green := "\x1b[32m"
	reset := "\x1b[0m"

	t.Run("leaves a line that fits", func(t *testing.T) {
		assert.EqualValues(t, "short", Truncate("short", 10))
	})

	t.Run("measures display width, not escapes", func(t *testing.T) {
		// Eight columns of text wearing 9 bytes of escapes either side.
		line := green + "web-1" + reset + " up"
		assert.EqualValues(t, line, Truncate(line, 8))
	})

	t.Run("cuts uncoloured text like Truncate", func(t *testing.T) {
		assert.EqualValues(t, "192.0.2.1…", Truncate("192.0.2.144", 10))
	})

	t.Run("carries escapes over and closes the colour", func(t *testing.T) {
		assert.EqualValues(t, green+"runn"+"…"+reset, Truncate(green+"running"+reset, 5))
	})

	t.Run("never cuts inside an escape sequence", func(t *testing.T) {
		truncated := Truncate("ab"+green+"cdef"+reset, 4)
		assert.EqualValues(t, "ab"+green+"c"+"…"+reset, truncated)
		assert.EqualValues(t, 4, DisplayWidth(truncated))
	})

	t.Run("steps over 256-colour escapes", func(t *testing.T) {
		orange := "\x1b[38;5;208m"
		truncated := Truncate(orange+"unhealthy"+reset, 5)
		assert.EqualValues(t, orange+"unhe…"+reset, truncated)
		assert.EqualValues(t, 5, DisplayWidth(truncated))
	})

	t.Run("leaves a line whose overflow is only padding", func(t *testing.T) {
		line := "web-1 " + green + "" + reset + "     "
		assert.EqualValues(t, line, Truncate(line, 6))
	})

	t.Run("a width with no room to mark the cut leaves the line alone", func(t *testing.T) {
		assert.EqualValues(t, "untouched", Truncate("untouched", 1))
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

func TestDisplayWidthCountsColumnsNotRunes(t *testing.T) {
	assert.Equal(t, 4, DisplayWidth("日本"))
	assert.Equal(t, 1, DisplayWidth("é"))
	assert.Equal(t, 2, DisplayWidth("👨‍👩‍👧"))
	assert.Equal(t, 3, DisplayWidth("\x1b[32mrun\x1b[0m"))
}

func TestTruncateKeepsGraphemeClustersWhole(t *testing.T) {
	assert.Equal(t, "日本…", Truncate("日本語テキスト", 5))
	assert.Equal(t, "café…", Truncate("café au lait", 5))
	assert.Equal(t, "👨‍👩‍👧…", Truncate("👨‍👩‍👧 family", 3))
}

func TestPaddingLinesUpWideCells(t *testing.T) {
	table, err := RenderTable([][]string{{"日本", "a"}, {"web", "b"}})

	assert.NoError(t, err)
	assert.Equal(t, "日本 a\nweb  b", table)
}
