/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jnum

import "strings"

// symbols is a java.text.DecimalFormatSymbols: the locale dependent characters and strings that
// java.text.DecimalFormat writes. Every field below was read out of the JDK the reference command
// runs on (DecimalFormatSymbols.getInstance(locale)) rather than guessed, because CLDR disagrees
// with the obvious choice more often than not: fr-FR groups with a narrow no-break space
// (U+202F), sv-SE's minus sign is U+2212 and its exponent separator is "x10^".
type symbols struct {
	decimal  rune   // getDecimalSeparator
	grouping rune   // getGroupingSeparator
	zero     rune   // getZeroDigit
	minus    string // getMinusSignText
	percent  string // getPercentText
	perMille string // getPerMillText
	infinity string // getInfinity
	nan      string // getNaN
	exponent string // getExponentSeparator
	currency string // getCurrencySymbol
}

// The JDK's data for the locales the port supports. Every one of them reports the number pattern
// "#,##0.###" with grouping size 3 and HALF_EVEN rounding, so NumberFormat.getInstance only
// differs between locales in the symbols below.
var (
	symRoot = symbols{'.', ',', '0', "-", "%", "‰", "∞", "NaN", "E", "¤"}

	localeSymbols = map[string]symbols{
		"en-US": {'.', ',', '0', "-", "%", "‰", "∞", "NaN", "E", "$"},
		"en-GB": {'.', ',', '0', "-", "%", "‰", "∞", "NaN", "E", "£"},
		"de-DE": {',', '.', '0', "-", "%", "‰", "∞", "NaN", "E", "€"},
		"fr-FR": {',', ' ', '0', "-", "%", "‰", "∞", "NaN", "E", "€"},
		"tr-TR": {',', '.', '0', "-", "%", "‰", "∞", "NaN", "E", "₺"},
		"ja-JP": {'.', ',', '0', "-", "%", "‰", "∞", "NaN", "E", "￥"},
		"es-ES": {',', '.', '0', "-", "%", "‰", "∞", "NaN", "E", "€"},
		"it-IT": {',', '.', '0', "-", "%", "‰", "∞", "NaN", "E", "€"},
		"nl-NL": {',', '.', '0', "-", "%", "‰", "∞", "NaN", "E", "€"},
		"pt-BR": {',', '.', '0', "-", "%", "‰", "∞", "NaN", "E", "R$"},
		"pl-PL": {',', ' ', '0', "-", "%", "‰", "∞", "NaN", "E", "zł"},
		"ru-RU": {',', ' ', '0', "-", "%", "‰", "∞", "не число", "E", "₽"},
		"sv-SE": {',', ' ', '0', "−", "%", "‰", "∞", "NaN", "×10^", "kr"},
		"zh-CN": {'.', ',', '0', "-", "%", "‰", "∞", "NaN", "E", "¥"},
		"ko-KR": {'.', ',', '0', "-", "%", "‰", "∞", "NaN", "E", "₩"},
		"hi-IN": {'.', ',', '0', "-", "%", "‰", "∞", "NaN", "E", "₹"},
		"ar-EG": {'٫', '٬', '٠', "؜-", "٪؜", "؉", "∞",
			"ليس رقم", "اس", "ج.م.‏"},
	}

	// languageSymbols is the fallback the JDK applies to a language without a country: the
	// symbols of that language's default region.
	languageSymbols = map[string]string{
		"en": "en-US", "de": "de-DE", "fr": "fr-FR", "tr": "tr-TR", "ja": "ja-JP",
		"es": "es-ES", "it": "it-IT", "nl": "nl-NL", "pt": "pt-BR", "pl": "pl-PL",
		"ru": "ru-RU", "sv": "sv-SE", "zh": "zh-CN", "ko": "ko-KR", "hi": "hi-IN",
		"ar": "ar-EG",
	}
)

// symbolsFor looks up the symbols of a locale given as a language tag ("de-DE"), accepting the
// Java Locale.toString spelling ("de_DE") as well. An unknown locale falls back to the symbols of
// the language, and then to the root locale's, which are the en-US ones.
func symbolsFor(locale string) *symbols {
	tag := strings.ReplaceAll(locale, "_", "-")
	if s, ok := localeSymbols[tag]; ok {
		return &s
	}
	lang := tag
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		lang = tag[:i]
	}
	lang = strings.ToLower(lang)
	if t, ok := languageSymbols[lang]; ok {
		s := localeSymbols[t]
		return &s
	}
	s := symRoot
	return &s
}
