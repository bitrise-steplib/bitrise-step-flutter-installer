package main

import "testing"

func Test_validateFlutterURL(t *testing.T) {
	tests := []struct {
		name      string
		bundleURL string
		wantErr   bool
	}{
		{
			name:      "Previous URL style",
			bundleURL: "https://storage.googleapis.com/flutter_infra/releases/stable/macos/flutter_macos_2.0.6-stable.zip",
			wantErr:   false,
		},
		{
			name:      "New URL style",
			bundleURL: "https://storage.googleapis.com/flutter_infra_release/releases/stable/macos/flutter_macos_2.2.0-stable.zip",
			wantErr:   false,
		},
		{
			name:      "Random Host",
			bundleURL: "https://vulnerable.com/flutter_infra/releases/stable/macos/flutter_macos_2.0.6-stable.zip",
			wantErr:   true,
		},
		{
			name:      "Invalid Schema",
			bundleURL: "http://storage.googleapis.com/flutter_infra/releases/stable/macos/flutter_macos_2.0.6-stable.zip",
			wantErr:   true,
		},
		{
			name:      "Random Flutter prefix",
			bundleURL: "https://storage.googleapis.com/flutter_infra_vulnerable/releases/stable/macos/flutter_macos_2.0.6-stable.zip",
			wantErr:   true,
		},
		{
			name:      "Random URL",
			bundleURL: "https://vulnerable.com/my_flutter.zip",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateFlutterURL(tt.bundleURL); (err != nil) != tt.wantErr {
				t.Errorf("validateFlutterURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
