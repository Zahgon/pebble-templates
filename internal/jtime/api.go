/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

// Package jtime reproduces the java.text.SimpleDateFormat and java.util.TimeZone semantics the
// date filter depends on: pattern letters, lenient parsing, the Gregorian calendar and the zone
// database of the Java runtime.
//
// Parse and Format live in parse.go and format.go, ZoneOffsetMillis and ZoneExists in zone.go.
// The locale and zone tables in data.go were dumped from the reference JDK rather than written
// by hand, so the names are exactly the ones that runtime prints.
package jtime

// Millis is a java.util.Date: milliseconds since the epoch.
type Millis int64
