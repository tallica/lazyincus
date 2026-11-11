package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/google/go-containerregistry/pkg/name"
)

// ComposeTagBump moves a service's image to the newer tag beacon found.
type ComposeTagBump struct {
	Service string
	// From is the image as the compose file names it, To with the new tag.
	From, To string
}

func (b ComposeTagBump) Tag() string {
	return b.To[strings.LastIndex(b.To, ":")+1:]
}

// TagBump is false unless beacon tracked the very image and tag the file names.
func (s *ComposeService) TagBump() (ComposeTagBump, bool) {
	verdict := s.Beacon()

	tag := verdict.NewerTag()
	if !validTag.MatchString(tag) || strings.Contains(s.Image, "$") {
		return ComposeTagBump{}, false
	}

	declared, err := name.NewTag(s.Image, name.WeakValidation)
	if err != nil || !strings.HasSuffix(s.Image, ":"+declared.TagStr()) {
		return ComposeTagBump{}, false
	}

	tracked, err := name.NewTag(s.trackedRef(), name.WeakValidation)
	if err != nil || tracked.Context().Name() != declared.Context().Name() || tracked.TagStr() != declared.TagStr() {
		return ComposeTagBump{}, false
	}

	return ComposeTagBump{
		Service: s.Name,
		From:    s.Image,
		To:      strings.TrimSuffix(s.Image, declared.TagStr()) + tag,
	}, true
}

// trackedRef is the image beacon checked, which its image key can set apart.
func (s *ComposeService) trackedRef() string {
	for _, instance := range s.SortedInstances() {
		if instance.Beacon().NewerTag() != "" {
			return instance.config(beaconCurrentKey)
		}
	}

	return ""
}

// validTag is the registry's tag grammar: beacon's key is anyone's to set,
// and what it says goes into the compose file.
var validTag = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// errImageNotInFile: an image from an override file or an extends.
var errImageNotInFile = errors.New("no image line of its own")

// RetagComposeImages edits only the image values, and returns the bumps it made.
func RetagComposeImages(file string, bumps []ComposeTagBump) ([]ComposeTagBump, error) {
	info, err := os.Stat(file)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}

	lines := strings.SplitAfter(string(data), "\n")

	var (
		made   []ComposeTagBump
		failed []error
	)

	for _, bump := range bumps {
		if err := retagLine(data, lines, bump); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", bump.Service, err))
			continue
		}

		made = append(made, bump)
	}

	if len(made) > 0 {
		if err := replaceFile(file, []byte(strings.Join(lines, "")), info.Mode().Perm()); err != nil {
			return nil, err
		}
	}

	return made, errors.Join(failed...)
}

// replaceFile swaps data in whole, so the stack poller or incus-compose
// never reads a half-written compose file; a symlink keeps pointing at it.
func replaceFile(file string, data []byte, perm os.FileMode) error {
	file, err := filepath.EvalSymlinks(file)
	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(file), "."+filepath.Base(file)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}

	if err := temp.Close(); err != nil {
		return err
	}

	if err := os.Chmod(temp.Name(), perm); err != nil {
		return err
	}

	return os.Rename(temp.Name(), file)
}

func retagLine(data []byte, lines []string, bump ComposeTagBump) error {
	node, err := serviceImageNode(data, bump.Service)
	if err != nil {
		return err
	}

	if node.Value != bump.From {
		return fmt.Errorf("the file names %s, not %s", node.Value, bump.From)
	}

	line := node.Token.Position.Line - 1
	if line < 0 || line >= len(lines) {
		return errImageNotInFile
	}

	// After the key's colon, so a key that spells the value isn't taken.
	text := lines[line]
	colon := strings.Index(text, ":")
	at := strings.Index(text[colon+1:], bump.From)
	if colon < 0 || at < 0 {
		return errImageNotInFile
	}

	at += colon + 1
	lines[line] = text[:at] + bump.To + text[at+len(bump.From):]

	return nil
}

// serviceImageNode is services.<service>.image in the file's first document.
func serviceImageNode(data []byte, service string) (*ast.StringNode, error) {
	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		return nil, err
	}

	if len(file.Docs) == 0 {
		return nil, errImageNotInFile
	}

	node := file.Docs[0].Body
	for _, key := range []string{"services", service, "image"} {
		if node = mappingValue(node, key); node == nil {
			return nil, errImageNotInFile
		}
	}

	image, ok := node.(*ast.StringNode)
	if !ok {
		return nil, errImageNotInFile
	}

	return image, nil
}

func mappingValue(node ast.Node, key string) ast.Node {
	var values []*ast.MappingValueNode

	switch mapping := node.(type) {
	case *ast.MappingNode:
		values = mapping.Values
	case *ast.MappingValueNode:
		values = []*ast.MappingValueNode{mapping}
	}

	for _, value := range values {
		if value.Key != nil && value.Key.GetToken() != nil && value.Key.GetToken().Value == key {
			return value.Value
		}
	}

	return nil
}
