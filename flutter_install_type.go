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
	Name                     string
	IsAvailable              bool                                          // if the tool is available, this will be set to true later
	InstalledVersionsCommand func() *command.Command                       // command to list available versions installed by the tool
	ReleasesCommand          func(version flutterVersion) *command.Command // command to list available releases (if applicable)
	Install                  func(version flutterVersion) error            // function to install a specific version
	SetDefault               func(version flutterVersion) error            // function to set a specific version as default (if applicable)
}

func (f *FlutterInstaller) NewFlutterInstallTypeFVM() FlutterInstallType {
	cmd := f.CmdFactory.Create("fvm", []string{"--version"}, nil)
	f.Donef("$ %s", cmd.PrintableCommandArgs())
	versionOut, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		f.Warnf("fvm version manager is not available")
		return FlutterInstallType{
			Name:        FVMName,
			IsAvailable: false,
		}
	} else {
		f.Debugf("fvm --version output: %s", versionOut)
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
			args := append([]string{"install", fvmCreateVersionString(version)}, defaultArgs...)
			if after3_0_0 {
				args = append(args, "--setup")
			}

			cmd := f.CmdFactory.Create("fvm", args, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
				return fmt.Errorf("install: %s %s", err, out)
			} else {
				f.Debugf("Installed Flutter: %s", out)
				cachePath := os.Getenv("HOME") + FVMCacheDefaultPath
				path := os.Getenv("PATH")
				if err := os.Setenv("PATH", fmt.Sprintf("%s:%s", cachePath, path)); err != nil {
					return fmt.Errorf("set env: %s", err)
				}
				f.Debugf("Added fvm cache to PATH: %s", os.Getenv("PATH"))
			}
			return nil
		},
		SetDefault: func(version flutterVersion) error {
			args := append([]string{"global", fvmCreateVersionString(version), "--force"}, defaultArgs...)
			cmd := f.CmdFactory.Create("fvm", args, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
				f.Debugf("Failed to set default version with fvm: %s %s", err, out)
				f.Warnf("Flutter version %s already exists in FVM cache, setting as default manually.", fvmCreateVersionString(version))

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

				// Set the symlink
				defaultBin := home + FVMCacheDefaultPath
				if err := os.Remove(defaultBin); err != nil {
					f.Debugf("Failed to remove existing default symlink: %s", err)
				}
				linkErr := os.Symlink(binFlutter, defaultBin)
				if linkErr != nil {
					return fmt.Errorf("failed to set default symlink: %w", linkErr)
				}
				f.Debugf("Set default Flutter to %s manually", binFlutter)
			}
			return nil
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

func fvmParseVersionAndFeatures(versionOut string) (after3_0_0, after3_1_0, after3_2_1 bool, err error) {
	after3_0_0 = false
	after3_2_1 = false
	regex := regexp.MustCompile(`\d+\.\d+\.\d+`)
	versionParts := strings.Split(regex.FindString(versionOut), ".")
	if len(versionParts) >= 3 {
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
	} else {
		err = fmt.Errorf("parse fvm version: %s", versionOut)
	}

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

func (f *FlutterInstaller) NewFlutterInstallTypeASDF() FlutterInstallType {
	cmd := f.CmdFactory.Create("asdf", []string{"plugin-list"}, nil)
	f.Donef("$ %s", cmd.PrintableCommandArgs())
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil || !strings.Contains(out, "flutter") {
		f.Warnf("asdf version manager is not available")
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
		Install: func(version flutterVersion) error {
			cmd := f.CmdFactory.Create("asdf", []string{"install", "flutter", asdfCreateVersionString(version)}, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
				return fmt.Errorf("install: %s %s", err, out)
			} else {
				f.Debugf("Installed Flutter: %s", out)
				shimsPath := os.Getenv("HOME") + ASDFShimsPath
				path := os.Getenv("PATH")
				if err := os.Setenv("PATH", fmt.Sprintf("%s:%s", shimsPath, path)); err != nil {
					return fmt.Errorf("set env: %s", err)
				}
				f.Debugf("Added asdf shims to PATH: %s", os.Getenv("PATH"))
			}
			return nil
		},
		SetDefault: func(version flutterVersion) error {
			versionString := asdfCreateVersionString(version)
			cmd := f.CmdFactory.Create("asdf", []string{"reshim", "flutter", versionString}, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
				return fmt.Errorf("reshim version: %s %s", err, out)
			}
			cmd = f.CmdFactory.Create("asdf", []string{"global", "flutter", versionString}, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
				return fmt.Errorf("set version global: %s %s", err, out)
			}
			cmd = f.CmdFactory.Create("asdf", []string{"local", "flutter", versionString}, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			if out, err := cmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
				return fmt.Errorf("set version local: %s %s", err, out)
			}
			return nil
		},
		ReleasesCommand: func(version flutterVersion) *command.Command {
			cmd := f.CmdFactory.Create("asdf", []string{"list", "all", "flutter"}, nil)
			f.Donef("$ %s", cmd.PrintableCommandArgs())
			return &cmd
		},
	}
}

func asdfCreateVersionString(version flutterVersion) string {
	versionString := version.version
	if versionString == "" {
		versionString = "latest" // default to latest if no version is specified
	} else {
		channelIncluded := false
		for _, channel := range Channels {
			if strings.Contains(versionString, channel) {
				channelIncluded = true
				break
			}
		}
		if !channelIncluded {
			if version.channel != "" {
				versionString += "-" + version.channel
			} else {
				versionString += "-stable" // default to stable if no version is specified
			}
		}

	}

	return versionString
}

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
