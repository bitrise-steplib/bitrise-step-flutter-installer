package main

import (
	"testing"
)

const versionMachineOut = `
{
  "frameworkVersion": "3.33.0-0.2.pre",
  "channel": "beta",
  "repositoryUrl": "https://github.com/flutter/flutter.git",
  "frameworkRevision": "1db45f74082217508069268b2f66801ca87e8a9b",
  "frameworkCommitDate": "2025-05-29 10:05:06 -0700",
  "engineRevision": "308a517184276f9526eb6026e55cfcbde1e5ad1f",
  "engineCommitDate": "2025-05-23 15:32:17 -0700",
  "dartSdkVersion": "3.9.0 (build 3.9.0-100.2.beta)",
  "devToolsVersion": "2.46.0",
  "flutterVersion": "3.33.0-0.2.pre",
  "flutterRoot": "/Users/vagrant/fvm/versions/3.33.0-0.2.pre"
}
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

const bundleURL = "https://storage.googleapis.com/flutter_infra/releases/beta/macos/flutter_macos_v1.6.3-beta.zip"

func Test_matchFlutterOutputVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    flutterVersion
		wantErr bool
	}{
		{
			name:  "normal case",
			input: versionMachineOut,
			want:  flutterVersion{version: "3.33.0-0.2.pre", channel: "beta", installType: &FlutterInstallTypeFVM},
		},
		{
			name:  "build flutter",
			input: versionOutWithBuild,
			want:  flutterVersion{version: "1.7.1-pre.49", channel: "master"},
		},
		{
			name:    "not found",
			input:   noVersion,
			want:    flutterVersion{},
			wantErr: true,
		},
		{
			name:  "bundle URL",
			input: bundleURL,
			want:  flutterVersion{version: "1.6.3", channel: "beta"},
		},
		{
			name:  "valid version and channel",
			input: "3.33.0-0.2.pre beta",
			want:  flutterVersion{version: "3.33.0-0.2.pre", channel: "beta"},
		},
		{
			name:  "valid version and channel (different order)",
			input: "beta 3.33.0-0.2.pre",
			want:  flutterVersion{version: "3.33.0-0.2.pre", channel: "beta"},
		},
		{
			name:  "missing version",
			input: "main",
			want:  flutterVersion{channel: "main"},
		},
		{
			name:  "dev channel",
			input: "dev",
			want:  flutterVersion{channel: "dev"},
		},
		{
			name:  "missing channel",
			input: "3.33.0-0.2.pre",
			want:  flutterVersion{version: "3.33.0-0.2.pre"},
		},
		{
			name:    "invalid input",
			input:   "foobar",
			want:    flutterVersion{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewFlutterVersion(tt.input)
			logger.Infof("got: %+v, err: %v", got, err)
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
