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
	"os"
	"path/filepath"
	"testing"
)

// The expected texts of these tests come from the reference command.

func TestResolveRelativePath(t *testing.T) {
	loader := &fileLoader{prefix: "/w/t"}
	cases := []struct {
		relative string
		anchor   string
		want     string
	}{
		{"./inc.peb", "sub/s.peb", "sub/inc.peb"},
		{"../inc.peb", "sub/s.peb", "inc.peb"},
		{"../../etc/passwd", "i.peb", "etc/passwd"},
		{".\\inc.peb", "sub\\s.peb", "sub/inc.peb"},
		{"./a/./b.peb", "sub/s.peb", "sub/a/b.peb"},
		{"inc.peb", "sub/s.peb", ""},
		{"/inc.peb", "sub/s.peb", ""},
		{"", "s.peb", ""},
	}
	for _, testCase := range cases {
		if got := loader.resolveRelativePath(testCase.relative, testCase.anchor); got != testCase.want {
			t.Errorf("resolveRelativePath(%q, %q) = %q, want %q",
				testCase.relative, testCase.anchor, got, testCase.want)
		}
	}
}

func TestLoaderPrefix(t *testing.T) {
	if _, err := newFileLoader("  "); err == nil || err.Error() != "Prefix cannot be empty (?:?)" {
		t.Fatalf("got %v", err)
	}
	if _, err := newFileLoader("t"); err == nil ||
		err.Error() != "Prefix must be an absolute path (?:?)" {
		t.Fatalf("got %v", err)
	}
}

func TestLoaderErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	loader, err := newFileLoader(dir)
	if err != nil {
		t.Fatal(err)
	}

	_, err = loader.read("nope.peb")
	pebbleErr, ok := err.(*Error)
	if !ok || pebbleErr.Kind != "LoaderException" {
		t.Fatalf("got %v", err)
	}
	if want := "Could not find template [prefix='" + dir + "', templateName='nope.peb'] (?:?)"; pebbleErr.Error() != want {
		t.Fatalf("got %q", pebbleErr.Error())
	}
	cause, ok := pebbleErr.Cause.(*JavaError)
	if !ok || cause.Kind != "FileNotFoundException" ||
		cause.Message != filepath.Join(dir, "nope.peb")+" (No such file or directory)" {
		t.Fatalf("got %v", pebbleErr.Cause)
	}

	_, err = loader.read("sub")
	pebbleErr, _ = err.(*Error)
	if cause, ok := pebbleErr.Cause.(*JavaError); !ok ||
		cause.Message != filepath.Join(dir, "sub")+" (Is a directory)" {
		t.Fatalf("got %v", pebbleErr.Cause)
	}

	_, err = loader.read("/etc/passwd")
	if err == nil || err.Error() != "templateName '/etc/passwd' must be relative (?:?)" {
		t.Fatalf("got %v", err)
	}

	_, err = loader.read("../outside.peb")
	want := "template is not in the base directory path [baseDir='" + dir +
		"', templateName='../outside.peb'] (?:?)"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}

func TestLoaderReadsUTF8(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "i.peb"), []byte("héllo"), 0o644); err != nil {
		t.Fatal(err)
	}
	loader, err := newFileLoader(dir)
	if err != nil {
		t.Fatal(err)
	}
	source, err := loader.read("i.peb")
	if err != nil || source != "héllo" {
		t.Fatalf("got %q, %v", source, err)
	}
}
