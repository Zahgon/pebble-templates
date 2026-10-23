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
	"fmt"
	"strings"
	"sync"
	"time"

	// Java carries its own copy of the zone database, so the port has to as well: without this
	// import a container without /usr/share/zoneinfo would silently fall back to UTC.
	_ "time/tzdata"
)

// javaZone is a java.util.TimeZone: the id Java reports for it plus the rules its offsets come
// from. Java hands back the GMT zone for an id it does not know, which is why unknown is kept.
type javaZone struct {
	id      string
	loc     *time.Location
	raw     int // TimeZone.getRawOffset, milliseconds
	savings int // TimeZone.getDSTSavings, milliseconds
	unknown bool
	// hasTransitions tells a zone with a history from a plain fixed offset, which matters only
	// for instants before the history starts.
	hasTransitions bool
}

var (
	zoneMu    sync.Mutex
	zoneCache = map[string]*javaZone{}
	zoneIDSet = func() map[string]bool {
		s := make(map[string]bool, len(javaZoneIDs))
		for _, id := range javaZoneIDs {
			s[id] = true
		}
		return s
	}()
)

// gmtZone is what TimeZone.getTimeZone hands back for an id it does not recognise.
func gmtZone(unknown bool) *javaZone {
	return &javaZone{id: "GMT", loc: time.UTC, unknown: unknown}
}

// resolveZone is java.util.TimeZone.getTimeZone: a known id, a custom "GMT+hh:mm" offset, or GMT.
// The order matters, because "GMT0" is a database id and must not be read as a custom offset.
func resolveZone(id string) *javaZone {
	zoneMu.Lock()
	defer zoneMu.Unlock()
	if z, ok := zoneCache[id]; ok {
		return z
	}
	z := buildZone(id)
	zoneCache[id] = z
	return z
}

func buildZone(id string) *javaZone {
	switch {
	case id == "GMT":
		return gmtZone(false)
	case id == "UT":
		// The JDK resolves this alias to the GMT zone itself, id included, unlike every other
		// alias (Asia/Calcutta and PST keep the id they were asked for).
		return gmtZone(false)
	case zoneIDSet[id]:
		target := id
		if t, ok := javaShortZoneIDs[id]; ok {
			target = t
		}
		if strings.HasPrefix(target, "+") || strings.HasPrefix(target, "-") {
			// A three letter id the JDK defines as a plain offset: EST, MST, HST.
			off := parseHourMinute(target)
			return &javaZone{id: id, loc: time.FixedZone(id, off/1000), raw: off}
		}
		loc, err := time.LoadLocation(target)
		if err != nil {
			// The Go database should hold every id Java's does; if it ever does not, fall back
			// to the standard offset Java reports for the zone rather than to UTC.
			meta := zoneMetadata[id]
			return &javaZone{id: id, loc: time.FixedZone(id, meta.raw/1000), raw: meta.raw}
		}
		z := &javaZone{id: id, loc: loc, hasTransitions: hasTransitions(loc)}
		if meta, ok := zoneMetadata[id]; ok {
			z.raw, z.savings = meta.raw, meta.dst
		} else {
			z.raw, z.savings = deriveRawOffset(loc)
		}
		return z
	case strings.HasPrefix(id, "GMT"):
		if norm, off, ok := parseCustomZone(id); ok {
			return &javaZone{id: norm, loc: time.FixedZone(norm, off/1000), raw: off}
		}
		return gmtZone(true)
	}
	return gmtZone(true)
}

// parseHourMinute reads the "-05:00" form the JDK uses for its fixed offset short ids.
func parseHourMinute(s string) int {
	sign := 1
	if s[0] == '-' {
		sign = -1
	}
	var h, m int
	fmt.Sscanf(s[1:], "%d:%d", &h, &m)
	return sign * (h*60 + m) * 60 * 1000
}

// parseCustomZone is TimeZone.parseCustomTimeZone: "GMT" and a signed offset written as h, hh,
// hhmm or hh:mm. Anything else is not a zone at all, and the caller falls back to GMT. The
// normalised id is always "GMT+hh:mm", which is also what the zone displays as.
func parseCustomZone(id string) (string, int, bool) {
	if len(id) < 5 {
		return "", 0, false
	}
	negative := false
	switch id[3] {
	case '-':
		negative = true
	case '+':
	default:
		return "", 0, false
	}
	hours, num, delims, digits := 0, 0, 0, 0
	for i := 4; i < len(id); i++ {
		c := id[i]
		if c == ':' {
			if delims > 0 || digits > 2 {
				return "", 0, false
			}
			hours, num, digits, delims = num, 0, 0, 1
			continue
		}
		if c < '0' || c > '9' {
			return "", 0, false
		}
		num = num*10 + int(c-'0')
		digits++
	}
	if delims == 0 {
		if digits <= 2 {
			hours, num = num, 0
		} else {
			hours, num = num/100, num%100
		}
	} else if digits != 2 {
		return "", 0, false
	}
	if hours > 23 || num > 59 {
		return "", 0, false
	}
	off := (hours*60 + num) * 60 * 1000
	sign := "+"
	if negative {
		off = -off
		sign = "-"
	}
	return fmt.Sprintf("GMT%s%02d:%02d", sign, hours, num), off, true
}

// hasTransitions reports whether the zone's history holds any transition at all.
func hasTransitions(loc *time.Location) bool {
	start, end := time.Date(2000, 1, 1, 0, 0, 0, 0, loc).ZoneBounds()
	return !start.IsZero() || !end.IsZero()
}

// deriveRawOffset guesses TimeZone.getRawOffset and getDSTSavings for a zone the dumped metadata
// does not cover, by sampling the current year: the smaller of the two offsets is standard time.
func deriveRawOffset(loc *time.Location) (raw, savings int) {
	_, jan := time.Date(time.Now().Year(), 1, 15, 0, 0, 0, 0, loc).Zone()
	_, jul := time.Date(time.Now().Year(), 7, 15, 0, 0, 0, 0, loc).Zone()
	if jan <= jul {
		return jan * 1000, (jul - jan) * 1000
	}
	return jul * 1000, (jan - jul) * 1000
}

// offset is TimeZone.getOffset(date): the whole offset from UTC, standard plus saving.
//
// Before a zone's first recorded transition Java answers with the zone's *current* standard
// offset, where Go answers with the local mean time of the first zone record: Europe/Paris in
// 1582 is +01:00 to Java and +00:09:21 to Go. Java wins here.
func (z *javaZone) offset(date Millis) int {
	t := time.UnixMilli(int64(date)).In(z.loc)
	if z.beforeHistory(t) {
		return z.raw
	}
	_, off := t.Zone()
	return off * 1000
}

func (z *javaZone) beforeHistory(t time.Time) bool {
	if !z.hasTransitions {
		return false
	}
	start, _ := t.ZoneBounds()
	return start.IsZero()
}

// inDST reports whether the saving part of the offset is non zero, which is how SimpleDateFormat
// decides between the standard and the daylight name of a zone.
func (z *javaZone) inDST(date Millis) bool {
	t := time.UnixMilli(int64(date)).In(z.loc)
	return !z.beforeHistory(t) && t.IsDST()
}

// offsetForLocal is the offset to subtract from a local time to get an instant, the direction
// java.util.Calendar needs when it turns parsed fields into a Date. The standard offset is used
// for the first guess, then the result is checked once, as ZoneInfo does.
func (z *javaZone) offsetForLocal(local int64) int {
	off := z.offset(Millis(local - int64(z.raw)))
	if o2 := z.offset(Millis(local - int64(off))); o2 != off {
		off = o2
	}
	return off
}

// displayName is TimeZone.getDisplayName(daylight, style, locale). A zone with no name in the
// locale data shows up as the custom id built from its offset, and a zone whose id already is
// such an offset shows that id.
func (z *javaZone) displayName(localeTag string, daylight, long bool) string {
	idx := 0
	if daylight {
		idx = 2
	}
	if long {
		idx++
	}
	if byLocale, ok := zoneDisplayNames[z.id]; ok {
		if names, ok := byLocale[localeTag]; ok {
			return names[idx]
		}
		if names, ok := byLocale["und"]; ok {
			return names[idx]
		}
	}
	if len(z.id) > 3 && strings.HasPrefix(z.id, "GMT") && (z.id[3] == '+' || z.id[3] == '-') {
		return z.id
	}
	off := z.raw
	if daylight {
		off += z.savings
	}
	return customID(off)
}

// customID is sun.util.calendar.ZoneInfoFile.toCustomID: "GMT+hh:mm", the name Java gives an
// offset with no zone behind it.
func customID(offset int) string {
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	minutes := offset / 60000
	return fmt.Sprintf("GMT%s%02d:%02d", sign, minutes/60, minutes%60)
}

// ZoneOffsetMillis is the offset of a zone at an instant, as java.util.TimeZone.getOffset returns
// it. An id Java does not know behaves as GMT, so the offset is 0 and no error is reported: use
// ZoneExists to find out whether the id meant anything.
func ZoneOffsetMillis(zone string, date Millis) (int, error) {
	return resolveZone(zone).offset(date), nil
}

// ZoneExists reports whether java.util.TimeZone.getTimeZone would recognise the id, which is
// decided by the id list of the reference JDK plus the custom "GMT+hh:mm" syntax.
func ZoneExists(zone string) bool {
	if zoneIDSet[zone] {
		return true
	}
	if strings.HasPrefix(zone, "GMT") {
		_, _, ok := parseCustomZone(zone)
		return ok
	}
	return false
}
