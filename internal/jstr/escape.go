/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package jstr

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// hexUpper spells the hexadecimal digits the way unbescape and java.net.URLEncoder do.
const hexUpper = "0123456789ABCDEF"

// EscapeHTML is org.unbescape.html.HtmlEscape.escapeHtml4Xml, the escaper extension's "html"
// strategy. That is an HTML 4 level 1 escape: only the five markup significant characters are
// replaced, every other code point (including the C0 and C1 controls and all non-ASCII) is
// copied through unchanged. HTML 4 has no &apos; entity, so the apostrophe falls back to its
// decimal character reference.
func EscapeHTML(s string) string {
	if !strings.ContainsAny(s, "\"&'<>") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			b.WriteString("&quot;")
		case '&':
			b.WriteString("&amp;")
		case '\'':
			b.WriteString("&#39;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// jsSingleEscape holds the single character escape sequences of
// JavaScriptEscapeType.SINGLE_ESCAPE_CHARS_DEFAULT_TO_XHEXA. U+000B is absent on purpose:
// unbescape spells it \x0B rather than \v, because \v is not an ECMAScript 3 escape.
var jsSingleEscape = map[rune]string{
	0x00: `\0`, 0x08: `\b`, 0x09: `\t`, 0x0A: `\n`, 0x0C: `\f`, 0x0D: `\r`,
	'"': `\"`, '\'': `\'`, '\\': `\\`,
}

// EscapeJS is org.unbescape.javascript.JavaScriptEscape.escapeJavaScript, the escaper
// extension's "js" strategy: a level 2 escape (every non-ASCII code point plus the basic escape
// set) with the single character escapes preferred and \xHH as the fallback.
//
// The escape is context free: unlike a hand written JavaScript escaper, unbescape emits \0 even
// when a digit follows (so "\u00001" becomes `\01`), and it leaves '<', '>' and '/' bare while
// escaping '&' as \x26.
func EscapeJS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x20 && r < 0x7F && r != '"' && r != '&' && r != '\'' && r != '\\' {
			b.WriteRune(r)
			continue
		}
		if esc, ok := jsSingleEscape[r]; ok {
			b.WriteString(esc)
			continue
		}
		switch {
		case r <= 0xFF:
			b.WriteString(`\x`)
			b.WriteByte(hexUpper[r>>4])
			b.WriteByte(hexUpper[r&0xF])
		case r <= 0xFFFF:
			writeJSUnit(&b, uint16(r))
		default:
			// Java strings are UTF-16, so unbescape escapes an astral code point as the two
			// units of its surrogate pair.
			hi, lo := utf16.EncodeRune(r)
			writeJSUnit(&b, uint16(hi))
			writeJSUnit(&b, uint16(lo))
		}
	}
	return b.String()
}

// writeJSUnit appends one \uHHHH escape for a UTF-16 code unit.
func writeJSUnit(b *strings.Builder, u uint16) {
	b.WriteString(`\u`)
	b.WriteByte(hexUpper[(u>>12)&0xF])
	b.WriteByte(hexUpper[(u>>8)&0xF])
	b.WriteByte(hexUpper[(u>>4)&0xF])
	b.WriteByte(hexUpper[u&0xF])
}

// cssBackslashEscaped reports whether unbescape spells r as a backslash followed by r itself.
// That is every printable ASCII non-alphanumeric except '-' (which is legal in an identifier)
// and ':' (which unbescape always writes as a hexadecimal escape).
func cssBackslashEscaped(r rune) bool {
	switch {
	case r >= 0x20 && r <= 0x2F:
		return r != '-'
	case r >= 0x3B && r <= 0x40:
		return true
	case r >= 0x5B && r <= 0x60:
		return true
	case r >= 0x7B && r <= 0x7E:
		return true
	}
	return false
}

// cssHexDigit reports whether r would be read as part of a preceding CSS hexadecimal escape.
func cssHexDigit(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}

// EscapeCSS is org.unbescape.css.CssEscape.escapeCssIdentifier, the escaper extension's "css"
// strategy: a level 2 escape (every non-alphanumeric code point) of a CSS identifier.
//
// Three rules are positional, because a CSS identifier may not begin with a digit nor with a
// hyphen that is followed by a digit or a second hyphen:
//
//   - a digit is escaped only at the start of the string ("0a" -> `\30 a`, "a0" -> "a0");
//   - a leading '-' is escaped only when a digit or another '-' follows ("-9" -> `\-9`);
//   - a hexadecimal escape is followed by a space whenever the next character is emitted
//     literally and would otherwise be read as another hexadecimal digit ("ÿa" ->
//     `\FF a`, "ÿg" -> `\FFg`).
func EscapeCSS(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range runes {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			if i == 0 {
				writeCSSHex(&b, r, runes, i)
			} else {
				b.WriteRune(r)
			}
		case r == '-':
			next := rune(-1)
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			if i == 0 && (next == '-' || (next >= '0' && next <= '9')) {
				b.WriteString(`\-`)
			} else {
				b.WriteRune('-')
			}
		case cssBackslashEscaped(r):
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			writeCSSHex(&b, r, runes, i)
		}
	}
	return b.String()
}

// writeCSSHex appends `\` plus the uppercase hexadecimal code point with no zero padding, plus
// the disambiguating space when the following character is a literal hexadecimal digit. A digit
// can only ever be escaped at index 0, so a digit following an escape is always literal.
func writeCSSHex(b *strings.Builder, r rune, runes []rune, i int) {
	b.WriteByte('\\')
	b.WriteString(fmt.Sprintf("%X", r))
	if i+1 < len(runes) && cssHexDigit(runes[i+1]) {
		b.WriteByte(' ')
	}
}

// uriQueryParamSafe is the set of characters org.unbescape.uri leaves bare in a URI query
// parameter: the unreserved characters plus the sub-delimiters that cannot be confused with the
// query separators, so '&', '=', '+' and '#' are escaped while '\'', '/' and '*' are not.
var uriQueryParamSafe = buildSafeSet("!$'()*,-./0123456789:;?@" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz~")

// urlEncoderSafe is the set java.net.URLEncoder leaves bare (its "dontNeedEncoding" minus the
// space, which it turns into '+').
var urlEncoderSafe = buildSafeSet("*-._0123456789" +
	"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

// buildSafeSet turns a set of ASCII characters into a lookup table.
func buildSafeSet(chars string) [128]bool {
	var set [128]bool
	for i := 0; i < len(chars); i++ {
		set[chars[i]] = true
	}
	return set
}

// EscapeURLParam is org.unbescape.uri.UriEscape.escapeUriQueryParam, the escaper extension's
// "url_param" strategy: every byte of the UTF-8 encoding outside the safe set becomes %XX with
// uppercase hexadecimal. Note that this is not java.net.URLEncoder: a space becomes %20 rather
// than '+', and '\'' and '/' are left bare. URLEncode is the URLEncoder based filter.
func EscapeURLParam(s string) string {
	return percentEncode(s, &uriQueryParamSafe, false)
}

// URLEncode is java.net.URLEncoder.encode(s, "UTF-8"), the urlencode filter: a space becomes
// '+', '*', '-', '_' and '.' stay, and every other byte of the UTF-8 encoding becomes %XX with
// uppercase hexadecimal.
func URLEncode(s string) string {
	return percentEncode(s, &urlEncoderSafe, true)
}

// percentEncode percent encodes the UTF-8 bytes of s that are not in safe, optionally mapping
// the space to '+'.
func percentEncode(s string, safe *[128]bool, plusForSpace bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c < 128 && safe[c]:
			b.WriteByte(c)
		case plusForSpace && c == ' ':
			b.WriteByte('+')
		default:
			b.WriteByte('%')
			b.WriteByte(hexUpper[c>>4])
			b.WriteByte(hexUpper[c&0xF])
		}
	}
	return b.String()
}

// Base64Encode is java.util.Base64.getEncoder().encodeToString of the UTF-8 bytes, as the
// base64encode filter does it: the basic alphabet, padded, no line breaks.
func Base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// Base64Decode is java.util.Base64.getDecoder().decode of the UTF-8 bytes, as the base64decode
// filter does it, followed by new String(bytes, UTF_8). The decoder is the strict basic one: it
// rejects any character outside the alphabet (including the line breaks the MIME decoder would
// skip) and reports the JDK's IllegalArgumentException texts.
func Base64Decode(s string) (string, error) {
	out, err := base64Decode([]byte(s))
	if err != nil {
		return "", err
	}
	return decodeUTF8(out), nil
}

// base64FromChar maps a byte to its base64 value, -1 for an illegal character and -2 for the
// padding character, exactly as the JDK's fromBase64 table does.
func base64FromChar(c byte) int {
	switch {
	case c >= 'A' && c <= 'Z':
		return int(c - 'A')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 26
	case c >= '0' && c <= '9':
		return int(c-'0') + 52
	case c == '+':
		return 62
	case c == '/':
		return 63
	case c == '=':
		return -2
	}
	return -1
}

// base64Decode reproduces java.util.Base64.Decoder.decode: the length pre-check of outLength
// followed by the decode0 state machine, so that every error path keeps the JDK's wording.
func base64Decode(src []byte) ([]byte, error) {
	if len(src) == 0 {
		return nil, nil
	}
	// outLength rejects a one character input before looking at it at all.
	if len(src) < 2 {
		return nil, errors.New("Input byte[] should at least have 2 bytes for base64 bytes")
	}
	var dst []byte
	bits, shiftto, sp := 0, 18, 0
	for sp < len(src) {
		c := src[sp]
		sp++
		v := base64FromChar(c)
		if v < 0 {
			if v == -2 { // the padding character
				if (shiftto == 6 && (sp == len(src) || src[sp] != '=')) || shiftto == 18 {
					return nil, errors.New("Input byte array has wrong 4-byte ending unit")
				}
				if shiftto == 6 {
					sp++
				}
				break
			}
			// The JDK prints the signed byte, so a byte above 0x7F shows a minus sign.
			return nil, fmt.Errorf("Illegal base64 character %s", signedHex(c))
		}
		bits |= v << shiftto
		shiftto -= 6
		if shiftto < 0 {
			dst = append(dst, byte(bits>>16), byte(bits>>8), byte(bits))
			shiftto, bits = 18, 0
		}
	}
	switch shiftto {
	case 6:
		dst = append(dst, byte(bits>>16))
	case 0:
		dst = append(dst, byte(bits>>16), byte(bits>>8))
	case 12:
		return nil, errors.New("Last unit does not have enough valid bits")
	}
	if sp < len(src) {
		return nil, fmt.Errorf("Input byte array has incorrect ending byte at %d", sp)
	}
	return dst, nil
}

// signedHex is Integer.toString((byte) c, 16): the byte is sign extended first, so 0xC3 prints
// as "-3d".
func signedHex(c byte) string {
	if c < 0x80 {
		return fmt.Sprintf("%x", c)
	}
	return fmt.Sprintf("-%x", 0x100-int(c))
}

// decodeUTF8 is new String(bytes, StandardCharsets.UTF_8): every malformed sequence is replaced
// by a single U+FFFD per maximal subpart, which is what Java's CharsetDecoder does with
// CodingErrorAction.REPLACE.
func decodeUTF8(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var out strings.Builder
	out.Grow(len(b))
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			out.WriteRune(utf8.RuneError)
			i += maximalSubpart(b[i:])
			continue
		}
		out.WriteRune(r)
		i += size
	}
	return out.String()
}

// maximalSubpart returns the length of the maximal subpart of an ill-formed UTF-8 sequence at
// the start of b, which is the number of bytes Java consumes per replacement character.
func maximalSubpart(b []byte) int {
	c := b[0]
	var want int
	switch {
	case c >= 0xC2 && c <= 0xDF:
		want = 2
	case c >= 0xE0 && c <= 0xEF:
		want = 3
	case c >= 0xF0 && c <= 0xF4:
		want = 4
	default:
		return 1
	}
	n := 1
	for n < want && n < len(b) {
		// Only the continuation bytes that are legal for this prefix extend the subpart.
		lo, hi := byte(0x80), byte(0xBF)
		if n == 1 {
			switch c {
			case 0xE0:
				lo = 0xA0
			case 0xED:
				hi = 0x9F
			case 0xF0:
				lo = 0x90
			case 0xF4:
				hi = 0x8F
			}
		}
		if b[n] < lo || b[n] > hi {
			break
		}
		n++
	}
	return n
}

// Sha256Hex is the sha256 filter's output: the SHA-256 digest of the UTF-8 bytes rendered with
// Integer.toHexString per byte and zero padded to two lowercase hexadecimal digits.
func Sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	out := make([]byte, 0, 2*len(sum))
	const hexLower = "0123456789abcdef"
	for _, b := range sum {
		out = append(out, hexLower[b>>4], hexLower[b&0xF])
	}
	return string(out)
}
