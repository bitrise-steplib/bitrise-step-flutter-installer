package main

import (
	"testing"
)

const versionOut = `
Flutter 1.7.1-pre.49 • channel master • https://github.com/flutter/flutter.git
Framework • revision 6d554827b6 (80 minutes ago) • 2019-06-03 22:00:45 -0700
Engine • revision 606a8ede2c
Tools • Dart 2.3.2 (build 2.3.2-dev.0.0 5b72293f49)
`

const versionOutWithBuild = `
Downloading Dart SDK from Flutter engine 606a8ede2c3e73e904413d5590feb3618933c161...
% Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
								Dload  Upload   Total   Spent    Left  Speed
100  121M  100  121M    0     0  10.3M      0  0:00:11  0:00:11 --:--:-- 11.1M
Building flutter tool...
╔════════════════════════════════════════════════════════════════════════════╗
║ A new version of Flutter is available!                                     ║
║                                                                            ║
║ To update to the latest version, run "flutter upgrade".                    ║
╚════════════════════════════════════════════════════════════════════════════╝


Flutter 1.7.1-pre.49 • channel master • https://github.com/flutter/flutter.git
Framework • revision 6d554827b6 (80 minutes ago) • 2019-06-03 22:00:45 -0700
Engine • revision 606a8ede2c
Tools • Dart 2.3.2 (build 2.3.2-dev.0.0 5b72293f49)
`

const noVersion = `
https://github.com/flutter/flutter.git
Framework • revision 6d554827b6 (80 minutes ago) • 2019-06-03 22:00:45 -0700
Engine • revision 606a8ede2c
Tools • Dart 2.3.2 (build 2.3.2-dev.0.0 5b72293f49)
`

func Test_matchVersion(t *testing.T) {
	tests := []struct {
		name          string
		versionOutput string
		want          string
		wantErr       bool
	}{
		{
			name:          "normal case",
			versionOutput: versionOut,
			want:          "1.7.1-pre.49",
		},
		{
			name:          "build flutter",
			versionOutput: versionOutWithBuild,
			want:          "1.7.1-pre.49",
		},
		{
			name:          "not found",
			versionOutput: noVersion,
			want:          "",
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchVersion(tt.versionOutput)
			if (err != nil) != tt.wantErr {
				t.Errorf("matchVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("matchVersion() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_matchChannel(t *testing.T) {
	tests := []struct {
		name          string
		versionOutput string
		want          string
		wantErr       bool
	}{
		{
			name:          "normal case",
			versionOutput: versionOut,
			want:          "master",
		},
		{
			name:          "build flutter",
			versionOutput: versionOutWithBuild,
			want:          "master",
		},
		{
			name:          "not found",
			versionOutput: noVersion,
			want:          "",
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchChannel(tt.versionOutput)
			if (err != nil) != tt.wantErr {
				t.Errorf("matchChannel() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("matchChannel() = %v, want %v", got, tt.want)
			}
		})
	}
}
