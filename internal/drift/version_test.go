package drift

import "testing"

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    string
		wantErr bool
	}{
		{"Node.js", "v20.11.0", "20.11.0", false},
		{"Node.js no v", "20.11.0", "20.11.0", false},
		{"Python", "Python 3.12.1", "3.12.1", false},
		{"Go", "go version go1.22.0 linux/amd64", "1.22.0", false},
		{"Ripgrep", "ripgrep 13.0.0", "13.0.0", false},
		{"With prefix", "version: 2.5.3", "2.5.3", false},
		{"Multiline", "node info\nv20.11.0\nmore info", "20.11.0", false},
		{"No version", "usage: tool [options]", "", true},
		{"Empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractVersion(tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExtractVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseToolSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    ToolSpec
		wantErr bool
	}{
		{
			name: "Core tool with version",
			spec: "node@20.11.0",
			want: ToolSpec{Backend: "", Name: "node", Version: "20.11.0"},
		},
		{
			name: "Cargo backend",
			spec: "cargo:ripgrep@13.0.0",
			want: ToolSpec{Backend: "cargo", Name: "ripgrep", Version: "13.0.0"},
		},
		{
			name: "UBI backend with repo",
			spec: "ubi:sharkdp/bat@0.24.0",
			want: ToolSpec{Backend: "ubi", Name: "bat", Version: "0.24.0"},
		},
		{
			name: "No version",
			spec: "python",
			want: ToolSpec{Backend: "", Name: "python", Version: ""},
		},
		{
			name: "NPM backend",
			spec: "npm:prettier@3.0.0",
			want: ToolSpec{Backend: "npm", Name: "prettier", Version: "3.0.0"},
		},
		{
			name:    "Invalid format - empty",
			spec:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseToolSpec(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseToolSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseToolSpec() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
