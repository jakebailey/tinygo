package builder

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tinygo-org/tinygo/compileopts"
)

func (l *Library) cachePath(config *compileopts.Config) string {
	path := config.LibraryPath(l.name)
	if len(l.sourcePatch) != 0 {
		path += fmt.Sprintf("-%x", sha256.Sum256(l.sourcePatch))
	}
	return path
}

func prepareLibrarySources(sourceDir, destination string, patch []byte) error {
	if err := os.Mkdir(destination, 0o755); err != nil {
		return err
	}
	err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Name() == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported source file type: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm()|0o200)
	})
	if err != nil {
		return err
	}
	return applyLibraryPatch(destination, patch)
}

var libraryPatchHunk = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@.*$`)

// Only modifications to existing, newline-terminated text files are supported.
func applyLibraryPatch(directory string, patch []byte) error {
	lines := strings.Split(strings.TrimSuffix(string(patch), "\n"), "\n")
	files := make(map[string]bool)
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(lines[i], "--- a/") {
			if strings.HasPrefix(lines[i], "diff --git ") || strings.HasPrefix(lines[i], "index ") {
				i++
				continue
			}
			return fmt.Errorf("unexpected patch line %d: %q", i+1, lines[i])
		}
		name := strings.TrimPrefix(lines[i], "--- a/")
		if !filepath.IsLocal(name) || strings.Contains(name, `\`) || files[name] {
			return fmt.Errorf("invalid or repeated patch path: %q", name)
		}
		files[name] = true
		i++
		if i >= len(lines) || lines[i] != "+++ b/"+name {
			return fmt.Errorf("patch must modify the existing file %s", name)
		}
		i++
		path := filepath.Join(directory, filepath.FromSlash(name))
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.HasSuffix(string(data), "\n") {
			return fmt.Errorf("%s is not newline-terminated", name)
		}
		original := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		var output []string
		cursor := 0
		hunks := 0
		for i < len(lines) && strings.HasPrefix(lines[i], "@@ ") {
			match := libraryPatchHunk.FindStringSubmatch(lines[i])
			if match == nil {
				return fmt.Errorf("invalid hunk for %s: %q", name, lines[i])
			}
			var numbers [4]int
			for n := range numbers {
				text := match[n+1]
				if text == "" {
					numbers[n] = 1
				} else {
					numbers[n], err = strconv.Atoi(text)
					if err != nil {
						return fmt.Errorf("invalid hunk position for %s: %w", name, err)
					}
				}
			}
			start := numbers[0] - 1
			if numbers[1] == 0 {
				start++
			}
			if start < cursor || start > len(original) {
				return fmt.Errorf("hunk outside source or out of order in %s", name)
			}
			output = append(output, original[cursor:start]...)
			newStart := numbers[2] - 1
			if numbers[3] == 0 {
				newStart++
			}
			if newStart != len(output) {
				return fmt.Errorf("incorrect output position in %s", name)
			}
			cursor = start
			i++
			oldCount, newCount := 0, 0
			for oldCount < numbers[1] || newCount < numbers[3] {
				if i >= len(lines) || len(lines[i]) == 0 {
					return fmt.Errorf("incomplete hunk in %s", name)
				}
				line := lines[i]
				switch line[0] {
				case ' ', '-':
					if cursor >= len(original) || original[cursor] != line[1:] {
						return fmt.Errorf("patch context mismatch in %s at line %d", name, cursor+1)
					}
					cursor++
					oldCount++
					if line[0] == ' ' {
						output = append(output, line[1:])
						newCount++
					}
				case '+':
					output = append(output, line[1:])
					newCount++
				default:
					return fmt.Errorf("invalid hunk line in %s: %q", name, line)
				}
				if oldCount > numbers[1] || newCount > numbers[3] {
					return fmt.Errorf("incorrect hunk counts in %s", name)
				}
				i++
			}
			hunks++
		}
		if hunks == 0 {
			return fmt.Errorf("no patch hunks for %s", name)
		}
		output = append(output, original[cursor:]...)
		if err := os.WriteFile(path, []byte(strings.Join(output, "\n")+"\n"), 0o644); err != nil {
			return err
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("source patch contains no files")
	}
	return nil
}
