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
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Parse is new SimpleDateFormat(pattern, locale).parse(text) with the default time zone: it is
// lenient, so out of range fields roll over. An unparseable text is a ParseException error whose
// message is the JDK's (Unparseable date: "..."), and a bad pattern an IllegalArgumentException.
func Parse(text, pattern, locale, defaultZone string) (Millis, error) {
	items, err := compilePattern(pattern)
	if err != nil {
		return 0, err
	}
	tag, ld := resolveLocale(locale)
	p := &dateParser{
		text:       []rune(text),
		errorIndex: -1,
		ld:         ld,
		tag:        tag,
		zone:       resolveZone(defaultZone),
	}
	if err := p.run(items); err != nil {
		return 0, err
	}
	// java.text.DateFormat.parse(String) reports failure by the parse position never having
	// moved, which is also what an empty pattern does: it raises the same ParseException with an
	// error offset of -1.
	if p.failed || p.index == 0 {
		return 0, &ParseError{Message: `Unparseable date: "` + text + `"`, Offset: p.errorIndex}
	}
	return p.resolve(), nil
}

// dateParser holds the fields a parse has gathered so far, the way java.text.CalendarBuilder
// does: which fields were set matters as much as their values, because Calendar resolves the
// overlapping ones by what was set.
type dateParser struct {
	text       []rune
	index      int
	errorIndex int
	failed     bool
	ld         localeData
	tag        string
	zone       *javaZone

	era, month, day           int
	dayOfYear, weekOfYear     int
	weekOfMonth, dayOfWeek    int
	hourOfDay, hour, ampm     int
	minute, second, millis    int
	year                      int64
	zoneOffset                int
	eraSet, yearSet, monthSet bool
	daySet, doySet, woySet    bool
	womSet, dowSet            bool
	hodSet, hourSet, ampmSet  bool
	zoneSet, ambiguousYear    bool

	// dateGroup records which set of date fields was filled in last, standing in for Calendar's
	// "most recently set field wins" resolution: 1 month and day, 2 day of year, 3 week of year,
	// 4 week of month.
	dateGroup int
}

// run walks the compiled pattern. A literal has to match exactly; a field run skips the blanks
// before it first, as SimpleDateFormat.subParse does.
func (p *dateParser) run(items []patternItem) error {
	for n, it := range items {
		// A numeric field immediately followed by another field reads at most as many digits as
		// it has pattern letters, which is what makes "yyyyMMdd" parse at all.
		obeyCount := n+1 < len(items) && items[n+1].letter != 0
		if it.letter == 0 {
			lit := []rune(it.text)
			if p.index+len(lit) > len(p.text) || string(p.text[p.index:p.index+len(lit)]) != it.text {
				p.fail(p.index)
				return nil
			}
			p.index += len(lit)
			continue
		}
		start := p.index
		for p.index < len(p.text) && (p.text[p.index] == ' ' || p.text[p.index] == '\t') {
			p.index++
		}
		if p.index >= len(p.text) {
			p.fail(start)
			return nil
		}
		err, ok := p.field(it, obeyCount)
		if err != nil {
			return err
		}
		if !ok {
			p.fail(start)
			return nil
		}
	}
	return nil
}

func (p *dateParser) fail(at int) {
	p.failed = true
	p.errorIndex = at
	p.index = 0
}

// field parses one run of pattern letters, returning false when the text does not match.
func (p *dateParser) field(it patternItem, obeyCount bool) (error, bool) {
	count := it.count
	maxDigits := 0
	if obeyCount {
		maxDigits = count
	}
	switch it.letter {
	case 'G':
		i, ok := p.matchText(p.ld.eras[:])
		if ok {
			p.era, p.eraSet = i, true
		}
		return nil, ok
	case 'y', 'Y':
		start := p.index
		v, ok := p.number(maxDigits)
		if !ok {
			return nil, false
		}
		// Two digit years pivot: a year written with at most two pattern letters and read as
		// exactly two digits belongs to the hundred years starting at get2DigitYearStart.
		if count <= 2 && p.index-start == 2 && unicode.IsDigit(p.text[start]) {
			p.year = p.pivotYear(v)
		} else {
			p.year = v
		}
		p.yearSet = true
		return nil, true
	case 'M', 'L':
		if count <= 2 {
			v, ok := p.number(maxDigits)
			if !ok {
				return nil, false
			}
			p.month, p.monthSet, p.dateGroup = int(v)-1, true, 1
			return nil, true
		}
		i, ok := p.matchText(p.ld.monthsLong[:], p.ld.monthsShort[:], p.ld.monthsStandLong[:],
			p.ld.monthsStandShort[:], p.ld.altMonthsLong[:], p.ld.altMonthsShort[:])
		if ok {
			p.month, p.monthSet, p.dateGroup = i, true, 1
		}
		return nil, ok
	case 'd':
		v, ok := p.number(maxDigits)
		if ok {
			p.day, p.daySet, p.dateGroup = int(v), true, 1
		}
		return nil, ok
	case 'D':
		v, ok := p.number(maxDigits)
		if ok {
			p.dayOfYear, p.doySet, p.dateGroup = int(v), true, 2
		}
		return nil, ok
	case 'w':
		v, ok := p.number(maxDigits)
		if ok {
			p.weekOfYear, p.woySet, p.dateGroup = int(v), true, 3
		}
		return nil, ok
	case 'W':
		v, ok := p.number(maxDigits)
		if ok {
			p.weekOfMonth, p.womSet, p.dateGroup = int(v), true, 4
		}
		return nil, ok
	case 'E':
		i, ok := p.matchText(p.ld.weekdaysLong[:], p.ld.weekdaysShort[:],
			p.ld.altWeekdaysLong[:], p.ld.altWeekdaysShort[:])
		if ok {
			p.dayOfWeek, p.dowSet = i+1, true
		}
		return nil, ok
	case 'u':
		v, ok := p.number(maxDigits)
		if ok {
			// Monday is 1 here and Sunday 7, against Calendar's Sunday first numbering.
			p.dayOfWeek, p.dowSet = int(v)%7+1, true
		}
		return nil, ok
	case 'F':
		// The day of week in month is parsed and then ignored, as a contradictory day of week is.
		_, ok := p.number(maxDigits)
		return nil, ok
	case 'a':
		i, ok := p.matchText(p.ld.amPM[:])
		if ok {
			p.ampm, p.ampmSet = i, true
		}
		return nil, ok
	case 'H':
		v, ok := p.number(maxDigits)
		if ok {
			p.hourOfDay, p.hodSet = int(v), true
		}
		return nil, ok
	case 'k':
		v, ok := p.number(maxDigits)
		if ok {
			if v == 24 {
				v = 0
			}
			p.hourOfDay, p.hodSet = int(v), true
		}
		return nil, ok
	case 'h':
		v, ok := p.number(maxDigits)
		if ok {
			if v == 12 {
				v = 0
			}
			p.hour, p.hourSet = int(v), true
		}
		return nil, ok
	case 'K':
		v, ok := p.number(maxDigits)
		if ok {
			p.hour, p.hourSet = int(v), true
		}
		return nil, ok
	case 'm':
		v, ok := p.number(maxDigits)
		if ok {
			p.minute = int(v)
		}
		return nil, ok
	case 's':
		v, ok := p.number(maxDigits)
		if ok {
			p.second = int(v)
		}
		return nil, ok
	case 'S':
		v, ok := p.number(maxDigits)
		if ok {
			p.millis = int(v)
		}
		return nil, ok
	case 'z', 'Z':
		// Java makes no distinction between the two on input: both take an RFC 822 offset, a
		// "GMT+hh:mm" offset or a zone name, and neither takes a bare "Z" or "+hh:mm".
		return nil, p.zoneField()
	case 'X':
		if count >= 4 {
			return &IllegalArgumentError{Message: "invalid ISO 8601 format: length=" + strconv.Itoa(count)}, false
		}
		return nil, p.isoZoneField(count)
	}
	return nil, false
}

// number reads a decimal integer, optionally signed, of at most maxDigits digits when that is
// positive. Java's NumberFormat accepts a minus sign but not a plus one.
func (p *dateParser) number(maxDigits int) (int64, bool) {
	i := p.index
	neg := false
	if i < len(p.text) && p.text[i] == '-' {
		neg, i = true, i+1
	}
	start := i
	var v int64
	for i < len(p.text) && p.text[i] >= '0' && p.text[i] <= '9' {
		if maxDigits > 0 && i-start >= maxDigits {
			break
		}
		v = v*10 + int64(p.text[i]-'0')
		i++
	}
	if i == start {
		return 0, false
	}
	p.index = i
	if neg {
		return -v, true
	}
	return v, true
}

// matchText is SimpleDateFormat.matchString: the longest of the candidate names that matches the
// text at the current position, ignoring case. Every style is offered, whatever the pattern count
// was, which is why "MMMM" happily reads "Mar".
func (p *dateParser) matchText(candidates ...[]string) (int, bool) {
	rest := p.text[p.index:]
	best, bestLen := -1, 0
	for _, arr := range candidates {
		for i, name := range arr {
			n := []rune(name)
			if len(n) == 0 || len(n) <= bestLen || len(n) > len(rest) {
				continue
			}
			if runesEqualFold(rest[:len(n)], n) {
				best, bestLen = i, len(n)
			}
		}
	}
	if best < 0 {
		return 0, false
	}
	p.index += bestLen
	return best, true
}

func runesEqualFold(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] && unicode.ToLower(a[i]) != unicode.ToLower(b[i]) {
			return false
		}
	}
	return true
}

// zoneField parses the general time zone forms accepted by both z and Z.
func (p *dateParser) zoneField() bool {
	rest := p.text[p.index:]
	if len(rest) >= 3 && strings.EqualFold(string(rest[:3]), "GMT") {
		p.index += 3
		p.zoneOffset, p.zoneSet = 0, true
		if off, n, ok := parseSignedOffset(p.text[p.index:], true); ok {
			p.index += n
			p.zoneOffset = off
		}
		return true
	}
	if len(rest) > 0 && (rest[0] == '+' || rest[0] == '-') {
		// RFC 822: exactly four digits after the sign, so "+05:30" is not a zone here.
		off, n, ok := parseSignedOffset(rest, false)
		if !ok {
			return false
		}
		p.index += n
		p.zoneOffset, p.zoneSet = off, true
		return true
	}
	return p.matchZoneName()
}

// parseSignedOffset reads a signed offset. Outside a "GMT" prefix only the RFC 822 "+hhmm" form
// is legal; after "GMT" the hours alone and a colon separated form are allowed too.
func parseSignedOffset(r []rune, afterGMT bool) (int, int, bool) {
	if len(r) < 2 || (r[0] != '+' && r[0] != '-') {
		return 0, 0, false
	}
	sign := 1
	if r[0] == '-' {
		sign = -1
	}
	digits := 0
	for digits < 4 && 1+digits < len(r) && isASCIIDigit(r[1+digits]) {
		digits++
	}
	switch {
	case !afterGMT && digits == 4:
		h := int(r[1]-'0')*10 + int(r[2]-'0')
		m := int(r[3]-'0')*10 + int(r[4]-'0')
		return sign * (h*60 + m) * 60000, 5, true
	case afterGMT && digits >= 1:
		// "GMT+5", "GMT+05", "GMT+05:30" and "GMT+0530".
		n := 1
		h := 0
		for n < len(r) && isASCIIDigit(r[n]) && n <= 2 {
			h = h*10 + int(r[n]-'0')
			n++
		}
		m := 0
		if n < len(r) && r[n] == ':' {
			n++
			for n < len(r) && isASCIIDigit(r[n]) {
				m = m*10 + int(r[n]-'0')
				n++
			}
		} else if digits == 4 {
			m = int(r[3]-'0')*10 + int(r[4]-'0')
			n = 5
		}
		return sign * (h*60 + m) * 60000, n, true
	}
	return 0, 0, false
}

func isASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

// zoneNameOrder is the order zone names are tried in, so that an abbreviation several zones share
// ("CST") always resolves the same way. Java's own order comes from its zone string table.
var zoneNameOrder = func() []string {
	ids := make([]string, 0, len(zoneDisplayNames))
	for id := range zoneDisplayNames {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}()

// matchZoneName matches a zone's display name and takes the offset from it: a daylight name sets
// the daylight offset even for an instant outside the daylight period, as Java does.
func (p *dateParser) matchZoneName() bool {
	rest := p.text[p.index:]
	bestLen, bestID, bestDST := 0, "", false
	for _, id := range zoneNameOrder {
		names, ok := zoneDisplayNames[id][p.tag]
		if !ok {
			names = zoneDisplayNames[id]["und"]
		}
		for i, name := range names {
			n := []rune(name)
			if len(n) == 0 || len(n) <= bestLen || len(n) > len(rest) {
				continue
			}
			if runesEqualFold(rest[:len(n)], n) {
				bestLen, bestID, bestDST = len(n), id, i >= 2
			}
		}
	}
	if bestLen == 0 {
		return false
	}
	z := resolveZone(bestID)
	off := z.raw
	if bestDST {
		off += z.savings
	}
	p.index += bestLen
	p.zoneOffset, p.zoneSet = off, true
	return true
}

// isoZoneField parses the X forms: "Z" or a signed offset whose shape depends on the count.
func (p *dateParser) isoZoneField(count int) bool {
	rest := p.text[p.index:]
	if len(rest) > 0 && rest[0] == 'Z' {
		p.index++
		p.zoneOffset, p.zoneSet = 0, true
		return true
	}
	if len(rest) < 3 || (rest[0] != '+' && rest[0] != '-') {
		return false
	}
	sign := 1
	if rest[0] == '-' {
		sign = -1
	}
	if !isASCIIDigit(rest[1]) || !isASCIIDigit(rest[2]) {
		return false
	}
	h := int(rest[1]-'0')*10 + int(rest[2]-'0')
	m, n := 0, 3
	switch count {
	case 1:
		// One letter stops at the hours, leaving any minutes in the text unconsumed.
	case 2:
		if len(rest) < 5 || !isASCIIDigit(rest[3]) || !isASCIIDigit(rest[4]) {
			return false
		}
		m, n = int(rest[3]-'0')*10+int(rest[4]-'0'), 5
	default:
		if len(rest) < 6 || rest[3] != ':' || !isASCIIDigit(rest[4]) || !isASCIIDigit(rest[5]) {
			return false
		}
		m, n = int(rest[4]-'0')*10+int(rest[5]-'0'), 6
	}
	p.index += n
	p.zoneOffset, p.zoneSet = sign*(h*60+m)*60000, true
	return true
}

// pivotYear turns a two digit year into a full one, relative to get2DigitYearStart. The value
// equal to the pivot's own two digits is ambiguous and is settled once the whole date is known.
func (p *dateParser) pivotYear(v int64) int64 {
	_, startYear := p.centuryStart()
	amb := floorMod(startYear, 100)
	century := startYear - amb
	switch {
	case v < amb:
		return century + 100 + v
	case v > amb:
		return century + v
	default:
		p.ambiguousYear = true
		return century + v
	}
}

// centuryStart is SimpleDateFormat.get2DigitYearStart. Java fixes it when the formatter is built,
// as "now minus 80 years" in the formatter's own zone, so it moves with the clock; this port does
// the same rather than freezing a pivot that would drift away from the Java it must match.
func (p *dateParser) centuryStart() (int64, int64) {
	nowMillis := time.Now().UnixMilli()
	f := computeFields(nowMillis+int64(p.zone.offset(Millis(nowMillis))), p.ld.firstDayOfWeek, p.ld.minimalDays)
	year := f.astroYear - 80
	local := daysFromYMD(year, f.month+1, f.day)*millisPerDay +
		int64(f.hour)*3600000 + int64(f.minute)*60000 + int64(f.second)*1000 + int64(f.millis)
	return local - int64(p.zone.offsetForLocal(local)), year
}

// resolve turns the gathered fields into an instant, the way java.util.Calendar does in lenient
// mode: every field is simply added on, so a month of 13 or an hour of 25 rolls over.
func (p *dateParser) resolve() Millis {
	result := p.instant(p.astronomicalYear())
	if p.ambiguousYear {
		if start, _ := p.centuryStart(); int64(result) < start {
			result = p.instant(p.astronomicalYear() + 100)
		}
	}
	return result
}

// astronomicalYear is the parsed year with the era folded in: Calendar.YEAR counts from 1 inside
// its era, so 1 BC is the astronomical year 0.
func (p *dateParser) astronomicalYear() int64 {
	if !p.yearSet {
		return 1970
	}
	if p.eraSet && p.era == eraBC {
		return 1 - p.year
	}
	return p.year
}

func (p *dateParser) instant(year int64) Millis {
	fdow, minDays := p.ld.firstDayOfWeek, p.ld.minimalDays
	dow := fdow
	if p.dowSet {
		dow = p.dayOfWeek
	}
	var days int64
	switch p.dateGroup {
	case 2:
		days = daysFromYMD(year, 1, 1) + int64(p.dayOfYear) - 1
	case 3:
		start := firstWeekStart(daysFromYMD(year, 1, 1), fdow, minDays)
		days = start + int64(p.weekOfYear-1)*7 + int64((dow-fdow+7)%7)
	case 4:
		month := p.month
		if !p.monthSet {
			month = 0
		}
		y := year + floorDiv(int64(month), 12)
		m := int(floorMod(int64(month), 12)) + 1
		start := firstWeekStart(daysFromYMD(y, m, 1), fdow, minDays)
		days = start + int64(p.weekOfMonth-1)*7 + int64((dow-fdow+7)%7)
	default:
		// The month is allowed out of range: month 12 is next January, month -1 last December.
		y := year + floorDiv(int64(p.month), 12)
		m := int(floorMod(int64(p.month), 12)) + 1
		day := 1
		if p.daySet {
			day = p.day
		}
		days = daysFromYMD(y, m, day)
	}

	// Calendar prefers the hour of day when it was set, and otherwise builds the hour out of the
	// twelve hour field and am/pm, whose default is am.
	hours := int64(p.hour)
	if p.hodSet {
		hours = int64(p.hourOfDay)
	} else if p.ampmSet {
		hours += 12 * int64(p.ampm)
	}
	local := days*millisPerDay + hours*3600000 +
		int64(p.minute)*60000 + int64(p.second)*1000 + int64(p.millis)
	if p.zoneSet {
		return Millis(local - int64(p.zoneOffset))
	}
	return Millis(local - int64(p.zone.offsetForLocal(local)))
}
