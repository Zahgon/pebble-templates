/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package pebble

import (
	"strconv"

	"github.com/PebbleTemplates/pebble/internal/jstr"
)

func init() {
	// MaxFunction and MinFunction declare no argument names, so their arguments arrive numbered.
	Functions["max"] = Function{Apply: maxFunction}
	Functions["min"] = Function{Apply: minFunction}
	Functions["range"] = Function{
		ArgNames: []string{"start", "end", "increment"}, Apply: rangeFunction}
}

// maxFunction is MaxFunction and minFunction MinFunction: they walk the numbered arguments and
// keep the one that wins every comparison. The comparison is OperatorUtils.gt and lt, which widen
// both operands to double and raise a bare RuntimeException for anything that is not a number;
// neither function wraps that failure in a PebbleException.
func maxFunction(args map[string]any, ctx Ctx, line int) (any, error) {
	return extremum(args, "GREATER_THAN")
}

func minFunction(args map[string]any, ctx Ctx, line int) (any, error) {
	return extremum(args, "LESS_THAN")
}

func extremum(args map[string]any, comparison string) (any, error) {
	var best any
	for i := 0; ; i++ {
		candidate, present := args[strconv.Itoa(i)]
		if !present {
			return best, nil
		}
		if best == nil {
			best = candidate
			continue
		}
		// A null candidate makes the comparison false, so it is simply skipped.
		if candidate == nil {
			continue
		}
		c, candidateNumber := javaDoubleValue(candidate)
		b, bestNumber := javaDoubleValue(best)
		if !candidateNumber || !bestNumber {
			return nil, &JavaError{Kind: "RuntimeException",
				Message: "invalid operands for mathematical comparison [" + comparison + "]"}
		}
		if comparison == "GREATER_THAN" && c > b || comparison == "LESS_THAN" && c < b {
			best = candidate
		}
	}
}

// rangeFunction is RangeFunction: it walks numbers as longs and single character strings as
// characters, and the .. operator calls it with two positional arguments.
func rangeFunction(args map[string]any, ctx Ctx, line int) (any, error) {
	start := args["start"]
	end := args["end"]

	increment := int64(1)
	if value, present := args["increment"]; present && value != nil {
		if _, isNumber := javaDoubleValue(value); !isNumber {
			return nil, NewError("The increment of the range function must be a number "+
				valueOf(value), ctx.Name(), line)
		}
		increment = javaLongValue(value)
	}

	_, startNumber := javaDoubleValue(start)
	_, endNumber := javaDoubleValue(end)
	if startNumber && endNumber {
		from, to := javaLongValue(start), javaLongValue(end)
		items := []any{}
		switch {
		case increment > 0:
			for i := from; i <= to; i += increment {
				items = append(items, i)
			}
		case increment < 0:
			for i := from; i >= to; i += increment {
				items = append(items, i)
			}
		default:
			return nil, rangeIncrementZero(ctx, line)
		}
		return &List{Items: items}, nil
	}

	startText, startIsText := start.(string)
	endText, endIsText := end.(string)
	if startIsText && endIsText {
		if jstr.Length(startText) != 1 || jstr.Length(endText) != 1 {
			return nil, rangeBadArguments(ctx, line)
		}
		from, err := jstr.CharAt(startText, 0)
		if err != nil {
			return nil, err
		}
		to, err := jstr.CharAt(endText, 0)
		if err != nil {
			return nil, err
		}
		items := []any{}
		switch {
		case increment > 0:
			for i := int64(from); i <= int64(to); i += increment {
				items = append(items, Char(uint16(i)))
			}
		case increment < 0:
			for i := int64(from); i >= int64(to); i += increment {
				items = append(items, Char(uint16(i)))
			}
		default:
			return nil, rangeIncrementZero(ctx, line)
		}
		return &List{Items: items}, nil
	}
	return nil, rangeBadArguments(ctx, line)
}

func rangeIncrementZero(ctx Ctx, line int) error {
	return NewError("The increment of the range function must be different than 0",
		ctx.Name(), line)
}

func rangeBadArguments(ctx Ctx, line int) error {
	return NewError("Arguments of range function must be of type Number or String with a length of 1",
		ctx.Name(), line)
}
