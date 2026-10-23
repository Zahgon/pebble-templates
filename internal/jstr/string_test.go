/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jstr

import "testing"

// The expected values below were taken from the Java runtime (JDK 21), the same one the release this port
// reproduces runs on.

// TestToUpperRoot checks String.toUpperCase(Locale.ROOT).
func TestToUpperRoot(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"stra\u00dfe", "STRASSE"},
		{"\ufb04", "FFL"},
		{"\u0149", "\u02bcN"},
		{"\u0130stanbul", "\u0130STANBUL"},
		{"\u03a3\u039f\u03a6\u039f\u03a3", "\u03a3\u039f\u03a6\u039f\u03a3"},
		{"\u039f\u0394\u039f\u03a3", "\u039f\u0394\u039f\u03a3"},
		{"\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2", "\u03a3\u038a\u03a3\u03a5\u03a6\u039f\u03a3"},
		{"a\U0001d518b", "A\U0001d518B"},
		{"\U0001f600x", "\U0001f600X"},
		{"", ""},
		{"\u001ca\u001c", "\u001cA\u001c"},
		{"\u01c5ungla", "\u01c4UNGLA"},
		{"hello", "HELLO"},
		{"\u01c6", "\u01c4"},
		{"", ""},
	}
	for _, each := range cases {
		if got := ToUpper(each.in, "en-US"); got != each.want {
			t.Errorf("%q: got %q, want %q", each.in, got, each.want)
		}
	}
}

// TestToLowerRoot checks String.toLowerCase(Locale.ROOT).
func TestToLowerRoot(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"stra\u00dfe", "stra\u00dfe"},
		{"\ufb04", "\ufb04"},
		{"\u0149", "\u0149"},
		{"\u0130stanbul", "i\u0307stanbul"},
		{"\u03a3\u039f\u03a6\u039f\u03a3", "\u03c3\u03bf\u03c6\u03bf\u03c2"},
		{"\u039f\u0394\u039f\u03a3", "\u03bf\u03b4\u03bf\u03c2"},
		{"\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2", "\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2"},
		{"a\U0001d518b", "a\U0001d518b"},
		{"\U0001f600x", "\U0001f600x"},
		{"", ""},
		{"\u001ca\u001c", "\u001ca\u001c"},
		{"\u01c5ungla", "\u01c6ungla"},
		{"hello", "hello"},
		{"\u01c6", "\u01c6"},
		{"", ""},
	}
	for _, each := range cases {
		if got := ToLower(each.in, "en-US"); got != each.want {
			t.Errorf("%q: got %q, want %q", each.in, got, each.want)
		}
	}
}

// TestToUpperTurkish checks the Turkish overrides.
func TestToUpperTurkish(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"stra\u00dfe", "STRASSE"},
		{"\ufb04", "FFL"},
		{"\u0149", "\u02bcN"},
		{"\u0130stanbul", "\u0130STANBUL"},
		{"\u03a3\u039f\u03a6\u039f\u03a3", "\u03a3\u039f\u03a6\u039f\u03a3"},
		{"\u039f\u0394\u039f\u03a3", "\u039f\u0394\u039f\u03a3"},
		{"\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2", "\u03a3\u038a\u03a3\u03a5\u03a6\u039f\u03a3"},
		{"a\U0001d518b", "A\U0001d518B"},
		{"\U0001f600x", "\U0001f600X"},
		{"", ""},
		{"\u001ca\u001c", "\u001cA\u001c"},
		{"\u01c5ungla", "\u01c4UNGLA"},
		{"hello", "HELLO"},
		{"\u01c6", "\u01c4"},
		{"", ""},
	}
	for _, each := range cases {
		if got := ToUpper(each.in, "tr-TR"); got != each.want {
			t.Errorf("%q: got %q, want %q", each.in, got, each.want)
		}
	}
}

// TestToLowerTurkish checks the Turkish overrides.
func TestToLowerTurkish(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"stra\u00dfe", "stra\u00dfe"},
		{"\ufb04", "\ufb04"},
		{"\u0149", "\u0149"},
		{"\u0130stanbul", "istanbul"},
		{"\u03a3\u039f\u03a6\u039f\u03a3", "\u03c3\u03bf\u03c6\u03bf\u03c2"},
		{"\u039f\u0394\u039f\u03a3", "\u03bf\u03b4\u03bf\u03c2"},
		{"\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2", "\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2"},
		{"a\U0001d518b", "a\U0001d518b"},
		{"\U0001f600x", "\U0001f600x"},
		{"", ""},
		{"\u001ca\u001c", "\u001ca\u001c"},
		{"\u01c5ungla", "\u01c6ungla"},
		{"hello", "hello"},
		{"\u01c6", "\u01c6"},
		{"", ""},
	}
	for _, each := range cases {
		if got := ToLower(each.in, "tr-TR"); got != each.want {
			t.Errorf("%q: got %q, want %q", each.in, got, each.want)
		}
	}
}

// TestTrim checks String.trim.
func TestTrim(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"stra\u00dfe", "stra\u00dfe"},
		{"\ufb04", "\ufb04"},
		{"\u0149", "\u0149"},
		{"\u0130stanbul", "\u0130stanbul"},
		{"\u03a3\u039f\u03a6\u039f\u03a3", "\u03a3\u039f\u03a6\u039f\u03a3"},
		{"\u039f\u0394\u039f\u03a3", "\u039f\u0394\u039f\u03a3"},
		{"\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2", "\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2"},
		{"a\U0001d518b", "a\U0001d518b"},
		{"\U0001f600x", "\U0001f600x"},
		{"", ""},
		{"\u001ca\u001c", "a"},
		{"\u01c5ungla", "\u01c5ungla"},
		{"hello", "hello"},
		{"\u01c6", "\u01c6"},
		{"", ""},
	}
	for _, each := range cases {
		if got := Trim(each.in); got != each.want {
			t.Errorf("%q: got %q, want %q", each.in, got, each.want)
		}
	}
}

// TestStrip checks String.strip.
func TestStrip(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"stra\u00dfe", "stra\u00dfe"},
		{"\ufb04", "\ufb04"},
		{"\u0149", "\u0149"},
		{"\u0130stanbul", "\u0130stanbul"},
		{"\u03a3\u039f\u03a6\u039f\u03a3", "\u03a3\u039f\u03a6\u039f\u03a3"},
		{"\u039f\u0394\u039f\u03a3", "\u039f\u0394\u039f\u03a3"},
		{"\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2", "\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2"},
		{"a\U0001d518b", "a\U0001d518b"},
		{"\U0001f600x", "\U0001f600x"},
		{"", ""},
		{"\u001ca\u001c", "a"},
		{"\u01c5ungla", "\u01c5ungla"},
		{"hello", "hello"},
		{"\u01c6", "\u01c6"},
		{"", ""},
	}
	for _, each := range cases {
		if got := Strip(each.in); got != each.want {
			t.Errorf("%q: got %q, want %q", each.in, got, each.want)
		}
	}
}

// TestLengthAndHashCode checks String.length and String.hashCode over UTF-16 code units.
func TestLengthAndHashCode(t *testing.T) {
	cases := []struct {
		in     string
		length int
		hash   int32
	}{
		{"stra\u00dfe", 6, -891990090},
		{"\ufb04", 1, 64260},
		{"\u0149", 1, 329},
		{"\u0130stanbul", 8, -363956677},
		{"\u03a3\u039f\u03a6\u039f\u03a3", 5, 888341550},
		{"\u039f\u0394\u039f\u03a3", 4, 28526201},
		{"\u03c3\u03af\u03c3\u03c5\u03c6\u03bf\u03c2", 7, 2114096571},
		{"a\U0001d518b", 4, 57834814},
		{"\U0001f600x", 3, 54959989},
		{"", 0, 0},
		{"\u001ca\u001c", 3, 29943},
		{"\u01c5ungla", 6, 195534910},
		{"hello", 5, 99162322},
		{"\u01c6", 1, 454},
		{"", 0, 0},
	}
	for _, each := range cases {
		if got := Length(each.in); got != each.length {
			t.Errorf("Length(%q) = %d, want %d", each.in, got, each.length)
		}
		if got := HashCode(each.in); got != each.hash {
			t.Errorf("HashCode(%q) = %d, want %d", each.in, got, each.hash)
		}
	}
}

// TestAbbreviate checks the abbreviate filter over UTF-16 code units.
func TestAbbreviate(t *testing.T) {
	cases := []struct {
		in    string
		width int
		want  string
	}{
		{"hello", 0, ""},
		{"hello", 2, "he"},
		{"hello", 3, "hel"},
		{"hello", 4, "h..."},
		{"hello", 6, "hello"},
		{"hello", 20, "hello"},
		{"a\U0001d518bcdef", 0, ""},
		// Go strings cannot represent Java's lone UTF-16 surrogate directly;
		// utf16.Decode uses the Unicode replacement character for that unit.
		{"a\U0001d518bcdef", 2, "a\ufffd"},
		{"a\U0001d518bcdef", 3, "a\U0001d518"},
		{"a\U0001d518bcdef", 4, "a..."},
		{"a\U0001d518bcdef", 6, "a\U0001d518..."},
		{"a\U0001d518bcdef", 20, "a\U0001d518bcdef"},
		{"ab", 0, "ab"},
		{"ab", 2, "ab"},
		{"ab", 3, "ab"},
		{"ab", 4, "ab"},
		{"ab", 6, "ab"},
		{"ab", 20, "ab"},
		{"abcd", 0, ""},
		{"abcd", 2, "ab"},
		{"abcd", 3, "abc"},
		{"abcd", 4, "a..."},
		{"abcd", 6, "abcd"},
		{"abcd", 20, "abcd"},
	}
	for _, each := range cases {
		got, err := Abbreviate(each.in, each.width)
		if err != nil || got != each.want {
			t.Errorf("Abbreviate(%q, %d) = %q, %v, want %q", each.in, each.width, got, err, each.want)
		}
	}
}
