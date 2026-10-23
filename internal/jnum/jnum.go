/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

// Package jnum reproduces the number semantics of the Java runtime that Pebble's output depends
// on: how each boxed type prints, how arithmetic promotes and rounds, java.math.BigDecimal, and
// java.text.DecimalFormat / NumberFormat.
package jnum

import (
	"math"
	"math/big"
)

// HashCode is the java.lang.Number hashCode of a boxed value (Long, Integer, Double, ...), used
// for java.util.HashMap iteration order. An unsupported type reports 0, which is the identity
// hash the port cannot reproduce anyway.
func HashCode(value any) int32 {
	switch v := value.(type) {
	case bool:
		// Boolean.hashCode
		if v {
			return 1231
		}
		return 1237
	case int64:
		// Long.hashCode folds the two halves of the long together.
		return int32(uint32(uint64(v)>>32)) ^ int32(uint32(uint64(v)&0xffffffff))
	case int32:
		return v
	case int16:
		return int32(v)
	case int8:
		return int32(v)
	case int:
		return int32(uint32(uint64(v)>>32)) ^ int32(uint32(uint64(v)&0xffffffff))
	case float64:
		// Double.hashCode works on doubleToLongBits, which collapses every NaN to one bit pattern.
		bits := math.Float64bits(v)
		if math.IsNaN(v) {
			bits = 0x7ff8000000000000
		}
		return int32(uint32(bits>>32)) ^ int32(uint32(bits&0xffffffff))
	case float32:
		// Float.hashCode is floatToIntBits itself.
		bits := math.Float32bits(v)
		if math.IsNaN(float64(v)) {
			bits = 0x7fc00000
		}
		return int32(bits)
	case *big.Int:
		return bigIntHashCode(v)
	case *BigDecimal:
		return v.HashCode()
	}
	return 0
}
