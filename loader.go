/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package pebble

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"
)

// fileLoader is io.pebbletemplates.pebble.loader.FileLoader: it reads templates from a directory
// of the file system. The reference command always builds one with an absolute prefix and the
// UTF-8 charset, and never sets a suffix.
type fileLoader struct {
	prefix string
}

// newFileLoader is FileLoader's constructor, which runs setPrefix and so rejects a prefix that is
// empty or relative with a LoaderException.
func newFileLoader(prefix string) (*fileLoader, error) {
	// Java checks null first; the Go zero value of a string is the empty one, which trim catches.
	trimmed := javaTrim(prefix)
	if trimmed == "" {
		return nil, NewLoaderError("Prefix cannot be empty", nil)
	}
	if !strings.HasPrefix(trimmed, "/") {
		return nil, NewLoaderError("Prefix must be an absolute path", nil)
	}
	return &fileLoader{prefix: trimmed}, nil
}

// read is getReader: it resolves the template name against the prefix, refuses anything outside
// the prefix and returns the file decoded as UTF-8.
func (l *fileLoader) read(templateName string) (string, error) {
	name := sanitizePath(templateName)
	path := javaPathsGet(l.prefix, name)
	if err := l.checkIfDirectoryTraversal(name); err != nil {
		return "", err
	}
	data, err := readJavaFile(path)
	if err != nil {
		return "", NewLoaderError(fmt.Sprintf("Could not find template [prefix='%s', templateName='%s']",
			l.prefix, templateName), err)
	}
	return decodeUTF8(data), nil
}

// checkIfDirectoryTraversal reproduces FileLoader.checkIfDirectoryTraversal: an absolute template
// name and a name that climbs out of the prefix are both LoaderExceptions.
func (l *fileLoader) checkIfDirectoryTraversal(templateName string) error {
	if strings.HasPrefix(templateName, "/") {
		return NewLoaderError(fmt.Sprintf("templateName '%s' must be relative", templateName), nil)
	}
	// Join the two paths together, then normalize so that any ".." elements in the user path can
	// remove parts of the prefix, as java.nio.file.Path.normalize does.
	resolved := normalizePath(javaPathsGet(l.prefix, templateName))
	if !pathStartsWith(resolved, normalizePath(l.prefix)) {
		return NewLoaderError(fmt.Sprintf(
			"template is not in the base directory path [baseDir='%s', templateName='%s']",
			l.prefix, templateName), nil)
	}
	return nil
}

// resolveRelativePath is PathUtils.resolveRelativePath: a path that starts with "./" or "../" is
// resolved against the directory of the anchor, anything else is left to the loader. The empty
// string stands for Java's null, that is "not resolved".
func (l *fileLoader) resolveRelativePath(relativePath, anchorPath string) string {
	if relativePath == "" {
		return ""
	}
	relativePath = sanitizePath(relativePath)
	anchorPath = sanitizePath(anchorPath)
	if !strings.HasPrefix(relativePath, "../") && !strings.HasPrefix(relativePath, "./") {
		return ""
	}
	// The anchor contributes its directory: its last segment is dropped unless it already ends
	// with a separator.
	var segments []string
	if anchorPath != "" {
		segments = strings.Split(anchorPath, "/")
		if !strings.HasSuffix(anchorPath, "/") && len(segments) > 0 {
			segments = segments[:len(segments)-1]
		}
	}
	for _, segment := range strings.Split(relativePath, "/") {
		switch segment {
		case ".":
			// do nothing
		case "..":
			if len(segments) > 0 {
				segments = segments[:len(segments)-1]
			}
		default:
			segments = append(segments, segment)
		}
	}
	return strings.Join(segments, "/")
}

// sanitizePath is PathUtils.sanitize: both separators become the platform one, which is "/" on
// the platforms the reference command runs on.
func sanitizePath(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

// javaPathsGet is java.nio.file.Paths.get(first, more): the segments are joined with a single
// separator, empty segments disappear, but "." and ".." are kept as they are.
func javaPathsGet(prefix, name string) string {
	parts := make([]string, 0, 8)
	for _, segment := range strings.Split(prefix+"/"+name, "/") {
		if segment != "" {
			parts = append(parts, segment)
		}
	}
	joined := strings.Join(parts, "/")
	if strings.HasPrefix(prefix, "/") {
		return "/" + joined
	}
	return joined
}

// normalizePath is java.nio.file.Path.normalize: "." disappears and ".." removes the previous
// segment; for an absolute path a leading ".." is dropped at the root.
func normalizePath(path string) string {
	absolute := strings.HasPrefix(path, "/")
	var parts []string
	for _, segment := range strings.Split(path, "/") {
		switch segment {
		case "", ".":
			// nothing to add
		case "..":
			if len(parts) > 0 && parts[len(parts)-1] != ".." {
				parts = parts[:len(parts)-1]
			} else if !absolute {
				parts = append(parts, "..")
			}
		default:
			parts = append(parts, segment)
		}
	}
	joined := strings.Join(parts, "/")
	if absolute {
		return "/" + joined
	}
	return joined
}

// pathStartsWith is java.nio.file.Path.startsWith, which compares whole name elements.
func pathStartsWith(path, prefix string) bool {
	if path == prefix {
		return true
	}
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.HasPrefix(path, prefix)
}

// readJavaFile reads a file and reports the failure the way java.io.FileInputStream does, since
// the FileNotFoundException's message reaches the output as the cause of the LoaderException.
func readJavaFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return nil, &JavaError{Kind: "FileNotFoundException", Message: path + " (Is a directory)"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &JavaError{Kind: "FileNotFoundException", Message: path + " (" + errnoText(err) + ")"}
	}
	return data, nil
}

// errnoText is the strerror text the JDK appends to a FileNotFoundException. Go spells the errno
// messages in lower case, the C library the JDK calls capitalises the first letter.
func errnoText(err error) string {
	text := err.Error()
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		text = pathErr.Err.Error()
	}
	if text == "" {
		return text
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// decodeUTF8 decodes template bytes the way an InputStreamReader with the UTF-8 charset does:
// every malformed byte becomes the replacement character.
func decodeUTF8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	var out strings.Builder
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size <= 1 {
			out.WriteRune(utf8.RuneError)
			i++
			continue
		}
		out.WriteRune(r)
		i += size
	}
	return out.String()
}

// javaTrim is String.trim: it removes every character up to and including the space.
func javaTrim(s string) string {
	start, end := 0, len(s)
	for start < end && s[start] <= ' ' {
		start++
	}
	for end > start && s[end-1] <= ' ' {
		end--
	}
	return s[start:end]
}
