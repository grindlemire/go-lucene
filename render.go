package lucene

import "github.com/grindlemire/go-lucene/pkg/driver"

var (
	postgres = driver.NewPostgresDriver()
)

// ToPostgres is a wrapper that will render the lucene expression string as a postgres sql filter string.
func ToPostgres(in string, opts ...Opt) (string, error) {
	e, err := Parse(in, opts...)
	if err != nil {
		return "", err
	}

	return postgres.Render(e)
}

// ToParameterizedPostgres is a wrapper that will render the lucene expression string as a postgres sql filter string with parameters.
// The returned string will contain placeholders for the parameters that can be passed directly to a Query statement.
func ToParameterizedPostgres(in string, opts ...Opt) (s string, params []any, err error) {
	e, err := Parse(in, opts...)
	if err != nil {
		return "", nil, err
	}

	return postgres.RenderParam(e)
}
