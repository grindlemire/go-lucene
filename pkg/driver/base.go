package driver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

type renderFN func(left, right string) (string, error)

func literal(left, right string) (string, error) {
	return left, nil
}

func equals(left, right string) (string, error) {
	// at this point the left is considered a column so we should treat it as such
	// and remove the quotes
	left = strings.ReplaceAll(left, "\"", "")
	return fmt.Sprintf("%s = %s", left, right), nil
}

func noop(left, right string) (string, error) {
	return left, nil
}

func like(left, right string) (string, error) {
	// at this point the left is considered a column so we should treat it as such
	// and remove the quotes
	left = strings.ReplaceAll(left, "\"", "")
	return fmt.Sprintf("%s SIMILAR TO %s", left, right), nil
}

func rang(left, right string) (string, error) {
	inclusive := true
	if right[0] == '(' && right[len(right)-1] == ')' {
		inclusive = false
	}

	stripped := right[1 : len(right)-1]
	rangeSlice := strings.Split(stripped, ",")

	if len(rangeSlice) != 2 {
		return "", fmt.Errorf("the BETWEEN operator needs a two item list in the right hand side, have %s", right)
	}

	rawMin := strings.Trim(rangeSlice[0], " ")
	rawMax := strings.Trim(rangeSlice[1], " ")

	iMin, iMax, err := toInts(rawMin, rawMax)
	if err == nil {
		if rawMin == "*" {
			if inclusive {
				return fmt.Sprintf("%s <= %d", left, iMax), nil
			}
			return fmt.Sprintf("%s < %d", left, iMax), nil
		}

		if rawMax == "*" {
			if inclusive {
				return fmt.Sprintf("%s >= %d", left, iMin), nil
			}
			return fmt.Sprintf("%s > %d", left, iMin), nil
		}

		if inclusive {
			return fmt.Sprintf("%s >= %d AND %s <= %d",
					left,
					iMin,
					left,
					iMax,
				),
				nil
		}

		return fmt.Sprintf("%s > %d AND %s < %d",
				left,
				iMin,
				left,
				iMax,
			),
			nil
	}
	fmt.Printf("HERE AS WELL: %s\n", err)

	fMin, fMax, err := toFloats(rawMin, rawMax)
	if err == nil {
		if rawMin == "*" {
			if inclusive {
				return fmt.Sprintf("%s <= %.2f", left, fMax), nil
			}
			return fmt.Sprintf("%s < %.2f", left, fMax), nil
		}

		if rawMax == "*" {
			if inclusive {
				return fmt.Sprintf("%s >= %.2f", left, fMin), nil
			}
			return fmt.Sprintf("%s > %.2f", left, fMin), nil
		}

		if inclusive {
			return fmt.Sprintf("%s >= %.2f AND %s <= %.2f",
					left,
					fMin,
					left,
					fMax,
				),
				nil
		}

		return fmt.Sprintf("%s > %.2f AND %s < %.2f",
				left,
				fMin,
				left,
				fMax,
			),
			nil
	}

	return fmt.Sprintf(`%s BETWEEN "%s" AND "%s"`,
			left,
			strings.Trim(rangeSlice[0], " "),
			strings.Trim(rangeSlice[1], " "),
		),
		nil
}

func basicCompound(op expr.Operator) renderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("(%s) %s (%s)", left, op, right), nil
	}
}

func basicWrap(op expr.Operator) renderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("%s(%s)", op, left), nil
	}
}

var shared = map[expr.Operator]renderFN{
	expr.Literal: literal,
	expr.And:     basicCompound(expr.And),
	expr.Or:      basicCompound(expr.Or),
	expr.Not:     basicWrap(expr.Not),
	expr.Equals:  equals,
	expr.Range:   rang,
	expr.Must:    noop,                // must doesn't really translate to sql
	expr.MustNot: basicWrap(expr.Not), // must not is really just a negation
	expr.Fuzzy:   noop,
	expr.Boost:   noop,
	expr.Wild:    noop, // this gets handled by the equals
	expr.Regexp:  noop, // this gets handled by the equals
	expr.Like:    like,
}

type base struct {
	renderFNs map[expr.Operator]renderFN
}

// Render will render the expression based on the renderFNs provided by the driver.
func (b base) Render(e *expr.Expression) (s string, err error) {
	if e == nil {
		return "", nil
	}

	left, err := b.serialize(e.Left)
	if err != nil {
		return s, err
	}

	right, err := b.serialize(e.Right)
	if err != nil {
		return s, err
	}

	fn, ok := b.renderFNs[e.Op]
	if !ok {
		return s, fmt.Errorf("unable to render operator [%s] - please file an issue for this", e.Op)
	}

	return fn(left, right)
}

func (b base) serialize(in any) (s string, err error) {
	if in == nil {
		return "", nil
	}

	switch v := in.(type) {
	case *expr.Expression:
		return b.Render(v)
	case *expr.RangeBoundary:
		if v.Inclusive {
			return fmt.Sprintf("[%s, %s]", v.Min, v.Max), nil
		}
		return fmt.Sprintf("(%s, %s)", v.Min, v.Max), nil

	case expr.Column:
		return fmt.Sprintf("%s", v), nil
	case string:
		return fmt.Sprintf("\"%s\"", v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func toInts(rawMin, rawMax string) (iMin, iMax int, err error) {
	iMin, err = strconv.Atoi(rawMin)
	if rawMin != "*" && err != nil {
		return 0, 0, err
	}

	iMax, err = strconv.Atoi(rawMax)
	if rawMax != "*" && err != nil {
		return 0, 0, err
	}

	return iMin, iMax, nil
}

func toFloats(rawMin, rawMax string) (fMin, fMax float64, err error) {
	fMin, err = strconv.ParseFloat(rawMin, 64)
	if rawMin != "*" && err != nil {
		return 0, 0, err
	}

	fMax, err = strconv.ParseFloat(rawMax, 64)
	if rawMax != "*" && err != nil {
		return 0, 0, err
	}

	return fMin, fMax, nil
}
