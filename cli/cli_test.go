/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// replayCase is one recorded run of the Java release 4.1.2: the files of its directory, the command line and what
// the Java command printed and returned. testdata/cases.json holds a sample of them, so that the port's behaviour
// stays pinned to the original's.
type replayCase struct {
	ID    string            `json:"id"`
	Argv  []string          `json:"argv"`
	Files map[string]string `json:"files"`
	Out   string            `json:"out"`
	Err   string            `json:"err"`
	Code  int               `json:"code"`
}

func TestReplayRecordedRuns(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "cases.json"))
	if err != nil {
		t.Fatalf("read cases: %v", err)
	}
	var cases []replayCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatalf("parse cases: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no recorded cases")
	}
	working, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for _, each := range cases {
		t.Run(each.ID, func(t *testing.T) {
			dir := t.TempDir()
			for name, text := range each.Files {
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("make directory: %v", err)
				}
				if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
					t.Fatalf("write %s: %v", name, err)
				}
			}
			if err := os.Chdir(dir); err != nil {
				t.Fatalf("change directory: %v", err)
			}
			defer func() {
				if err := os.Chdir(working); err != nil {
					t.Fatalf("restore directory: %v", err)
				}
			}()
			var out, errOut bytes.Buffer
			code := Run(each.Argv, &out, &errOut)
			// The recorded standard error names the case directory in loader messages; the replay runs in a
			// temporary one, so only compare the part before that path when it appears.
			if out.String() != each.Out || code != each.Code || !sameDiagnostics(errOut.String(), each.Err, dir) {
				t.Errorf("argv %q\n out: %q\nwant: %q\n err: %q\nwant: %q\ncode: %d want %d",
					each.Argv, out.String(), each.Out, errOut.String(), each.Err, code, each.Code)
			}
		})
	}
}

// sameDiagnostics compares standard error, allowing the recorded absolute case directory to appear as this run's
// temporary directory.
func sameDiagnostics(got, want, dir string) bool {
	if got == want {
		return true
	}
	start := indexOfPrefixPath(want)
	if start < 0 {
		return false
	}
	return len(got) > start && got[:start] == want[:start] && containsDir(got, dir)
}

func indexOfPrefixPath(want string) int {
	return bytes.Index([]byte(want), []byte("[prefix='"))
}

func containsDir(got, dir string) bool {
	return bytes.Contains([]byte(got), []byte(dir))
}
