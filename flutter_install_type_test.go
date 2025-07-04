package main

import "testing"

func Test_fvmParseVersionAndFeatures(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		after3_0_0 bool
		after3_2_1 bool
		after3_1_0 bool
		wantErr    bool
	}{
		{
			name:  "Legacy case",
			input: "2.0.6",
		},
		{
			name:  "Before setup change",
			input: "2.23.2",
		},
		{
			name:       "On setup change",
			input:      "3.0.0",
			after3_0_0: true,
		},
		{
			name:       "Before API release",
			input:      "3.0.19",
			after3_0_0: true,
		},
		{
			name:       "On API release",
			input:      "3.1.0",
			after3_0_0: true,
			after3_1_0: true,
		},
		{
			name:       "Before skip input flag worked",
			input:      "3.2.0",
			after3_0_0: true,
			after3_1_0: true,
			after3_2_1: false,
		},
		{
			name:       "On skip input flag working",
			input:      "3.2.1",
			after3_0_0: true,
			after3_1_0: true,
			after3_2_1: true,
		},
		{
			name:       "v prefix",
			input:      "v3.3.3",
			after3_0_0: true,
			after3_1_0: true,
			after3_2_1: true,
		},
		{
			name:       "Long version",
			input:      "13.172.76",
			after3_0_0: true,
			after3_1_0: true,
			after3_2_1: true,
		},
		{
			name:       "fvm and flutter version",
			input:      "fvm 3.1.6 with flutter 2.1.3",
			after3_0_0: true,
			after3_1_0: true,
		},
		{
			name:    "Not found",
			input:   "fvm version 3.2",
			wantErr: true,
		},
		{
			name:    "Incorrect version format",
			input:   "fvm version 3.b.6",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			after3_0_0, after3_1_0, after3_2_1, err := fvmParseVersionAndFeatures(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("fvmParseVersionAndFeatures error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if after3_0_0 != tt.after3_0_0 {
				t.Errorf("fvmParseVersionAndFeatures useSetup = %v, want %v", after3_0_0, tt.after3_0_0)
			}
			if after3_2_1 != tt.after3_2_1 {
				t.Errorf("fvmParseVersionAndFeatures useSkipInputFlag = %v, want %v", after3_2_1, tt.after3_2_1)
			}
			if after3_1_0 != tt.after3_1_0 {
				t.Errorf("fvmParseVersionAndFeatures useAPI = %v, want %v", after3_1_0, tt.after3_1_0)
			}
		})
	}
}
func Test_fvmCreateVersionString(t *testing.T) {
	tests := []struct {
		name     string
		input    flutterVersion
		expected string
		wantErr  bool
	}{
		{
			name:     "Version only",
			input:    flutterVersion{version: "13.172.76", channel: ""},
			expected: "13.172.76",
		},
		{
			name:     "No input",
			input:    flutterVersion{version: "", channel: ""},
			expected: "stable",
		},
		{
			name:     "Channel only",
			input:    flutterVersion{version: "", channel: "dev"},
			expected: "dev",
		},
		{
			name:     "Version and channel",
			input:    flutterVersion{version: "13.172.76", channel: "beta"},
			expected: "13.172.76@beta",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fvmCreateVersionString(tt.input)
			if result != tt.expected {
				t.Errorf("fvmCreateVersionString() got: %s expected: %s", result, tt.expected)
				return
			}
		})
	}
}

func Test_asdfCreateVersionString(t *testing.T) {
	tests := []struct {
		name     string
		input    flutterVersion
		expected string
		wantErr  bool
	}{
		{
			name:     "Version only",
			input:    flutterVersion{version: "13.172.76", channel: ""},
			expected: "13.172.76-stable",
		},
		{
			name:     "No input",
			input:    flutterVersion{version: "", channel: ""},
			expected: "latest",
		},
		{
			name:     "Channel only",
			input:    flutterVersion{version: "", channel: "dev"},
			expected: "latest",
		},
		{
			name:     "Version and channel",
			input:    flutterVersion{version: "13.172.76", channel: "beta"},
			expected: "13.172.76-beta",
		},
		{
			name:     "Channel included in version",
			input:    flutterVersion{version: "1.6.3-beta", channel: "stable"},
			expected: "1.6.3-beta",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := asdfCreateVersionString(tt.input)
			if result != tt.expected {
				t.Errorf("asdfCreateVersionString() got: %s expected: %s", result, tt.expected)
				return
			}
		})
	}
}
