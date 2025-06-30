package main

import "testing"

func Test_fvmInvestigateOutput(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		useSetupFlag     bool
		useSkipInputFlag bool
		useAPI           bool
		wantErr          bool
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
			name:         "On setup change",
			input:        "3.0.0",
			useSetupFlag: true,
		},
		{
			name:         "Before API release",
			input:        "3.0.19",
			useSetupFlag: true,
		},
		{
			name:         "On API release",
			input:        "3.1.0",
			useSetupFlag: true,
			useAPI:       true,
		},
		{
			name:             "Before skip input flag worked",
			input:            "3.2.0",
			useSetupFlag:     true,
			useAPI:           true,
			useSkipInputFlag: false,
		},
		{
			name:             "On skip input flag working",
			input:            "3.2.1",
			useSetupFlag:     true,
			useAPI:           true,
			useSkipInputFlag: true,
		},
		{
			name:             "v prefix",
			input:            "v3.3.3",
			useSetupFlag:     true,
			useAPI:           true,
			useSkipInputFlag: true,
		},
		{
			name:             "Long version",
			input:            "13.172.76",
			useSetupFlag:     true,
			useAPI:           true,
			useSkipInputFlag: true,
		},
		{
			name:         "fvm and flutter version",
			input:        "fvm 3.1.6 with flutter 2.1.3",
			useSetupFlag: true,
			useAPI:       true,
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
			useSetup, useInput, useAPI, err := fvmParseVersionAndFeatures(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("matchVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if useSetup != tt.useSetupFlag {
				t.Errorf("matchVersion() useSetup = %v, want %v", useSetup, tt.useSetupFlag)
			}
			if useInput != tt.useSkipInputFlag {
				t.Errorf("matchVersion() useSkipInputFlag = %v, want %v", useInput, tt.useSkipInputFlag)
			}
			if useAPI != tt.useAPI {
				t.Errorf("matchVersion() useAPI = %v, want %v", useAPI, tt.useAPI)
			}
		})
	}
}
