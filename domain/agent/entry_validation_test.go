package agent

import "testing"

func TestMetadataValueMissing(t *testing.T) {
	metadata := map[string]any{"escola": "Prisma", "turma": "  ", "vagas": 0}
	cases := []struct {
		key  string
		want bool
	}{
		{key: "escola", want: false},
		{key: "turma", want: true},
		{key: "vagas", want: false},
		{key: "cidade", want: true},
	}
	for _, tc := range cases {
		if got := MetadataValueMissing(metadata, tc.key); got != tc.want {
			t.Fatalf("MetadataValueMissing(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}
