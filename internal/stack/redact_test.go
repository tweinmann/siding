package stack

import (
	"reflect"
	"testing"
)

func TestDotEnvValues(t *testing.T) {
	t.Parallel()
	content := `# a comment
DB_PASSWORD=not-a-real-token
export API_KEY="not-a-real-key" # trailing comment
SINGLE='it''s'
PLAIN=value with spaces # comment
EMPTY=
NO_EQUALS
`
	want := []string{"not-a-real-token", "not-a-real-key", "it", "value with spaces"}
	if got := DotEnvValues(content); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedactor(t *testing.T) {
	t.Parallel()
	r := NewRedactor("not-a-real-token", "not-a-real", "on", "true", "3000", "abc", "development")
	tests := []struct{ in, want string }{
		{in: "connecting with not-a-real-token", want: "connecting with ***"},
		{in: "listening on 3000 in development, debug true", want: "listening on 3000 in ***, debug true"},
		{in: "prefix not-a-real only", want: "prefix *** only"},
		{in: "abc stays, it is short", want: "abc stays, it is short"},
	}
	for _, tt := range tests {
		if got := r.Line(tt.in); got != tt.want {
			t.Errorf("Line(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
