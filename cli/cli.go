/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

// Package cli is the pebble command: it renders one template of a template directory against a JSON context, the
// way the Java reference command of this repository's port does.
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PebbleTemplates/pebble"
)

const usage = "usage: pebble [options] <template-dir> <template-name> [context.json]"

// Exit statuses of the command, as the reference command defines them.
const (
	exitOK      = 0
	exitParser  = 2
	exitLoader  = 3
	exitPebble  = 4
	exitUsage   = 64
	exitContext = 65
	exitRead    = 66
	exitOther   = 70
	exitOutput  = 74
)

// Run is the command. It writes the rendered template to out, its diagnostics to errOut, and returns the exit
// status. Whatever the template wrote before a failure stays on out.
func Run(args []string, out, errOut io.Writer) int {
	opts := &pebble.Options{NewLineTrimming: true, AutoEscaping: true, DefaultEscapingStrategy: "html",
		MaxRenderedSize: -1, Locale: "en-US", TimeZone: "UTC"}
	bigDecimals := false
	hashMaps := false
	var vars []string
	var positional []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			positional = append(positional, args[i+1:]...)
			i = len(args)
		case arg == "--strict-variables":
			opts.StrictVariables = true
		case arg == "--no-newline-trimming":
			opts.NewLineTrimming = false
		case arg == "--no-autoescaping":
			opts.AutoEscaping = false
		case arg == "--literal-decimals-as-integers":
			opts.LiteralDecimalTreatedAsInt = true
		case arg == "--literal-numbers-as-bigdecimals":
			opts.LiteralNumbersAsBigDecimals = true
		case arg == "--big-decimals":
			bigDecimals = true
		case arg == "--hash-maps":
			hashMaps = true
		case strings.HasPrefix(arg, "--escaping-strategy="):
			opts.DefaultEscapingStrategy = strings.TrimPrefix(arg, "--escaping-strategy=")
		case strings.HasPrefix(arg, "--locale="):
			opts.Locale = strings.TrimPrefix(arg, "--locale=")
		case strings.HasPrefix(arg, "--timezone="):
			opts.TimeZone = strings.TrimPrefix(arg, "--timezone=")
		case strings.HasPrefix(arg, "--max-rendered-size="):
			text := strings.TrimPrefix(arg, "--max-rendered-size=")
			size, err := strconv.ParseInt(text, 10, 32)
			if err != nil {
				fmt.Fprintln(errOut, "pebble: bad --max-rendered-size: "+text)
				return exitUsage
			}
			opts.MaxRenderedSize = int(size)
		case strings.HasPrefix(arg, "--var="):
			vars = append(vars, strings.TrimPrefix(arg, "--var="))
		case strings.HasPrefix(arg, "--"):
			fmt.Fprintln(errOut, "pebble: unknown option: "+arg)
			fmt.Fprintln(errOut, usage)
			return exitUsage
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) < 2 || len(positional) > 3 {
		fmt.Fprintln(errOut, usage)
		return exitUsage
	}

	// The reference command resolves the zone with java.time.ZoneId.of, which knows the region ids and the offset
	// forms but not the three-letter abbreviations, and then makes it the default time zone of the runtime.
	if !zoneIDExists(opts.TimeZone) {
		fmt.Fprintln(errOut, "pebble: bad --timezone: "+opts.TimeZone)
		return exitUsage
	}
	opts.Locale, opts.LocaleDisplay = parseLanguageTag(opts.Locale)

	context := &pebble.Map{Vals: map[any]any{}}
	if len(positional) == 3 {
		text, err := os.ReadFile(positional[2])
		if err != nil {
			fmt.Fprintln(errOut, "pebble: cannot read context: "+positional[2])
			return exitRead
		}
		parsed, err := newJSONReader(string(text), bigDecimals, hashMaps).parse()
		if err != nil {
			fmt.Fprintln(errOut, "context error: "+err.Error())
			return exitContext
		}
		parsedMap, ok := parsed.(*pebble.Map)
		if !ok {
			fmt.Fprintln(errOut, "context error: the context must be a JSON object")
			return exitContext
		}
		context = parsedMap
	}
	for _, variable := range vars {
		equals := strings.Index(variable, "=")
		if equals <= 0 {
			fmt.Fprintln(errOut, "pebble: bad --var: "+variable)
			return exitUsage
		}
		value, err := newJSONReader(variable[equals+1:], bigDecimals, hashMaps).parse()
		if err != nil {
			fmt.Fprintln(errOut, "context error: "+err.Error())
			return exitContext
		}
		name := variable[:equals]
		if _, seen := context.Vals[any(name)]; !seen {
			context.Keys = append(context.Keys, any(name))
		}
		context.Vals[any(name)] = value
	}

	// The loader's prefix is the directory resolved against the working directory, without normalising it: it
	// appears verbatim in the loader's error messages.
	prefix := positional[0]
	if !filepath.IsAbs(prefix) {
		if working, err := os.Getwd(); err == nil {
			prefix = filepath.Join(working, prefix)
			if strings.HasSuffix(positional[0], "/") {
				prefix += "/"
			}
		}
	}
	opts.Prefix = prefix

	buffered := bufio.NewWriterSize(out, 1<<16)
	renderErr := pebble.NewEngine(opts).Render(buffered, positional[1], context)
	if flushErr := buffered.Flush(); flushErr != nil && renderErr == nil {
		return exitOutput
	}
	if renderErr == nil {
		return exitOK
	}
	return describe(errOut, renderErr)
}

// describe prints the exception chain the way the reference command prints it and returns its exit status.
func describe(errOut io.Writer, thrown error) int {
	status := exitOther
	if pebbleErr, ok := thrown.(*pebble.Error); ok {
		switch pebbleErr.Kind {
		case "ParserException":
			status = exitParser
		case "LoaderException":
			status = exitLoader
		default:
			status = exitPebble
		}
	}
	fmt.Fprintf(errOut, "%s: %s\n", kindOf(thrown), thrown.Error())
	for cause := errors.Unwrap(thrown); cause != nil; cause = errors.Unwrap(cause) {
		fmt.Fprintf(errOut, "caused by: %s: %s\n", kindOf(cause), cause.Error())
	}
	return status
}

// kindOf is the simple name of the Java exception class the error stands for.
func kindOf(err error) string {
	switch typed := err.(type) {
	case *pebble.Error:
		return typed.Kind
	case *pebble.JavaError:
		return typed.Kind
	default:
		return "RuntimeException"
	}
}
