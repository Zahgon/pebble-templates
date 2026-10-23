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
	"strings"
	"time"
	_ "time/tzdata" // the zone database, so the command resolves zones without the host's files
)

// zoneIDExists reports whether java.time.ZoneId.of accepts the id. It knows "Z", "UTC", "GMT", "UT" with an
// optional offset, the plain offset forms and the region ids of the zone database; unlike
// java.util.TimeZone.getTimeZone it rejects the three-letter abbreviations such as "PST" and everything else
// instead of falling back to GMT.
func zoneIDExists(id string) bool {
	switch id {
	case "Z", "UTC", "GMT", "UT":
		return true
	}
	for _, prefix := range []string{"UTC", "GMT", "UT"} {
		if strings.HasPrefix(id, prefix) {
			return offsetExists(id[len(prefix):])
		}
	}
	if strings.HasPrefix(id, "+") || strings.HasPrefix(id, "-") {
		return offsetExists(id)
	}
	if len(id) < 2 {
		return false
	}
	_, err := time.LoadLocation(id)
	return err == nil
}

// offsetExists reports whether the text is one of the offset forms ZoneOffset.of accepts: "+h", "+hh", "+hh:mm",
// "+hhmm", "+hh:mm:ss", "+hhmmss" and the negative forms, with hours up to 18.
func offsetExists(text string) bool {
	if text == "" {
		return true
	}
	if text[0] != '+' && text[0] != '-' {
		return false
	}
	digits := strings.ReplaceAll(text[1:], ":", "")
	if len(digits) != len(text)-1 && strings.Count(text[1:], ":") != len(digits)/2-1 {
		return false
	}
	switch len(digits) {
	case 1, 2, 4, 6:
	default:
		return false
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return false
		}
	}
	hours := int(digits[0] - '0')
	if len(digits) > 1 {
		hours = hours*10 + int(digits[1]-'0')
	}
	if hours > 18 {
		return false
	}
	if len(digits) >= 4 && digits[2] > '5' {
		return false
	}
	return !(len(digits) == 6 && digits[4] > '5')
}

// parseLanguageTag reads a language tag the way java.util.Locale.forLanguageTag does, as far as the command needs
// it, and returns the canonical tag used inside the engine together with the java.util.Locale.toString form that
// the locale global variable prints. An ill-formed tag gives the root locale, whose toString is empty.
func parseLanguageTag(tag string) (canonical, display string) {
	parts := strings.Split(strings.ReplaceAll(tag, "_", "-"), "-")
	language := strings.ToLower(parts[0])
	if !alpha(language) || len(language) < 2 || len(language) > 8 {
		return "", ""
	}
	var script, region string
	for _, part := range parts[1:] {
		switch {
		case len(part) == 4 && alpha(part) && script == "":
			script = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		case (len(part) == 2 && alpha(part) || len(part) == 3 && digits(part)) && region == "":
			region = strings.ToUpper(part)
		}
	}
	canonical = language
	if region != "" {
		canonical += "-" + region
	}
	display = language
	if region != "" {
		display += "_" + region
	} else if script != "" {
		display += "_"
	}
	if script != "" {
		display += "_#" + script
	}
	return canonical, display
}

func alpha(text string) bool {
	for _, c := range text {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return text != ""
}

func digits(text string) bool {
	for _, c := range text {
		if c < '0' || c > '9' {
			return false
		}
	}
	return text != ""
}
