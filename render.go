package lucene

import "github.com/grindlemire/go-lucene/pkg/driver"

var (
	postgres = driver.NewPostgresDriver()
)

// ToPostgres is a wrapper that will render the lucene expression string as a postgres sql filter string.
func ToPostgres(in string, opts ...opt) (string, error) {
	e, err := Parse(in, opts...)
	if err != nil {
		return "", err
	}

	return postgres.Render(e)
}
