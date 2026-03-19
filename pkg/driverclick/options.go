package driverclick

import "fmt"

type FieldType string

const (
	StringField FieldType = "string"
	NumberField FieldType = "number"
	BoolField   FieldType = "bool"
)

type StorageKind string

const (
	ArrayKVStorage     StorageKind = "array_kv"
	MaterializedColumn StorageKind = "materialized_column"
)

type FieldBinding struct {
	Type    FieldType
	Storage StorageKind
	Column  string
}

type Option func(*ClickhouseDriver) error

func WithFieldBindings(bindings map[string]FieldBinding) Option {
	return func(d *ClickhouseDriver) error {
		if d.FieldBindings == nil {
			d.FieldBindings = map[string]FieldBinding{}
		}

		for field, binding := range bindings {
			if field == "" {
				return fmt.Errorf("field binding name is empty")
			}
			if binding.Type == "" {
				return fmt.Errorf("field binding type is empty for %q", field)
			}
			if binding.Storage == "" {
				binding.Storage = ArrayKVStorage
			}
			if binding.Column == "" {
				binding.Column = field
			}
			d.FieldBindings[field] = binding
		}

		return nil
	}
}
