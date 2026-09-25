package utils

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-errors/errors"
	"github.com/rivo/uniseg"

	"github.com/fatih/color"
	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/printer"
)

// DisplayWidth is how many terminal columns str takes, colour escapes
// taking none. Every width lazyincus lays out goes through here, measured
// by grapheme cluster with uniseg the way gocui draws - tcell sets
// uniseg's ambiguous width for an East Asian locale for both of us.
func DisplayWidth(str string) int {
	return uniseg.StringWidth(Decolorise(str))
}

// WithPadding pads a string as much as you want
func WithPadding(str string, padding int) string {
	width := DisplayWidth(str)
	if padding < width {
		return str
	}
	return str + strings.Repeat(" ", padding-width)
}

// ColoredString takes a string and a colour attribute and returns a colored
// string with that attribute
func ColoredString(str string, colorAttribute color.Attribute) string {
	if colorAttribute == color.FgWhite {
		return str
	}
	colour := color.New(colorAttribute)
	return ColoredStringDirect(str, colour)
}

// ColoredYamlString takes an YAML formatted string and returns a colored string
// with colors hardcoded as:
// keys: cyan
// Booleans: magenta
// Numbers: yellow
// Strings: green
func ColoredYamlString(str string) string {
	format := func(attr color.Attribute) string {
		return fmt.Sprintf("%s[%dm", "\x1b", attr)
	}
	tokens := lexer.Tokenize(str)
	var p printer.Printer
	p.Bool = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgMagenta),
			Suffix: format(color.Reset),
		}
	}
	p.Number = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgYellow),
			Suffix: format(color.Reset),
		}
	}
	p.MapKey = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgCyan),
			Suffix: format(color.Reset),
		}
	}
	p.String = func() *printer.Property {
		return &printer.Property{
			Prefix: format(color.FgGreen),
			Suffix: format(color.Reset),
		}
	}
	return p.PrintTokens(tokens)
}

// ColoredStringDirect used for aggregating a few color attributes rather than
// just sending a single one
func ColoredStringDirect(str string, colour *color.Color) string {
	return colour.SprintFunc()(fmt.Sprint(str))
}

// NormalizeLinefeeds - Removes all Windows and Mac style line feeds
func NormalizeLinefeeds(str string) string {
	str = strings.ReplaceAll(str, "\r\n", "\n")
	str = strings.ReplaceAll(str, "\r", "")
	return str
}

// Loader dumps a string to be displayed as a loader
func Loader() string {
	characters := "|/-\\"
	now := time.Now()
	nanos := now.UnixNano()
	index := nanos / 50000000 % int64(len(characters))
	return characters[index : index+1]
}

// ResolvePlaceholderString populates a template with values
func ResolvePlaceholderString(str string, arguments map[string]string) string {
	for key, value := range arguments {
		str = strings.ReplaceAll(str, "{{"+key+"}}", value)
	}
	return str
}

// Max returns the maximum of two integers
func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// Min returns the minimum of two integers
func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// RenderTable takes an array of string arrays and returns a table containing the values
func RenderTable(rows [][]string) (string, error) {
	if len(rows) == 0 {
		return "", nil
	}
	if !displayArraysAligned(rows) {
		return "", errors.New("Each item must return the same number of strings to display")
	}

	columnPadWidths := getPadWidths(rows)
	paddedDisplayRows := getPaddedDisplayStrings(rows, columnPadWidths)

	return strings.Join(paddedDisplayRows, "\n"), nil
}

const ellipsis = "…"

// colorEscapePattern matches one SGR sequence, 256-colour and truecolor forms
// included.
var colorEscapePattern = regexp.MustCompile(`\x1B\[[0-9;]*[mK]`)

// Decolorise strips a string of color
func Decolorise(str string) string {
	return colorEscapePattern.ReplaceAllString(str, "")
}

// Truncate shortens a string to the given display width, marking the cut
// with an ellipsis. It steps over colour escapes rather than counting them -
// a width count measures a sequence as though it were text, so a coloured
// line that fits would otherwise be cut, and the cut could land inside a
// sequence - and over grapheme clusters rather than runes, so it can't
// split an accent from its letter or an emoji sequence apart. A string whose overflow is only blanks - a table row's padding
// - hides nothing and is left alone, as is one with no room to mark a cut.
// A reset closes a cut line, the escapes that would have done it being
// past the cut.
func Truncate(str string, width int) string {
	// Ambiguous width: two columns under an East Asian locale.
	ellipsisWidth := DisplayWidth(ellipsis)

	visibleWidth := DisplayWidth(strings.TrimRight(Decolorise(str), " "))
	if width <= ellipsisWidth || visibleWidth <= width {
		return str
	}

	var kept strings.Builder

	limit := width - ellipsisWidth
	used := 0

	// Reports whether the limit was reached, which is where the line ends.
	writeText := func(text string) bool {
		state := -1

		for text != "" {
			var cluster string
			var clusterWidth int

			cluster, text, clusterWidth, state = uniseg.FirstGraphemeClusterInString(text, state)
			if used+clusterWidth > limit {
				return true
			}

			kept.WriteString(cluster)
			used += clusterWidth
		}

		return false
	}

	cursor := 0
	full := false
	colored := false

	for _, escape := range colorEscapePattern.FindAllStringIndex(str, -1) {
		if full = writeText(str[cursor:escape[0]]); full {
			break
		}

		kept.WriteString(str[escape[0]:escape[1]])
		colored = true
		cursor = escape[1]
	}

	if !full {
		writeText(str[cursor:])
	}

	kept.WriteString(ellipsis)

	if colored {
		kept.WriteString("\x1b[0m")
	}

	return kept.String()
}

func getPadWidths(rows [][]string) []int {
	if len(rows[0]) <= 1 {
		return []int{}
	}
	columnPadWidths := make([]int, len(rows[0])-1)
	for i := range columnPadWidths {
		for _, cells := range rows {
			if width := DisplayWidth(cells[i]); width > columnPadWidths[i] {
				columnPadWidths[i] = width
			}
		}
	}
	return columnPadWidths
}

func getPaddedDisplayStrings(rows [][]string, columnPadWidths []int) []string {
	paddedDisplayRows := make([]string, len(rows))
	for i, cells := range rows {
		for j, columnPadWidth := range columnPadWidths {
			paddedDisplayRows[i] += WithPadding(cells[j], columnPadWidth) + " "
		}
		paddedDisplayRows[i] += cells[len(columnPadWidths)]
	}
	return paddedDisplayRows
}

// displayArraysAligned returns true if every string array returned from our
// list of displayables has the same length
func displayArraysAligned(stringArrays [][]string) bool {
	for _, strings := range stringArrays {
		if len(strings) != len(stringArrays[0]) {
			return false
		}
	}
	return true
}

func SafeTruncate(str string, limit int) string {
	if len(str) > limit {
		return str[0:limit]
	} else {
		return str
	}
}

func IsValidHexValue(v string) bool {
	if len(v) != 4 && len(v) != 7 {
		return false
	}

	if v[0] != '#' {
		return false
	}

	for _, char := range v[1:] {
		switch char {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f', 'A', 'B', 'C', 'D', 'E', 'F':
			continue
		default:
			return false
		}
	}

	return true
}

// Style used on menu items that open another menu
func OpensMenuStyle(str string) string {
	return ColoredString(fmt.Sprintf("%s...", str), color.FgMagenta)
}

// MarshalIntoYaml renders json-tagged data - the Incus API's structs have
// no yaml tags - as YAML, in the order the struct declares its fields.
func MarshalIntoYaml(data any) ([]byte, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return yaml.JSONToYAML(dataJSON)
}
