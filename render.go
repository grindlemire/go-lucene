package lucene

import "github.com/grindlemire/go-lucene/pkg/driver"

var (
	postgres = driver.NewPostgresDriver()
)

// ToPostgres is a helper that will render the lucene expression to a postgres sql filter.
func ToPostgres(in string) (string, error) {
	e, err := Parse(in)
	if err != nil {
		return "", err
	}

	return postgres.Render(e)
}
