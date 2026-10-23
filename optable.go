/*
 * This file is part of Pebble.
 *
 * Copyright (c) 2013 by Mitchell Bösecke
 *
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

package pebble

// OperatorType distinguishes the operators the expression parser treats specially, as
// io.pebbletemplates.pebble.operator.BinaryOperatorType does.
type OperatorType int

// The operator types.
const (
	OpNormal OperatorType = iota
	OpTest
	OpFilter
)

// BinaryOp is one entry of the binary operator table of the core extension.
type BinaryOp struct {
	Symbol     string
	Precedence int
	Type       OperatorType
	RightAssoc bool
}

// UnaryOp is one entry of the unary operator table.
type UnaryOp struct {
	Symbol     string
	Precedence int
}

// BinaryOps is the core extension's binary operator table, with the precedences of Pebble 4.1.2.
var BinaryOps = []BinaryOp{
	{"or", 10, OpNormal, false},
	{"and", 15, OpNormal, false},
	{"is", 20, OpTest, false},
	{"is not", 20, OpTest, false},
	{"contains", 20, OpNormal, false},
	{"==", 30, OpNormal, false},
	{"equals", 30, OpNormal, false},
	{"!=", 30, OpNormal, false},
	{">", 30, OpNormal, false},
	{"<", 30, OpNormal, false},
	{">=", 30, OpNormal, false},
	{"<=", 30, OpNormal, false},
	{"+", 40, OpNormal, false},
	{"-", 40, OpNormal, false},
	{"*", 60, OpNormal, false},
	{"/", 60, OpNormal, false},
	{"%", 60, OpNormal, false},
	{"|", 100, OpFilter, false},
	{"~", 110, OpNormal, false},
	{"..", 120, OpNormal, false},
}

// UnaryOps is the core extension's unary operator table.
var UnaryOps = []UnaryOp{
	{"not", 500},
	{"+", 500},
	{"-", 500},
}
