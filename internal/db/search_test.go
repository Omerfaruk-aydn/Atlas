package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchQuery(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, in, want string
	}{
		{"words are ANDed", "retry loop", `"retry" AND "loop"`},
		{"operators are just words", "NOT really", `"NOT" AND "really"`},
		{"quotes cannot break out", `he said "hi"`, `"he" AND "said" AND """hi"""`},
		{"a trailing star is kept", "migrat*", `"migrat"*`},
		{"a lone star is a word", "*", `"*"`},
		{"empty input matches nothing", "   ", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, MatchQuery(tc.in))
		})
	}
}
