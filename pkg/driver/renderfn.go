package driver

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/grindlemire/go-lucene/pkg/lucene/expr"
)

// RenderFN is a rendering function. It takes the left and right side of the operator serialized to a string
// and serializes the entire expression
type RenderFN func(left, right string) (string, error)

func literal(left, right string) (string, error) {
	if !utf8.ValidString(left) {
		return "", fmt.Errorf("literal contains invalid utf8: %q", left)
	}
	if strings.ContainsRune(left, 0) {
		return "", fmt.Errorf("literal contains null byte: %q", left)
	}

	return left, nil
}

func equals(left, right string) (string, error) {
	return fmt.Sprintf("%s = %s", left, right), nil
}

func noop(left, right string) (string, error) {
	return left, nil
}

func like(left, right string) (string, error) {
	if len(right) >= 4 && right[1] == '/' && right[len(right)-2] == '/' {
		return fmt.Sprintf("%s ~ %s", left, right), nil
	}

	right = strings.ReplaceAll(right, "*", "%")
	right = strings.ReplaceAll(right, "?", "_")
	return fmt.Sprintf("%s SIMILAR TO %s", left, right), nil
}

func likeParam(left, right string, params []any) (string, error) {
	if len(params) == 1 {
		pright := params[0].(string)
		if len(pright) >= 4 && pright[0] == '/' && pright[len(pright)-1] == '/' {
			return fmt.Sprintf("%s ~ %s", left, right), nil
		}
	}

	return fmt.Sprintf("%s SIMILAR TO %s", left, right), nil
}

func inFn(left, right string) (string, error) {
	return fmt.Sprintf("%s IN %s", left, right), nil
}

func list(left, right string) (string, error) {
	return fmt.Sprintf("(%s)", left), nil
}

func greater(left, right string) (string, error) {
	return fmt.Sprintf("%s > %s", left, right), nil
}

func less(left, right string) (string, error) {
	return fmt.Sprintf("%s < %s", left, right), nil
}

func greaterEq(left, right string) (string, error) {
	return fmt.Sprintf("%s >= %s", left, right), nil
}

func lessEq(left, right string) (string, error) {
	return fmt.Sprintf("%s <= %s", left, right), nil
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
		if rawMin == "'*'" {
			if inclusive {
				return fmt.Sprintf("%s <= %d", left, iMax), nil
			}
			return fmt.Sprintf("%s < %d", left, iMax), nil
		}

		if rawMax == "'*'" {
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
		if rawMin == "'*'" {
			if inclusive {
				return fmt.Sprintf("%s <= %.2f", left, fMax), nil
			}
			return fmt.Sprintf("%s < %.2f", left, fMax), nil
		}

		if rawMax == "'*'" {
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

	return fmt.Sprintf(`%s BETWEEN %s AND %s`,
			left,
			strings.Trim(rangeSlice[0], " "),
			strings.Trim(rangeSlice[1], " "),
		),
		nil
}

func rangParam(left, right string, params []any) (string, error) {
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

	// if we have a parameterized input then we need to check the type
	if rawMin == "?" || rawMax == "?" {
		switch params[0].(type) {
		case int, float64, float32:
			if rawMin == "'*'" {
				if inclusive {
					return fmt.Sprintf("%s <= %s", left, rawMax), nil
				}
				return fmt.Sprintf("%s < %s", left, rawMax), nil
			}

			if rawMax == "'*'" {
				if inclusive {
					return fmt.Sprintf("%s >= %s", left, rawMin), nil
				}
				return fmt.Sprintf("%s > %s", left, rawMin), nil
			}

			if inclusive {
				return fmt.Sprintf("%s >= %s AND %s <= %s",
						left,
						rawMin,
						left,
						rawMax,
					),
					nil
			}

			return fmt.Sprintf("%s > %s AND %s < %s",
					left,
					rawMin,
					left,
					rawMax,
				),
				nil
		default:
			return fmt.Sprintf(`%s BETWEEN %s AND %s`,
					left,
					strings.Trim(rangeSlice[0], " "),
					strings.Trim(rangeSlice[1], " "),
				),
				nil
		}

	}

	iMin, iMax, err := toInts(rawMin, rawMax)
	if err == nil {
		if rawMin == "'*'" {
			if inclusive {
				return fmt.Sprintf("%s <= %d", left, iMax), nil
			}
			return fmt.Sprintf("%s < %d", left, iMax), nil
		}

		if rawMax == "'*'" {
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
		if rawMin == "'*'" {
			if inclusive {
				return fmt.Sprintf("%s <= %.2f", left, fMax), nil
			}
			return fmt.Sprintf("%s < %.2f", left, fMax), nil
		}

		if rawMax == "'*'" {
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

	return fmt.Sprintf(`%s BETWEEN %s AND %s`,
			left,
			strings.Trim(rangeSlice[0], " "),
			strings.Trim(rangeSlice[1], " "),
		),
		nil
}

func basicCompound(op expr.Operator) RenderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("%s %s %s", left, op, right), nil
	}
}

func basicWrap(op expr.Operator) RenderFN {
	return func(left, right string) (string, error) {
		return fmt.Sprintf("%s(%s)", op, left), nil
	}
}

func toInts(rawMin, rawMax string) (iMin, iMax int, err error) {
	iMin, err = strconv.Atoi(rawMin)
	if rawMin != "'*'" && err != nil {
		return 0, 0, err
	}

	iMax, err = strconv.Atoi(rawMax)
	if rawMax != "'*'" && err != nil {
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
