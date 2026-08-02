package runtime

import "testing"

func TestSelect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested Backend
		probe     Probe
		want      Backend
		wantError bool
	}{
		{
			name:      "auto prefers managed herdr",
			requested: BackendAuto,
			probe: Probe{
				HerdrBinary:  true,
				HerdrManaged: true,
				TmuxBinary:   true,
			},
			want: BackendHerdr,
		},
		{
			name:      "auto ignores unconnected herdr and selects tmux",
			requested: BackendAuto,
			probe: Probe{
				HerdrBinary: true,
				TmuxBinary:  true,
			},
			want: BackendTmux,
		},
		{
			name:      "auto falls back to direct",
			requested: BackendAuto,
			probe:     Probe{},
			want:      BackendDirect,
		},
		{
			name:      "explicit unavailable backend fails",
			requested: BackendHerdr,
			probe:     Probe{},
			wantError: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Select(tt.requested, tt.probe)
			if tt.wantError {
				if err == nil {
					t.Fatal("Select() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Select() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Select() = %q, want %q", got, tt.want)
			}
		})
	}
}
