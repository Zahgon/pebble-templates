/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jtime

import "testing"

// These tables are the readable counterpart to the generated corpus: every expectation was taken
// from the reference JDK, and each case stands for a rule worth stating out loud.

func TestFormatPatternLetters(t *testing.T) {
	cases := []struct {
		name         string
		date         Millis
		pattern      string
		locale, zone string
		want         string
	}{
		{"epoch, every letter", 0, "G y Y M L w W D d F E u a H k K h m s S z Z X", "en-US", "UTC",
			"AD 1970 1970 1 1 1 1 1 1 1 Thu 4 AM 0 24 0 12 0 0 0 UTC +0000 Z"},
		{"yy clips to two digits", 1700000000000, "yy/yyy/yyyyy", "en-US", "UTC", "23/2023/02023"},
		{"S is the millisecond field, not a fraction", 1136214245678, "S/SS/SSSS", "en-US", "UTC", "678/678/0678"},
		{"month counts", 1700000000000, "M/MM/MMM/MMMM/MMMMM", "en-US", "UTC", "11/11/Nov/November/November"},
		{"German MMM is contextual beside another field", 1700000000000, "d MMM", "de-DE", "UTC", "14 Nov."},
		{"German MMM stands alone", 1700000000000, "MMM", "de-DE", "UTC", "Nov"},
		{"L is always the standalone name", 1700000000000, "LLL/LLLL", "de-DE", "UTC", "Nov/November"},
		{"weekday counts", 1700000000000, "E/EEEE", "fr-FR", "UTC", "mar./mardi"},
		{"half hour zone", 1700000000000, "z/zzzz/Z/X/XX/XXX", "en-US", "Asia/Kolkata",
			"IST/India Standard Time/+0530/+05/+0530/+05:30"},
		{"X is Z at zero offset", 0, "X/XX/XXX/Z", "en-US", "UTC", "Z/Z/Z/+0000"},
		{"daylight picks the summer name", 1626000000000, "z/zzzz/Z", "en-US", "America/New_York",
			"EDT/Eastern Daylight Time/-0400"},
		{"German localises the short zone name", 1626000000000, "z/zzzz", "de-DE", "Europe/Berlin",
			"MESZ/Mitteleuropäische Sommerzeit"},
		{"Lord Howe saves half an hour", 1609459200000, "z/Z", "en-US", "Australia/Lord_Howe", "LHDT/+1100"},
		{"a custom zone displays as its own id", 1700000000000, "z/zzzz/Z", "en-US", "GMT+05:30",
			"GMT+05:30/GMT+05:30/+0530"},
		{"an unnamed zone displays as a GMT offset", 1700000000000, "z/zzzz", "en-US", "Etc/GMT+5",
			"GMT-05:00/GMT-05:00"},
		{"an unknown zone is GMT", 1700000000000, "z/Z", "en-US", "Nonsense/Zone", "GMT/+0000"},
		{"the three letter PST keeps US daylight rules", 1626000000000, "z/Z", "en-US", "PST", "PDT/-0700"},
		{"the three letter EST has none", 1626000000000, "z/Z", "en-US", "EST", "EST/-0500"},
		{"hour variants at midnight", 0, "H/k/K/h/a", "en-US", "UTC", "0/24/0/12/AM"},
		{"hour variants at noon", 43200000, "H/k/K/h/a", "en-US", "UTC", "12/12/0/12/PM"},
		{"week based year rolls back", 1609459200000, "yyyy/YYYY/ww/W", "de-DE", "UTC", "2021/2020/53/0"},
		{"and forward", 1577750400000, "yyyy/YYYY/ww/W", "en-US", "UTC", "2019/2020/01/5"},
		{"day of week in month", 1614816000000, "D/F/W", "en-US", "UTC", "63/1/1"},
		{"the Gregorian cutover", -12219292800000, "yyyy-MM-dd G D F", "en-US", "UTC", "1582-10-15 AD 278 1"},
		{"the day before it is Julian", -12219379200000, "yyyy-MM-dd G D", "en-US", "UTC", "1582-10-04 AD 277"},
		{"a BC year prints era relative", -62167392000000, "yyyy G/Y", "en-US", "UTC", "0001 BC/0"},
		{"and the week year goes negative", -62198928000000, "yyyy G/Y/YYYY", "en-US", "UTC", "0002 BC/-1/-0001"},
		{"Java uses the standard offset before a zone's history", -12219292800000, "yyyy-MM-dd Z", "en-US",
			"Europe/Paris", "1582-10-15 +0100"},
		{"quoting", 1136214245678, "hh 'o''clock' a", "en-US", "UTC", "03 o'clock PM"},
		{"a doubled quote outside a quoted run", 1136214245678, "'' HH", "en-US", "UTC", "' 15"},
		{"an empty pattern", 1136214245678, "", "en-US", "UTC", ""},
		{"non letters are literal", 1136214245678, "#@1-", "en-US", "UTC", "#@1-"},
		{"Turkish names", 1609459200000, "EEEE MMMM a G", "tr-TR", "UTC", "Cuma Ocak ÖÖ MS"},
		{"Japanese names", 1609459200000, "EEEE MMMM a G", "ja-JP", "UTC", "金曜日 1月 午前 西暦"},
		{"the root locale leaves names untranslated", 1609459200000, "MMMM G", "und", "UTC", "Jan CE"},
		{"an unknown locale falls back to the root", 1609459200000, "MMMM G", "xx-YY", "UTC", "Jan CE"},
		{"a language without a region", 1609459200000, "EEEE", "de", "UTC", "Freitag"},
		{"an underscore separated tag", 1609459200000, "EEEE", "fr_FR", "UTC", "vendredi"},
	}
	for _, c := range cases {
		got, err := Format(c.date, c.pattern, c.locale, c.zone)
		if err != nil {
			t.Errorf("%s: Format(%q) returned %v", c.name, c.pattern, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: Format(%d, %q, %q, %q) = %q, want %q", c.name, c.date, c.pattern, c.locale, c.zone, got, c.want)
		}
	}
}

func TestFormatPatternErrors(t *testing.T) {
	cases := []struct{ pattern, want string }{
		{"q", "Illegal pattern character 'q'"},
		{"qq", "Illegal pattern character 'q'"},
		{"yyyyb", "Illegal pattern character 'b'"},
		{"o''clock", "Illegal pattern character 'o'"},
		{"a'b'c", "Illegal pattern character 'c'"},
		{"B", "Illegal pattern character 'B'"},
		{"V", "Illegal pattern character 'V'"},
		{"'", "Unterminated quote"},
		{"yyyy'", "Unterminated quote"},
		{"'''", "Unterminated quote"},
		{"'abc", "Unterminated quote"},
		// The constructor accepts a long ISO zone run; formatting is where Java gives up.
		{"XXXX", "invalid ISO 8601 format: length=4"},
		{"XXXXX", "invalid ISO 8601 format: length=5"},
	}
	for _, c := range cases {
		_, err := Format(0, c.pattern, "en-US", "UTC")
		if err == nil {
			t.Errorf("Format(%q) succeeded, want error %q", c.pattern, c.want)
			continue
		}
		if err.Error() != c.want {
			t.Errorf("Format(%q) error = %q, want %q", c.pattern, err.Error(), c.want)
		}
		if _, ok := err.(*IllegalArgumentError); !ok {
			t.Errorf("Format(%q) error type = %T, want *IllegalArgumentError", c.pattern, err)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		pattern, text string
		wantOffset    int
	}{
		{"yyyy-MM-dd", "garbage", 0},
		{"yyyy-MM-dd", "", 0},
		{"yyyy-MM-dd", "2021/03/04", 4},
		{"yyyy-MM-dd", "2021-03", 7},
		{"yyyy-MM-dd", "x2021-03-04", 0},
		{"MMMM", "Foo", 0},
		{"", "", -1},
	}
	for _, c := range cases {
		_, err := Parse(c.text, c.pattern, "en-US", "UTC")
		pe, ok := err.(*ParseError)
		if !ok {
			t.Errorf("Parse(%q, %q) error type = %T, want *ParseError", c.text, c.pattern, err)
			continue
		}
		if want := `Unparseable date: "` + c.text + `"`; pe.Message != want {
			t.Errorf("Parse(%q, %q) message = %q, want %q", c.text, c.pattern, pe.Message, want)
		}
		if pe.Offset != c.wantOffset {
			t.Errorf("Parse(%q, %q) offset = %d, want %d", c.text, c.pattern, pe.Offset, c.wantOffset)
		}
	}
}

func TestZones(t *testing.T) {
	// getOffset at 2021-01-01 and at 2021-07-01, the way java.util.TimeZone reports them.
	const jan, jul = Millis(1609459200000), Millis(1625097600000)
	cases := []struct {
		zone   string
		winter int
		summer int
		exists bool
	}{
		{"UTC", 0, 0, true},
		{"GMT", 0, 0, true},
		{"America/New_York", -18000000, -14400000, true},
		{"Europe/Berlin", 3600000, 7200000, true},
		{"Asia/Kolkata", 19800000, 19800000, true},
		{"Asia/Tokyo", 32400000, 32400000, true},
		{"Australia/Lord_Howe", 39600000, 37800000, true},
		{"Pacific/Kiritimati", 50400000, 50400000, true},
		{"America/Sao_Paulo", -10800000, -10800000, true},
		{"GMT+05:30", 19800000, 19800000, true},
		{"GMT+0530", 19800000, 19800000, true},
		{"GMT+5", 18000000, 18000000, true},
		{"PST", -28800000, -25200000, true},
		{"IST", 19800000, 19800000, true},
		{"EST", -18000000, -18000000, true},
		{"CST", -21600000, -18000000, true},
		{"HST", -36000000, -36000000, true},
		{"Etc/GMT+5", -18000000, -18000000, true},
		{"Asia/Calcutta", 19800000, 19800000, true},
		// Java hands back GMT for anything it does not know, bare offsets included.
		{"+05:30", 0, 0, false},
		{"Z", 0, 0, false},
		{"Nonsense/Zone", 0, 0, false},
		{"", 0, 0, false},
		{"gmt", 0, 0, false},
		{"GMT+", 0, 0, false},
		{"GMT+24:00", 0, 0, false},
	}
	for _, c := range cases {
		if got, _ := ZoneOffsetMillis(c.zone, jan); got != c.winter {
			t.Errorf("ZoneOffsetMillis(%q, winter) = %d, want %d", c.zone, got, c.winter)
		}
		if got, _ := ZoneOffsetMillis(c.zone, jul); got != c.summer {
			t.Errorf("ZoneOffsetMillis(%q, summer) = %d, want %d", c.zone, got, c.summer)
		}
		if got := ZoneExists(c.zone); got != c.exists {
			t.Errorf("ZoneExists(%q) = %v, want %v", c.zone, got, c.exists)
		}
	}
}

func TestParseTwoDigitYearPivot(t *testing.T) {
	// The pivot is "now minus 80 years", so only the shape of the rule can be asserted: a two
	// digit year below the pivot's own two digits lands in the current century, above it in the
	// previous one.
	year := func(text string) int64 {
		ms, err := Parse(text, "yy", "en-US", "UTC")
		if err != nil {
			t.Fatalf("Parse(%q, yy): %v", text, err)
		}
		s, err := Format(ms, "yyyy", "en-US", "UTC")
		if err != nil {
			t.Fatal(err)
		}
		var y int64
		for _, r := range s {
			y = y*10 + int64(r-'0')
		}
		return y
	}
	if y := year("00"); y != 2000 {
		t.Errorf(`Parse("00", "yy") year = %d, want 2000`, y)
	}
	if y := year("99"); y != 1999 {
		t.Errorf(`Parse("99", "yy") year = %d, want 1999`, y)
	}
	// Three digits or a pattern of three letters or more switch the pivot off entirely.
	if y := year("100"); y != 100 {
		t.Errorf(`Parse("100", "yy") year = %d, want 100`, y)
	}
	if ms, err := Parse("21", "yyyy", "en-US", "UTC"); err != nil {
		t.Fatal(err)
	} else if got, _ := Format(ms, "yyyy", "en-US", "UTC"); got != "0021" {
		t.Errorf(`Parse("21", "yyyy") = %s, want 0021`, got)
	}
}
