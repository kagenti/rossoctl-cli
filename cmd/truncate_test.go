package cmd

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "short", in: "Handles orders", want: "Handles orders"},
		{name: "exactly 30", in: "123456789012345678901234567890", want: "123456789012345678901234567890"},
		{name: "31 chars", in: "1234567890123456789012345678901", want: "123456789012345678901234567..."},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.in)
			if got != tt.want {
				t.Errorf("truncate(%q) = %q (len %d), want %q", tt.in, got, len(got), tt.want)
			}
			if len(got) > 30 {
				t.Errorf("truncate(%q) length %d exceeds 30", tt.in, len(got))
			}
		})
	}
}
