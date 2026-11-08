package reduce

import "testing"

func TestWrapDefaultFieldNil(t *testing.T) {
	if got := WrapDefaultField(nil, "f"); got != nil {
		t.Fatalf("WrapDefaultField(nil) = %#v, want nil", got)
	}
}
