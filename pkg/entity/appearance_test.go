package entity

import "testing"

func TestDescribeAppearance(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		ent   *Entity
		limit int
		want  string
	}{
		{name: "neither", ent: &Entity{}, want: ""},
		{name: "baseline only", ent: &Entity{Appearance: "Silver hair."}, want: "Silver hair."},
		{name: "current only", ent: &Entity{CurrentAppearance: "Soaked."}, want: "Soaked."},
		{
			name: "layered",
			ent:  &Entity{Appearance: "Silver hair.", CurrentAppearance: "Soaked."},
			want: "Silver hair.; now Soaked.",
		},
		{
			name:  "truncated",
			ent:   &Entity{Appearance: "Silver hair.", CurrentAppearance: "Soaked."},
			limit: 10,
			want:  "Silver hai",
		},
		{name: "whitespace is trimmed", ent: &Entity{Appearance: "  Silver hair.  "}, want: "Silver hair."},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := DescribeAppearance(tc.ent, tc.limit); got != tc.want {
				t.Errorf("DescribeAppearance = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDescribeAppearanceNilEntity(t *testing.T) {
	t.Parallel()
	if got := DescribeAppearance(nil, 0); got != "" {
		t.Errorf("DescribeAppearance(nil) = %q, want empty", got)
	}
}
