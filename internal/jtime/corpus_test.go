/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jtime

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The two corpora under testdata were produced by running the equivalent java.text.SimpleDateFormat
// calls on the reference JDK and recording what came back, so every expectation here is Java's own
// answer rather than a guess. format_cases.tsv is pattern, millis, zone, locale, result; a result of
// "EX" followed by a tab is the exception message. parse_cases.tsv is pattern, text, locale, default
// zone, result.

func readCases(t *testing.T, name string) [][]string {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out [][]string
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1<<16), 1<<20)
	for s.Scan() {
		if s.Text() == "" {
			continue
		}
		out = append(out, strings.Split(s.Text(), "\t"))
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// unescape reverses the escaping the generator applied to keep one case on one line.
func unescape(s string) string {
	s = strings.ReplaceAll(s, "\\t", "\t")
	s = strings.ReplaceAll(s, "\\n", "\n")
	return strings.ReplaceAll(s, "\\\\", "\\")
}

// knownDifferences are the corpus cases this port knowingly gets wrong. GregorianCalendar's
// getWeekYear mixes up the astronomical year with the era relative one on its slow path, which it
// takes for years at or before the Gregorian cutover: it rolls a BC date in week 1 of the next
// year forward for the astronomical year -1 but not for -2, in the same locale. The port computes
// the week based year consistently instead.
var knownDifferences = map[string]bool{
	"-62167392000000 YYYY-ww-u de-DE": true,
	"-62167392000000 YYYY-ww-u fr-FR": true,
	"-62167392000000 YYYY-ww-u en-GB": true,
}

func TestFormatAgainstJava(t *testing.T) {
	cases := readCases(t, "format_cases.tsv")
	if len(cases) < 1000 {
		t.Fatalf("corpus looks truncated: %d cases", len(cases))
	}
	bad := 0
	for _, c := range cases {
		pattern, zone, locale := unescape(c[0]), c[2], c[3]
		millis, err := strconv.ParseInt(c[1], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		want := unescape(c[4])
		if want == "EX" {
			want = "EX " + unescape(c[5])
		}
		if knownDifferences[c[1]+" "+pattern+" "+locale] {
			continue
		}
		got, err := Format(Millis(millis), pattern, locale, zone)
		if err != nil {
			got = "EX " + err.Error()
		}
		if got != want {
			if bad++; bad <= 20 {
				t.Errorf("Format(%d, %q, %q, %q) = %q, want %q", millis, pattern, locale, zone, got, want)
			}
		}
	}
	if bad > 20 {
		t.Errorf("%d of %d format cases differ from Java", bad, len(cases))
	}
}

func TestParseAgainstJava(t *testing.T) {
	cases := readCases(t, "parse_cases.tsv")
	bad := 0
	for _, c := range cases {
		pattern, text, locale, zone := unescape(c[0]), unescape(c[1]), c[2], c[3]
		want := c[4]
		if want == "EX" {
			want = "EX " + unescape(c[5])
		}
		var got string
		if ms, err := Parse(text, pattern, locale, zone); err != nil {
			got = "EX " + err.Error()
		} else {
			got = strconv.FormatInt(int64(ms), 10)
		}
		if got != want {
			if bad++; bad <= 20 {
				t.Errorf("Parse(%q, %q, %q, %q) = %s, want %s", text, pattern, locale, zone, got, want)
			}
		}
	}
	if bad > 20 {
		t.Errorf("%d of %d parse cases differ from Java", bad, len(cases))
	}
}
