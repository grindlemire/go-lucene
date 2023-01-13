package driver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// RenderFN is a rendering function. It takes the left and right side of the operator serialized to a string
// and serializes the entire expression
type RenderFN func(left, right string) (string, error)

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

// rang is more complicated than the others because it has to handle inclusive and exclusive ranges,
// number and string ranges, and ranges that only have one bound
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

func basicCompound(op expr.Operator) RenderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("(%s) %s (%s)", left, op, right), nil
	}
}

func basicWrap(op expr.Operator) RenderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("%s(%s)", op, left), nil
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
