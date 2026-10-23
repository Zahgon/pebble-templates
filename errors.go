/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package pebble

import "fmt"

// Error is the Go form of io.pebbletemplates.pebble.error.PebbleException and its subclasses: Kind
// is the simple class name the reference command prints, Message the message without the location,
// and Error() the Java getMessage(), "<message> (<file>:<line>)".
type Error struct {
	Kind      string // "PebbleException", "ParserException", "LoaderException", ...
	Message   string
	File      string // "" prints as "?"
	Line      int    // 0 prints as "?"
	NoLine    bool   // true when the exception carries no line at all (LoaderException)
	Attribute string // AttributeNotFoundException only
	Cause     error  // printed by the command as "caused by: <Kind>: <message>"
}

func (e *Error) Error() string {
	file, line := e.File, fmt.Sprint(e.Line)
	if file == "" {
		file = "?"
	}
	if e.NoLine || e.Line == 0 {
		line = "?"
	}
	return fmt.Sprintf("%s (%s:%s)", e.Message, file, line)
}

func (e *Error) Unwrap() error { return e.Cause }

// JavaError is a non-Pebble Java exception the reference command prints, either as the top-level
// failure or in the "caused by:" chain (NumberFormatException, ParseException, ...).
type JavaError struct {
	Kind    string
	Message string
	Cause   error
}

func (e *JavaError) Error() string { return e.Message }
func (e *JavaError) Unwrap() error { return e.Cause }

// NewError builds a PebbleException.
func NewError(message, file string, line int) *Error {
	return &Error{Kind: "PebbleException", Message: message, File: file, Line: line}
}

// NewParserError builds a ParserException.
func NewParserError(message, file string, line int) *Error {
	return &Error{Kind: "ParserException", Message: message, File: file, Line: line}
}

// NewLoaderError builds a LoaderException, which carries neither file nor line.
func NewLoaderError(message string, cause error) *Error {
	return &Error{Kind: "LoaderException", Message: message, NoLine: true, Cause: cause}
}

// NewAttributeError builds an AttributeNotFoundException.
func NewAttributeError(message, attribute, file string, line int) *Error {
	return &Error{Kind: "AttributeNotFoundException", Message: message, Attribute: attribute,
		File: file, Line: line}
}

// NewRootAttributeError builds a RootAttributeNotFoundException.
func NewRootAttributeError(message, attribute, file string, line int) *Error {
	return &Error{Kind: "RootAttributeNotFoundException", Message: message, Attribute: attribute,
		File: file, Line: line}
}
