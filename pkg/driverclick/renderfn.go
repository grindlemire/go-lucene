package driverclick

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/AlxBystrov/go-lucene/pkg/lucene/expr"
)

// RenderFN is a rendering function. It takes the left and right side of the operator serialized to a string
// and serializes the entire expression.
type RenderFN func(b Base, left, right string) (string, error)

func literal(b Base, left, right string) (string, error) {
	if !utf8.ValidString(left) {
		return "", fmt.Errorf("literal contains invalid utf8: %q", left)
	}
	if strings.ContainsRune(left, 0) {
		return "", fmt.Errorf("literal contains null byte: %q", left)
	}
	return left, nil
}

func equals(b Base, left, right string) (string, error) {
	if left == "'_source'" {
		if strings.HasPrefix(right, "'") && strings.HasSuffix(right, "'") {
			// magic for converting into 'some text' -> '%some text%' for proper searching with like
			if len(right) > 1 && right[0] == '\'' && right[len(right)-1] == '\'' {
				right = "'%" + right[1:len(right)-1] + "%'"
			}

			return fmt.Sprintf(`lowerUTF8(_source) like lowerUTF8(%s)`, right), nil
		}
		return fmt.Sprintf("lowerUTF8(_source) like lowerUTF8('%%%s%%')", right), nil
	}

	fieldType := inferFieldType(right)
	fieldExpr, fieldType, err := b.resolveFieldExpr(left, fieldType)
	if err != nil {
		return "", err
	}

	switch fieldType {
	case NumberField, BoolField:
		return fmt.Sprintf("%s = %s", fieldExpr, right), nil
	default:
		if right == "''" {
			return fmt.Sprintf("%s = ''", fieldExpr), nil
		}
		// magic for converting into 'some text' -> '%some text%' for proper searching with like
		if len(right) > 2 && right[0] == '\'' && right[len(right)-1] == '\'' {
			right = "'%" + right[1:len(right)-1] + "%'"
		}
		return fmt.Sprintf("lowerUTF8(%s) like lowerUTF8(%s)", fieldExpr, right), nil
	}
}

func noop(b Base, left, right string) (string, error) {
	return left, nil
}

func like(b Base, left, right string) (string, error) {
	fieldExpr, _, err := b.resolveFieldExpr(left, StringField)
	if err != nil {
		return "", err
	}

	if len(right) >= 4 && right[1] == '/' && right[len(right)-2] == '/' {
		right = strings.Replace(right, "'/", "'", 1)
		right = strings.Replace(right, "/'", "'", 1)

		if left == "'_source'" {
			left = strings.Replace(left, "'", "", -1)
			return fmt.Sprintf("match(lowerUTF8(%s),lowerUTF8(%s))", left, right), nil
		}
		return fmt.Sprintf("match(lowerUTF8(%s),lowerUTF8(%s))", fieldExpr, right), nil
	}

	right = strings.ReplaceAll(right, "*", "%")
	right = strings.ReplaceAll(right, "?", "_")
	return fmt.Sprintf("lowerUTF8(%s) like lowerUTF8(%s)", fieldExpr, right), nil
}

func inFn(b Base, left, right string) (string, error) {
	fieldExpr, _, err := b.resolveFieldExpr(left, StringField)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s IN %s", fieldExpr, right), nil
}

func list(b Base, left, right string) (string, error) {
	return fmt.Sprintf("(%s)", left), nil
}

func greater(b Base, left, right string) (string, error) {
	if _, err := strconv.ParseInt(right, 0, 64); err != nil {
		return "", nil
	}
	fieldExpr, _, err := b.resolveFieldExpr(left, NumberField)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s > %s", fieldExpr, right), nil
}

func less(b Base, left, right string) (string, error) {
	if _, err := strconv.ParseInt(right, 0, 64); err != nil {
		return "", nil
	}
	fieldExpr, _, err := b.resolveFieldExpr(left, NumberField)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s < %s", fieldExpr, right), nil
}

func greaterEq(b Base, left, right string) (string, error) {
	if _, err := strconv.ParseInt(right, 0, 64); err != nil {
		return "", nil
	}
	fieldExpr, _, err := b.resolveFieldExpr(left, NumberField)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s >= %s", fieldExpr, right), nil
}

func lessEq(b Base, left, right string) (string, error) {
	if _, err := strconv.ParseInt(right, 0, 64); err != nil {
		return "", nil
	}
	fieldExpr, _, err := b.resolveFieldExpr(left, NumberField)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s <= %s", fieldExpr, right), nil
}

// rang is more complicated than the others because it has to handle inclusive and exclusive ranges,
// number and string ranges, and ranges that only have one bound.
func rang(b Base, left, right string) (string, error) {
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
		fieldExpr, _, err := b.resolveFieldExpr(left, NumberField)
		if err != nil {
			return "", err
		}

		if rawMin == "'*'" {
			if inclusive {
				return fmt.Sprintf("%s <= %d", fieldExpr, iMax), nil
			}
			return fmt.Sprintf("%s < %d", fieldExpr, iMax), nil
		}

		if rawMax == "'*'" {
			if inclusive {
				return fmt.Sprintf("%s >= %d", fieldExpr, iMin), nil
			}
			return fmt.Sprintf("%s > %d", fieldExpr, iMin), nil
		}

		if inclusive {
			return fmt.Sprintf("%s >= %d AND %s <= %d", fieldExpr, iMin, fieldExpr, iMax), nil
		}

		return fmt.Sprintf("%s > %d AND %s < %d", fieldExpr, iMin, fieldExpr, iMax), nil
	}

	fMin, fMax, err := toFloats(rawMin, rawMax)
	if err == nil {
		fieldExpr, _, err := b.resolveFieldExpr(left, NumberField)
		if err != nil {
			return "", err
		}

		if rawMin == "'*'" {
			if inclusive {
				return fmt.Sprintf("%s <= %.2f", fieldExpr, fMax), nil
			}
			return fmt.Sprintf("%s < %.2f", fieldExpr, fMax), nil
		}

		if rawMax == "'*'" {
			if inclusive {
				return fmt.Sprintf("%s >= %.2f", fieldExpr, fMin), nil
			}
			return fmt.Sprintf("%s > %.2f", fieldExpr, fMin), nil
		}

		if inclusive {
			return fmt.Sprintf("%s >= %.2f AND %s <= %.2f", fieldExpr, fMin, fieldExpr, fMax), nil
		}

		return fmt.Sprintf("%s > %.2f AND %s < %.2f", fieldExpr, fMin, fieldExpr, fMax), nil
	}

	fieldExpr, _, err := b.resolveFieldExpr(left, StringField)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`%s BETWEEN %s AND %s`, fieldExpr, rawMin, rawMax), nil
}

func basicCompound(op expr.Operator) RenderFN {
	return func(b Base, left, right string) (string, error) {
		return fmt.Sprintf("%s %s %s", left, op, right), nil
	}
}

func basicWrap(op expr.Operator) RenderFN {
	return func(b Base, left, right string) (string, error) {
		return fmt.Sprintf("%s(%s)", op, left), nil
	}
}

func inferFieldType(right string) FieldType {
	if _, err := strconv.ParseInt(right, 0, 64); err == nil {
		return NumberField
	}
	if _, err := strconv.ParseBool(right); err == nil {
		return BoolField
	}
	return StringField
}

func (b Base) resolveFieldExpr(left string, fallbackType FieldType) (string, FieldType, error) {
	fieldName, err := unquoteSQLString(left)
	if err != nil {
		return "", fallbackType, err
	}

	if fieldName == "_source" {
		return "_source", StringField, nil
	}

	if binding, ok := b.FieldBindings[fieldName]; ok {
		columnName := binding.Column
		if columnName == "" {
			columnName = fieldName
		}
		if binding.Storage == MaterializedColumn {
			columnExpr, err := quoteIdentifier(columnName)
			if err != nil {
				return "", binding.Type, err
			}
			return columnExpr, binding.Type, nil
		}
		return arrayExpr(fieldName, binding.Type), binding.Type, nil
	}

	return arrayExpr(fieldName, fallbackType), fallbackType, nil
}

func arrayExpr(fieldName string, fieldType FieldType) string {
	quoted := fmt.Sprintf("'%s'", strings.ReplaceAll(fieldName, "'", "''"))
	switch fieldType {
	case NumberField:
		return "numbers.value[indexOf(numbers.name," + quoted + ")]"
	case BoolField:
		return "bools.value[indexOf(bools.name," + quoted + ")]"
	default:
		return "strings.value[indexOf(strings.name," + quoted + ")]"
	}
}

func unquoteSQLString(in string) (string, error) {
	if len(in) < 2 || in[0] != '\'' || in[len(in)-1] != '\'' {
		return "", fmt.Errorf("expected single quoted sql string, got %q", in)
	}
	return strings.ReplaceAll(in[1:len(in)-1], "''", "'"), nil
}

func quoteIdentifier(in string) (string, error) {
	if in == "" {
		return "", fmt.Errorf("identifier is empty")
	}

	parts := strings.Split(in, ".")
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("identifier contains an empty segment: %q", in)
		}
		for i, r := range part {
			if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				continue
			}
			if i > 0 && r >= '0' && r <= '9' {
				continue
			}
			return "", fmt.Errorf("identifier contains unsupported character %q: %q", r, in)
		}
	}

	return strings.Join(parts, "."), nil
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
	if rawMin != "'*'" && err != nil {
		return 0, 0, err
	}

	fMax, err = strconv.ParseFloat(rawMax, 64)
	if rawMax != "'*'" && err != nil {
		return 0, 0, err
	}

	return fMin, fMax, nil
}
