//go:build windows

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/lxn/walk"
)

var decimalPixelNumber = regexp.MustCompile(`[0-9]+(?:\.[0-9]*)?|\.[0-9]+`)
var imageWidthReference = regexp.MustCompile(`\$[0-9]+`)

func pixelExpression(input string, current int, imageWidths ...int) (int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, nil
	}
	if len(input) > 128 {
		return 0, fmt.Errorf("expression too long")
	}
	var referenceErr error
	input = imageWidthReference.ReplaceAllStringFunc(input, func(reference string) string {
		index, err := strconv.Atoi(reference[1:])
		if err != nil || index < 1 || index > len(imageWidths) || imageWidths[index-1] <= 0 {
			referenceErr = fmt.Errorf("image width unavailable: %s", reference)
			return "0"
		}
		return "(" + strconv.Itoa(imageWidths[index-1]) + ")"
	})
	if referenceErr != nil {
		return 0, referenceErr
	}
	for _, character := range input {
		if !strings.ContainsRune("0123456789.+-*/%() \t", character) {
			return 0, fmt.Errorf("invalid character")
		}
	}
	// Pixel counts are decimal even when entered with leading zeroes.
	input = decimalPixelNumber.ReplaceAllStringFunc(input, func(number string) string {
		number = strings.TrimLeft(number, "0")
		if number == "" {
			return "0"
		}
		return number
	})
	if strings.ContainsRune("+-*/%", rune(input[0])) {
		input = strconv.Itoa(current) + input
	}
	expression, err := parser.ParseExpr(input)
	if err != nil {
		return 0, err
	}
	var evaluate func(ast.Expr) (float64, error)
	evaluate = func(expr ast.Expr) (float64, error) {
		switch e := expr.(type) {
		case *ast.BasicLit:
			if e.Kind == token.INT || e.Kind == token.FLOAT {
				return strconv.ParseFloat(e.Value, 64)
			}
		case *ast.ParenExpr:
			return evaluate(e.X)
		case *ast.UnaryExpr:
			x, err := evaluate(e.X)
			if e.Op == token.ADD {
				return x, err
			}
			if e.Op == token.SUB {
				return -x, err
			}
		case *ast.BinaryExpr:
			a, err := evaluate(e.X)
			if err != nil {
				return 0, err
			}
			b, err := evaluate(e.Y)
			if err != nil {
				return 0, err
			}
			switch e.Op {
			case token.ADD:
				return a + b, nil
			case token.SUB:
				return a - b, nil
			case token.MUL:
				return a * b, nil
			case token.QUO:
				if b != 0 {
					return a / b, nil
				}
			case token.REM:
				if b != 0 {
					return math.Mod(a, b), nil
				}
			}
		}
		return 0, fmt.Errorf("invalid calculation")
	}
	result, err := evaluate(expression)
	if err != nil || math.IsNaN(result) || math.IsInf(result, 0) || result < 0 || result > 1000000 {
		return 0, fmt.Errorf("invalid pixel size")
	}
	return int(math.Round(result)), nil
}

func pixelInputCharacter(key walk.Key, modifiers walk.Modifiers) (byte, bool) {
	if modifiers & ^(walk.ModControl|walk.ModShift) != 0 {
		return 0, false
	}
	shift := modifiers&walk.ModShift != 0
	if key >= walk.Key0 && key <= walk.Key9 {
		if !shift {
			return byte(key-walk.Key0) + '0', true
		}
		switch key {
		case walk.Key4:
			return '$', true
		case walk.Key5:
			return '%', true
		case walk.Key8:
			return '*', true
		case walk.Key9:
			return '(', true
		case walk.Key0:
			return ')', true
		}
		return 0, false
	}
	if key >= walk.KeyNumpad0 && key <= walk.KeyNumpad9 {
		return byte(key-walk.KeyNumpad0) + '0', true
	}
	switch key {
	case walk.KeyAdd, walk.KeyOEMPlus:
		return '+', true
	case walk.KeySubtract, walk.KeyOEMMinus:
		if !shift {
			return '-', true
		}
	case walk.KeyMultiply:
		return '*', true
	case walk.KeyDivide:
		return '/', true
	case walk.KeyDecimal, walk.KeyOEMPeriod:
		if !shift {
			return '.', true
		}
	case walk.KeySpace:
		return ' ', true
	case walk.Key(0xbf):
		if !shift {
			return '/', true
		} // OEM slash.
	}
	return 0, false
}

func (v *viewer) evaluatePixelExpression(input string, current int) (int, error) {
	widths := make([]int, len(v.workspace))
	for i, item := range v.workspace {
		if item.image != nil {
			widths[i] = item.image.width
		}
	}
	return pixelExpression(input, current, widths...)
}
