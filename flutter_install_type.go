package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-utils/v2/command"
)

const (
	FVMName              = "fvm"
	ASDFName             = "asdf"
	ManualName           = "manual"
	FVMCacheVersionsPath = "/fvm/versions"
	FVMCacheDefaultPath  = "/fvm/default/bin/flutter"
	ASDFShimsPath        = "/.asdf/shims/flutter"
)

type FlutterInstallType struct {
	Name string
	// IsAvailable is set to true if the tool is available.
	IsAvailable bool
	// InstalledVersionsCommand returns a command to list versions installed by the tool.
	InstalledVersionsCommand func() *command.Command
	// ReleasesCommand returns a command to list available releases, if applicable.
	ReleasesCommand func(version flutterVersion) *command.Command
	// Install installs a specific Flutter version.
	Install func(version flutterVersion) error
	// SetDefault sets a specific Flutter version as default, if applicable.
	SetDefault func(version flutterVersion) error
}

// NewFlutterInstallTypeFVM creates a FlutterInstallType for FVM (Flutter Version Management).
//
// It checks if FVM is available, retrieves its version, and sets up commands for listing installed versions,
// installing a specific version, and setting a default version based on the FVM version features.
func (f *FlutterInstaller) NewFlutterInstallTypeFVM() FlutterInstallType {
	available, versionOut := f.fvmIsAvailable()
	if !available {
		return FlutterInstallType{
			Name:        FVMName,
			IsAvailable: false,
		}
	}

	after3_0_0, after3_1_0, after3_2_1, err := fvmParseVersionAndFeatures(versionOut)
	if err != nil {
		f.Warnf("Failed to investigate FVM version: %s", err)
	}
	listArgs := []string{"list"}
	if after3_1_0 {
		listArgs = []string{"api", "list", "--skip-size-calculation"}
	}

	defaultArgs := []string{}
	if after3_2_1 {
		// FVM sometimes does not take CI environment into account,
		// so we need to skip the input prompt, but this flag is only working great after 3.2.1.
		defaultArgs = append(defaultArgs, "--fvm-skip-input")
	}

	return FlutterInstallType{
		Name:        FVMName,
		IsAvailable: true,
		InstalledVersionsCommand: func() *command.Command {
			cmd := f.CmdFactory.Create("fvm", listArgs, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			return &cmd
		},
		Install: func(version flutterVersion) error {
			args := defaultArgs
			if after3_0_0 {
				// FVM 3.0.0 and above requires the --setup flag to setup the version.
				args = append(args, "--setup")
			}
			return f.fvmInstallVersion(version, args)
		},
		SetDefault: func(version flutterVersion) error {
			return f.fvmSetDefault(version, defaultArgs)
		},
		ReleasesCommand: func(version flutterVersion) *command.Command {
			args := append([]string{"releases"}, defaultArgs...)
			if after3_0_0 && version.channel != "stable" && version.channel != "" {
				args = append(args, "--channel", version.channel)
			}

			cmd := f.CmdFactory.Create("fvm", args, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			return &cmd
		},
	}
}

func (f *FlutterInstaller) fvmInstallVersion(version flutterVersion, defaultArgs []string) error {
	args := append([]string{"install", fvmCreateVersionString(version)}, defaultArgs...)

	cmd := f.CmdFactory.Create("fvm", args, nil)
	f.Donef("$ %s", cmd.PrintableCommandArgs())
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return fmt.Errorf("install: %s %s", err, out)
	}
	f.Debugf("Installed Flutter: %s", out)

	cachePath := os.Getenv("HOME") + FVMCacheDefaultPath
	path := os.Getenv("PATH")
	if !strings.Contains(path, cachePath) {
		// Add FVM cache to PATH
		if err := os.Setenv("PATH", fmt.Sprintf("%s:%s", cachePath, path)); err != nil {
			return fmt.Errorf("set env: %s", err)
		}
		f.Debugf("Added fvm cache to PATH: %s", os.Getenv("PATH"))
	}

	return nil
}

func (f *FlutterInstaller) fvmSetDefault(version flutterVersion, defaultArgs []string) error {
	args := append([]string{"global", fvmCreateVersionString(version), "--force"}, defaultArgs...)
	cmd := f.CmdFactory.Create("fvm", args, nil)
	f.Donef("$ %s", cmd.PrintableCommandArgs())
	_, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err == nil {
		return nil
	}

	f.Warnf("Flutter version %s already exists in FVM cache, setting as default manually.", fvmCreateVersionString(version))

	// Older FVM versions cannot operate with the --force flag, but without it, the command
	// hangs if a legacy version is installed with a 'v' prefix.
	// In this case, we try to set the default version by adding a symlink manually.
	home := os.Getenv("HOME")
	versionsDir := home + FVMCacheVersionsPath
	versionDir := versionsDir + "/" + fvmCreateVersionString(version)
	binFlutter := versionDir + "/bin/flutter"

	info, statErr := os.Stat(versionDir)
	if statErr != nil || !info.IsDir() {
		return fmt.Errorf("version directory does not exist: %s", versionDir)
	}
	entries, readErr := os.ReadDir(versionDir)
	if readErr != nil || len(entries) == 0 {
		return fmt.Errorf("version directory is empty: %s", versionDir)
	}

	// Set the default symlink to the selected Flutter version.
	defaultBin := home + FVMCacheDefaultPath
	if err := os.Remove(defaultBin); err != nil {
		f.Debugf("Failed to remove existing default symlink: %s", err)
	}
	linkErr := os.Symlink(binFlutter, defaultBin)
	if linkErr != nil {
		return fmt.Errorf("failed to set default symlink: %w", linkErr)
	}
	f.Debugf("Set Flutter default to %s manually", binFlutter)

	return nil
}

func (f *FlutterInstaller) fvmIsAvailable() (bool, string) {
	cmd := f.CmdFactory.Create("fvm", []string{"--version"}, nil)
	f.Donef("$ %s", cmd.PrintableCommandArgs())
	versionOut, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		f.Warnf("fvm version manager is not available")
		return false, versionOut
	}
	f.Debugf("fvm version: %s", versionOut)

	return true, ""
}

// fvmParseVersionAndFeatures parses the FVM version output and determines if it supports features introduced in specific versions.
func fvmParseVersionAndFeatures(versionOut string) (after3_0_0, after3_1_0, after3_2_1 bool, err error) {
	after3_0_0 = false
	after3_2_1 = false
	regex := regexp.MustCompile(`\d+\.\d+\.\d+`)

	versionParts := strings.Split(regex.FindString(versionOut), ".")
	if len(versionParts) < 3 {
		err = fmt.Errorf("parse fvm version: %s: not enough version parts found", versionOut)
		return
	}

	var major, minor, patch int
	_, majorErr := fmt.Sscan(versionParts[0], &major)
	_, minorErr := fmt.Sscan(versionParts[1], &minor)
	_, patchErr := fmt.Sscan(versionParts[2], &patch)

	if majorErr == nil && minorErr == nil && patchErr == nil {
		if major < 3 {
			return
		}

		after3_0_0 = true                                                // FVM 3.0.0 and above
		after3_1_0 = major > 3 || minor > 0 || (minor == 1 && patch > 0) // FVM 3.1.0 and above
		after3_2_1 = major > 3 || minor > 2 || (minor == 2 && patch > 0) // FVM 3.2.1 and above

		return
	}
	err = fmt.Errorf("parse fvm version: %s:\nmajor:%w\nminor: %w\npatch: %w", versionOut, majorErr, minorErr, patchErr)

	return
}

func fvmCreateVersionString(version flutterVersion) string {
	versionString := version.version
	if versionString != "" {
		if version.channel != "" {
			versionString += "@" + version.channel
		}
	} else if version.channel != "" {
		versionString = version.channel
	} else {
		versionString = "stable" // default to stable if no version or channel is specified
	}
	return versionString
}

// NewFlutterInstallTypeASDF creates a FlutterInstallType for ASDF (Another System Definition Facility).
//
// It checks if ASDF is available, retrieves its version, and sets up commands for listing installed versions,
// installing a specific version, and setting a default version based on the ASDF version features.
func (f *FlutterInstaller) NewFlutterInstallTypeASDF() FlutterInstallType {
	if !f.asdfIsAvailable() {
		return FlutterInstallType{
			Name:        ASDFName,
			IsAvailable: false,
		}
	}

	return FlutterInstallType{
		Name:        ASDFName,
		IsAvailable: true,
		InstalledVersionsCommand: func() *command.Command {
			cmd := f.CmdFactory.Create("asdf", []string{"list", "flutter"}, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			return &cmd
		},
		Install:    f.asdfInstallVersion,
		SetDefault: f.asdfSetDefault,
		ReleasesCommand: func(version flutterVersion) *command.Command {
			cmd := f.CmdFactory.Create("asdf", []string{"list", "all", "flutter"}, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			return &cmd
		},
	}
}

func (f *FlutterInstaller) asdfInstallVersion(version flutterVersion) error {
	versionString := asdfCreateVersionString(version)
	cmd := f.CmdFactory.Create("asdf", []string{"install", "flutter", versionString}, nil)
	f.Donef("$ %s", cmd.PrintableCommandArgs())
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return fmt.Errorf("install: %s %s", err, out)
	}
	f.Debugf("Installed Flutter: %s", out)

	shimsPath := os.Getenv("HOME") + ASDFShimsPath
	path := os.Getenv("PATH")
	if !strings.Contains(path, shimsPath) {
		// Add asdf shims to PATH
		if err := os.Setenv("PATH", fmt.Sprintf("%s:%s", shimsPath, path)); err != nil {
			return fmt.Errorf("set env: %s", err)
		}
		f.Debugf("Added asdf shims to PATH: %s", os.Getenv("PATH"))
	}

	// Reshim the flutter command to ensure the new version is available
	cmd = f.CmdFactory.Create("asdf", []string{"reshim", "flutter", versionString}, nil)
	f.Donef("$ %s", cmd.PrintableCommandArgs())
	if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("reshim version: %s %s", err, out)
	}

	return nil
}

func (f *FlutterInstaller) asdfSetDefault(version flutterVersion) error {
	versionString := asdfCreateVersionString(version)
	cmd := f.CmdFactory.Create("asdf", []string{"global", "flutter", versionString}, nil)
	f.Donef("$ %s", cmd.PrintableCommandArgs())
	if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("set version global: %s %s", err, out)
	}
	return nil
}

func (f *FlutterInstaller) asdfIsAvailable() bool {
	cmd := f.CmdFactory.Create("asdf", []string{"plugin-list"}, nil)
	f.Donef("$ %s", cmd.PrintableCommandArgs())
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil || !strings.Contains(out, "flutter") {
		f.Warnf("asdf version manager is not available")
		return false
	}

	return true
}

func asdfCreateVersionString(version flutterVersion) string {
	versionString := version.version
	if versionString == "" {
		// Default to latest if no version is specified.
		return "latest"
	}

	for _, channel := range Channels {
		if strings.Contains(versionString, channel) {
			// If the version already contains a channel, return it as is.
			return versionString
		}
	}

	if version.channel != "" {
		versionString += "-" + version.channel
		return versionString
	}

	// Default to stable if no channel is specified.
	versionString += "-stable"

	return versionString
}

// NewFlutterInstallTypeManual creates a FlutterInstallType for manual installation.
//
// To install a specific version, it downloads the Flutter SDK from the official website or
// uses git to clone the repository.
func (f *FlutterInstaller) NewFlutterInstallTypeManual() FlutterInstallType {
	return FlutterInstallType{
		Name:        ManualName,
		IsAvailable: true,
		InstalledVersionsCommand: func() *command.Command {
			cmd := f.CmdFactory.Create("flutter", []string{"--version", "--machine"}, nil)
			return &cmd
		},
		Install: func(version flutterVersion) error {
			return f.DownloadFlutterSDK(version)
		},
	}
}
