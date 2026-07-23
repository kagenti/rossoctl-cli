package cmd

import (
	"testing"

	"github.com/rossoctl/rossoctl-cli/internal/apiclient"
)

func TestParseServicePorts(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []apiclient.CreateServicePort
	}{
		{
			name: "default",
			in:   defaultToolPorts,
			want: []apiclient.CreateServicePort{{Name: "http", Port: 9090, TargetPort: 9090, Protocol: "TCP"}},
		},
		{
			name: "bare port",
			in:   []string{"8080"},
			want: []apiclient.CreateServicePort{{Name: "http", Port: 8080, TargetPort: 8080, Protocol: "TCP"}},
		},
		{
			name: "name and port (targetPort defaults to port, TCP)",
			in:   []string{"grpc:9000"},
			want: []apiclient.CreateServicePort{{Name: "grpc", Port: 9000, TargetPort: 9000, Protocol: "TCP"}},
		},
		{
			name: "name port targetPort",
			in:   []string{"grpc:9000:9001"},
			want: []apiclient.CreateServicePort{{Name: "grpc", Port: 9000, TargetPort: 9001, Protocol: "TCP"}},
		},
		{
			name: "full spec with protocol",
			in:   []string{"dns:53:53:UDP"},
			want: []apiclient.CreateServicePort{{Name: "dns", Port: 53, TargetPort: 53, Protocol: "UDP"}},
		},
		{
			name: "multiple",
			in:   []string{"http:9090:9090:TCP", "8080"},
			want: []apiclient.CreateServicePort{
				{Name: "http", Port: 9090, TargetPort: 9090, Protocol: "TCP"},
				{Name: "http", Port: 8080, TargetPort: 8080, Protocol: "TCP"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseServicePorts(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d ports, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("port[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseServicePortsErrors(t *testing.T) {
	bad := [][]string{
		{"notaport"},           // non-integer bare port
		{"name:notaport"},      // non-integer port
		{"name:9000:notaport"}, // non-integer targetPort
		{":9000"},              // empty name
		{"a:1:2:3:4"},          // too many fields
	}
	for _, in := range bad {
		if _, err := parseServicePorts(in); err == nil {
			t.Errorf("parseServicePorts(%q) should error", in)
		}
	}
}
