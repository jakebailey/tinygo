// Portions copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package debug is a very partially implemented package to allow compilation.
package debug

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// SetMaxStack sets the maximum amount of memory that can be used by a single
// goroutine stack.
//
// Not implemented.
func SetMaxStack(n int) int {
	return n
}

// PrintStack prints to standard error the stack trace returned by runtime.Stack.
//
// Not implemented.
func PrintStack() {}

// Stack returns a formatted stack trace of the goroutine that calls it.
//
// Not implemented.
func Stack() []byte {
	return nil
}

// ReadBuildInfo returns the build information embedded
// in the running binary. The information is available only
// in binaries built with module support.
//
// Not implemented.
func ReadBuildInfo() (info *BuildInfo, ok bool) {
	return &BuildInfo{GoVersion: runtime.Compiler + runtime.Version()}, true
}

// BuildInfo represents the build information read from
// the running binary.
type BuildInfo struct {
	GoVersion string    // version of the Go toolchain that built the binary, e.g. "go1.19.2"
	Path      string    // The main package path
	Main      Module    // The module containing the main package
	Deps      []*Module // Module dependencies
	Settings  []BuildSetting
}

type BuildSetting struct {
	// Key and Value describe the build setting.
	// Key must not contain an equals sign, space, tab, or newline.
	// Value must not contain newlines ('\n').
	Key, Value string
}

// Module represents a module.
type Module struct {
	Path    string  // module path
	Version string  // module version
	Sum     string  // checksum
	Replace *Module // replaced by this module
}

// Not implemented.
func SetGCPercent(n int) int {
	return n
}

// Start of stolen from big go. TODO: import/reuse without copy pasta.

// quoteKey reports whether key is required to be quoted.
func quoteKey(key string) bool {
	return len(key) == 0 || strings.ContainsAny(key, "= \t\r\n\"`")
}

// quoteValue reports whether value is required to be quoted.
func quoteValue(value string) bool {
	return strings.ContainsAny(value, " \t\r\n\"`")
}

func (bi *BuildInfo) String() string {
	buf := new(strings.Builder)
	if bi.GoVersion != "" {
		fmt.Fprintf(buf, "go\t%s\n", bi.GoVersion)
	}
	if bi.Path != "" {
		fmt.Fprintf(buf, "path\t%s\n", bi.Path)
	}
	var formatMod func(string, Module)
	formatMod = func(word string, m Module) {
		buf.WriteString(word)
		buf.WriteByte('\t')
		buf.WriteString(m.Path)
		buf.WriteByte('\t')
		buf.WriteString(m.Version)
		if m.Replace == nil {
			buf.WriteByte('\t')
			buf.WriteString(m.Sum)
		} else {
			buf.WriteByte('\n')
			formatMod("=>", *m.Replace)
		}
		buf.WriteByte('\n')
	}
	if bi.Main != (Module{}) {
		formatMod("mod", bi.Main)
	}
	for _, dep := range bi.Deps {
		formatMod("dep", *dep)
	}
	for _, s := range bi.Settings {
		key := s.Key
		if quoteKey(key) {
			key = strconv.Quote(key)
		}
		value := s.Value
		if quoteValue(value) {
			value = strconv.Quote(value)
		}
		fmt.Fprintf(buf, "build\t%s=%s\n", key, value)
	}

	return buf.String()
}

// ParseBuildInfo parses the string returned by [*BuildInfo.String],
// restoring the original [BuildInfo],
// except that the GoVersion field is not set.
// Programs should normally not call this function,
// but instead call [ReadBuildInfo], [debug/buildinfo.ReadFile],
// or [debug/buildinfo.Read].
func ParseBuildInfo(data string) (bi *BuildInfo, err error) {
	lineNum := 1
	defer func() {
		if err != nil {
			err = fmt.Errorf("could not parse Go build info: line %d: %w", lineNum, err)
		}
	}()

	const (
		pathLine  = "path\t"
		modLine   = "mod\t"
		depLine   = "dep\t"
		repLine   = "=>\t"
		buildLine = "build\t"
		newline   = "\n"
		tab       = "\t"
	)

	readModuleLine := func(elem []string) (Module, error) {
		if len(elem) != 2 && len(elem) != 3 {
			return Module{}, fmt.Errorf("expected 2 or 3 columns; got %d", len(elem))
		}
		version := elem[1]
		sum := ""
		if len(elem) == 3 {
			sum = elem[2]
		}
		return Module{
			Path:    elem[0],
			Version: version,
			Sum:     sum,
		}, nil
	}

	bi = new(BuildInfo)
	var (
		last *Module
		line string
		ok   bool
	)
	// Reverse of BuildInfo.String(), except for go version.
	for len(data) > 0 {
		line, data, ok = strings.Cut(data, newline)
		if !ok {
			break
		}
		switch {
		case strings.HasPrefix(line, pathLine):
			elem := line[len(pathLine):]
			bi.Path = elem
		case strings.HasPrefix(line, modLine):
			elem := strings.Split(line[len(modLine):], tab)
			last = &bi.Main
			*last, err = readModuleLine(elem)
			if err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, depLine):
			elem := strings.Split(line[len(depLine):], tab)
			last = new(Module)
			bi.Deps = append(bi.Deps, last)
			*last, err = readModuleLine(elem)
			if err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, repLine):
			elem := strings.Split(line[len(repLine):], tab)
			if len(elem) != 3 {
				return nil, fmt.Errorf("expected 3 columns for replacement; got %d", len(elem))
			}
			if last == nil {
				return nil, fmt.Errorf("replacement with no module on previous line")
			}
			last.Replace = &Module{
				Path:    elem[0],
				Version: elem[1],
				Sum:     elem[2],
			}
			last = nil
		case strings.HasPrefix(line, buildLine):
			kv := line[len(buildLine):]
			if len(kv) < 1 {
				return nil, fmt.Errorf("build line missing '='")
			}

			var key, rawValue string
			switch kv[0] {
			case '=':
				return nil, fmt.Errorf("build line with missing key")

			case '`', '"':
				rawKey, err := strconv.QuotedPrefix(kv)
				if err != nil {
					return nil, fmt.Errorf("invalid quoted key in build line")
				}
				if len(kv) == len(rawKey) {
					return nil, fmt.Errorf("build line missing '=' after quoted key")
				}
				if c := kv[len(rawKey)]; c != '=' {
					return nil, fmt.Errorf("unexpected character after quoted key: %q", c)
				}
				key, _ = strconv.Unquote(rawKey)
				rawValue = kv[len(rawKey)+1:]

			default:
				var ok bool
				key, rawValue, ok = strings.Cut(kv, "=")
				if !ok {
					return nil, fmt.Errorf("build line missing '=' after key")
				}
				if quoteKey(key) {
					return nil, fmt.Errorf("unquoted key %q must be quoted", key)
				}
			}

			var value string
			if len(rawValue) > 0 {
				switch rawValue[0] {
				case '`', '"':
					var err error
					value, err = strconv.Unquote(rawValue)
					if err != nil {
						return nil, fmt.Errorf("invalid quoted value in build line")
					}

				default:
					value = rawValue
					if quoteValue(value) {
						return nil, fmt.Errorf("unquoted value %q must be quoted", value)
					}
				}
			}

			bi.Settings = append(bi.Settings, BuildSetting{Key: key, Value: value})
		}
		lineNum++
	}
	return bi, nil
}
