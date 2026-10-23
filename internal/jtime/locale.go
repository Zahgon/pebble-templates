/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jtime

import "strings"

// resolveLocale maps a locale tag onto the data this package carries. The tag may use either
// separator ("de-DE" or "de_DE"). An exact match wins; failing that the first entry with the same
// language is used (so "de-AT" is served by "de-DE", which is an approximation: CLDR gives
// Austrian German its own abbreviated month names); failing that java.util.Locale.ROOT, whose
// names are the untranslated CLDR root ones ("Jan" even for MMMM, eras "BCE"/"CE").
func resolveLocale(tag string) (string, localeData) {
	norm := normalizeTag(tag)
	if ld, ok := locales[norm]; ok {
		return norm, ld
	}
	lang := norm
	if i := strings.IndexByte(norm, '-'); i >= 0 {
		lang = norm[:i]
	}
	// Deterministic order: the preferred region for each language this package knows.
	for _, cand := range []string{"en-US", "de-DE", "fr-FR", "tr-TR", "ja-JP", "en-GB"} {
		if strings.HasPrefix(cand, lang+"-") {
			return cand, locales[cand]
		}
	}
	return "und", locales["und"]
}

// normalizeTag puts a locale tag into the language-REGION shape the tables use.
func normalizeTag(tag string) string {
	tag = strings.ReplaceAll(tag, "_", "-")
	parts := strings.Split(tag, "-")
	if parts[0] == "" {
		return "und"
	}
	out := strings.ToLower(parts[0])
	if len(parts) > 1 && len(parts[1]) == 2 {
		out += "-" + strings.ToUpper(parts[1])
	}
	return out
}
