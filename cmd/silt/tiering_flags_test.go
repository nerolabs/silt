package main

import "testing"

// D-TIERING capability axes at the daemon surface (docs/decisions.md D-TIERING §4,
// the near-term build-gated mode flags). The content-serving axis has two
// spellings — the positive `-serve-content` and the older negative `-freeload` —
// and the whole point of the positive form is that a tier profile composes
// without double negatives. These pin that the two spellings agree, that the
// legacy flag is untouched, and that a contradictory pair fails loudly instead of
// silently picking one (S3).
func TestResolveContentServing(t *testing.T) {
	cases := []struct {
		name            string
		serveContent    bool
		serveContentSet bool
		freeload        bool
		wantRefuses     bool
		wantErr         bool
	}{
		{
			name:         "default: an ordinary node serves content",
			serveContent: true, serveContentSet: false, freeload: false,
			wantRefuses: false,
		},
		{
			name:         "legacy -freeload still refuses (unchanged behavior)",
			serveContent: true, serveContentSet: false, freeload: true,
			wantRefuses: true,
		},
		{
			name:         "the positive form: -serve-content=false refuses",
			serveContent: false, serveContentSet: true, freeload: false,
			wantRefuses: true,
		},
		{
			name:         "agreeing pair: -freeload -serve-content=false",
			serveContent: false, serveContentSet: true, freeload: true,
			wantRefuses: true,
		},
		{
			name:         "CONTRADICTION: -freeload with an explicit -serve-content=true",
			serveContent: true, serveContentSet: true, freeload: true,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refuses, err := resolveContentServing(tc.serveContent, tc.serveContentSet, tc.freeload)
			if tc.wantErr {
				if err == nil {
					t.Fatal("a contradictory -freeload/-serve-content pair must be refused, not silently resolved")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if refuses != tc.wantRefuses {
				t.Fatalf("refuses = %v, want %v", refuses, tc.wantRefuses)
			}
		})
	}
}
