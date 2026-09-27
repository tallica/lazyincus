package utils

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"slices"
	"strconv"
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

// FlexColumn is a column that gives up width for a table to fit, down to
// MinWidth.
type FlexColumn struct {
	Index    int
	MinWidth int
}

// RenderTableToWidth is RenderTable with columns that give up width for the
// table to fit in width: each flex column in turn, down to its MinWidth,
// truncated with an ellipsis. Short of that the rows run past width, for the
// view to clip. A flex column the rows don't have is skipped.
func RenderTableToWidth(rows [][]string, width int, flex []FlexColumn) (string, error) {
	if len(rows) == 0 || len(flex) == 0 {
		return RenderTable(rows)
	}

	if !displayArraysAligned(rows) {
		return "", errors.New("Each item must return the same number of strings to display")
	}

	widths := make([]int, len(rows[0]))
	for _, cells := range rows {
		for i, cell := range cells {
			widths[i] = max(widths[i], DisplayWidth(cell))
		}
	}

	excess := len(widths) - 1 - width
	for _, columnWidth := range widths {
		excess += columnWidth
	}

	shrunk := []int{}

	for _, column := range flex {
		if excess <= 0 {
			break
		}

		if column.Index < 0 || column.Index >= len(widths) || widths[column.Index] <= column.MinWidth {
			continue
		}

		narrowed := max(column.MinWidth, widths[column.Index]-excess)
		excess -= widths[column.Index] - narrowed
		widths[column.Index] = narrowed
		shrunk = append(shrunk, column.Index)
	}

	if len(shrunk) > 0 {
		rows = slices.Clone(rows)
		for i, cells := range rows {
			rows[i] = slices.Clone(cells)
			for _, index := range shrunk {
				rows[i][index] = Truncate(cells[index], widths[index])
			}
		}
	}

	return strings.Join(getPaddedDisplayStrings(rows, widths[:len(widths)-1]), "\n"), nil
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

// escapePattern matches one escape sequence: CSI, OSC, or a two-byte ESC
// sequence. sgrPattern is the CSI that sets colours, clearPattern the ones
// that clear the screen or reset the terminal.
var (
	escapePattern = regexp.MustCompile(`\x1B\[[0-?]*[ -/]*[@-~]|\x1B\][^\x07\x1B]*(?:\x07|\x1B\\)|\x1B[ -~]`)
	sgrPattern    = regexp.MustCompile(`^\x1B\[([0-9;]*)m$`)
	clearPattern  = regexp.MustCompile(`^\x1B(?:\[[0-9;]*J|c)$`)
)

// ConsoleText makes a serial console's output fit to draw in a panel: only
// its colours survive, less backgrounds and black or white text, and a
// screen clear becomes a line break (see docs/Incus.md, "Logs").
func ConsoleText(str string) string {
	var out strings.Builder

	last := 0
	for _, loc := range escapePattern.FindAllStringIndex(str, -1) {
		out.WriteString(str[last:loc[0]])
		last = loc[1]

		sequence := str[loc[0]:loc[1]]
		if sgr := sgrPattern.FindStringSubmatch(sequence); sgr != nil {
			out.WriteString(foregroundOnly(sgr[1]))
		} else if clearPattern.MatchString(sequence) && !endsLine(out.String()) {
			out.WriteString("\n")
		}
	}

	out.WriteString(str[last:])

	return out.String()
}

// endsLine is whether text ends at the start of a line, the escapes and
// carriage returns after its last newline aside.
func endsLine(text string) bool {
	tail := text[strings.LastIndexByte(text, '\n')+1:]

	return strings.Trim(Decolorise(tail), "\r") == ""
}

// foregroundOnly rewrites an SGR's parameters without its backgrounds and
// black or white foregrounds, or drops it when nothing is left.
func foregroundOnly(params string) string {
	fields := strings.Split(params, ";")
	kept := make([]string, 0, len(fields))

	for i := 0; i < len(fields); i++ {
		field := fields[i]
		switch field {
		case "30", "37", "40", "41", "42", "43", "44", "45", "46", "47":
			continue
		case "38", "48":
			// 38;5;n and 38;2;r;g;b carry their colour in the fields after.
			width := 0
			if i+1 < len(fields) {
				switch fields[i+1] {
				case "5":
					width = 2
				case "2":
					width = 4
				}
			}
			end := min(i+1+width, len(fields))
			if field == "38" {
				kept = append(kept, fields[i:end]...)
			}
			i = end - 1
			continue
		}

		if len(field) == 3 && strings.HasPrefix(field, "10") {
			continue // 100-107, bright backgrounds
		}

		kept = append(kept, field)
	}

	if len(kept) == 0 {
		return ""
	}

	return "\x1B[" + strings.Join(kept, ";") + "m"
}

// Fingerprint is a short digest of the values' JSON, for telling whether an
// item a refresh brought is the one a tab was drawn from.
func Fingerprint(values ...any) string {
	hash := fnv.New64a()

	for _, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return ""
		}

		_, _ = hash.Write(encoded)
	}

	return strconv.FormatUint(hash.Sum64(), 36)
}
