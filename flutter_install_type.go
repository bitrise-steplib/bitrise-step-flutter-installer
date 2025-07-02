package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/v2/command"
)

const (
	FVMName    = "fvm"
	ASDFName   = "asdf"
	ManualName = "manual"
)

type FlutterInstallType struct {
	Name                     string
	IsAvailable              bool                                          // if the tool is available, this will be set to true later
	InstalledVersionsCommand command.Command                               // command to list available versions installed by the tool
	ReleasesCommand          command.Command                               // command to list available releases (if applicable)
	InstallCommand           func(version flutterVersion) command.Command  // function to install a specific version
	SetDefaultCommand        func(version flutterVersion) *command.Command // function to set a specific version as default (if applicable)
	FullInstall              func() error                                  // function to perform a full install, if needed
}

func (f *FlutterInstaller) NewFlutterInstallTypeFVM() FlutterInstallType {
	cmd := f.CmdFactory.Create("fvm", []string{"--version"}, nil)
	f.Debugf("$ %s", cmd.PrintableCommandArgs())
	versionOut, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		f.Warnf("fvm is not available: %s", versionOut)
		return FlutterInstallType{
			Name:        FVMName,
			IsAvailable: false,
		}
	}

	useSetupFlag, useSkipInputFlag, useAPI, err := fvmParseVersionAndFeatures(versionOut)
	if err != nil {
		f.Warnf("Failed to investigate FVM version: %s", err)
	}
	listArgs := []string{"list"}
	if useAPI {
		listArgs = []string{"api", "list", "--skip-size-calculation"}
	}

	return FlutterInstallType{
		Name:                     FVMName,
		IsAvailable:              true,
		InstalledVersionsCommand: f.CmdFactory.Create("fvm", listArgs, nil),
		InstallCommand: func(version flutterVersion) command.Command {
			options := command.Opts{
				Env: []string{"CI=true"},
			}
			args := []string{"install", fvmCreateVersionString(version)}
			if useSetupFlag {
				args = append(args, "--setup")
			}
			if useSkipInputFlag {
				args = append(args, "--fvm-skip-input")
			}

			return f.CmdFactory.Create("fvm", args, &options)
		},
		SetDefaultCommand: func(version flutterVersion) *command.Command {
			options := command.Opts{
				Env: []string{"CI=true"},
			}
			args := []string{"global", fvmCreateVersionString(version)}
			if useSkipInputFlag {
				args = append(args, "--fvm-skip-input")
			}

			cmd := f.CmdFactory.Create("fvm", args, &options)
			return &cmd
		},
		ReleasesCommand: f.CmdFactory.Create("fvm", []string{"releases"}, nil),
	}
}

func fvmParseVersionAndFeatures(versionOut string) (useSetupFlag, useSkipInputFlag, useAPIFlag bool, err error) {
	useSetupFlag = false
	useSkipInputFlag = false
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

			useSetupFlag = true                                                    // FVM 3.0.0 and above
			useAPIFlag = major > 3 || minor > 0 || (minor == 1 && patch > 0)       // FVM 3.1.0 and above
			useSkipInputFlag = major > 3 || minor > 2 || (minor == 2 && patch > 0) // FVM 3.2.1 and above

			return
		}
		err = fmt.Errorf("failed to parse fvm version: %s: major:%w minor: %w patch: %w", versionOut, majorErr, minorErr, patchErr)
	} else {
		err = fmt.Errorf("failed to parse fvm version: %s", versionOut)
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
	f.Debugf("$ %s", cmd.PrintableCommandArgs())
	out, err := cmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil || !strings.Contains(out, "flutter") {
		f.Warnf("asdf is not available: %s", out)
		return FlutterInstallType{
			Name:        ASDFName,
			IsAvailable: false,
		}
	}
	return FlutterInstallType{
		Name:                     ASDFName,
		IsAvailable:              true,
		InstalledVersionsCommand: f.CmdFactory.Create("asdf", []string{"list", "flutter"}, nil),
		InstallCommand: func(version flutterVersion) command.Command {
			options := command.Opts{
				Env: []string{"CI=true"},
			}
			return f.CmdFactory.Create("asdf", []string{"install", "flutter", asdfCreateVersionString(version)}, &options)
		},
		SetDefaultCommand: func(version flutterVersion) *command.Command {
			options := command.Opts{
				Env: []string{"CI=true"},
			}
			cmd := f.CmdFactory.Create("asdf", []string{"global", "flutter", asdfCreateVersionString(version)}, &options)
			return &cmd
		},
		ReleasesCommand: f.CmdFactory.Create("asdf", []string{"list", "all", "flutter"}, nil),
	}
}

func asdfCreateVersionString(version flutterVersion) string {
	versionString := version.version
	if versionString == "" {
		versionString = "latest" // default to latest if no version is specified
	} else if version.channel != "" {
		versionString += "-" + version.channel
	}
	return versionString
}

func (f *FlutterInstaller) NewFlutterInstallTypeManual() FlutterInstallType {
	return FlutterInstallType{
		Name:                     ManualName,
		IsAvailable:              true,
		InstalledVersionsCommand: f.CmdFactory.Create("flutter", []string{"--version"}, nil),
		FullInstall: func() error {
			return f.downloadFlutterSDK()
		},
	}
}

func (f *FlutterInstaller) downloadFlutterSDK() error {
	f.Infof("Downloading Flutter SDK")

	sdkPathParent := filepath.Join(os.Getenv("HOME"), "flutter-sdk")
	flutterSDKPath := filepath.Join(sdkPathParent, "flutter")

	f.Printf("Cleaning SDK target path: %s", sdkPathParent)
	if err := os.RemoveAll(sdkPathParent); err != nil {
		return fmt.Errorf("remove path(%s): %s", sdkPathParent, err)
	}

	if err := os.MkdirAll(sdkPathParent, 0770); err != nil {
		return fmt.Errorf("create folder (%s): %s", sdkPathParent, err)
	}

	if f.Config.BundleSpecified {
		f.Infof("Downloading and unarchiving Flutter from installation bundle: %s", f.Config.BundleURL)

		if err := f.downloadAndUnarchiveBundle(f.Config.BundleURL, sdkPathParent); err != nil {
			return fmt.Errorf("download and unarchive bundle: %s", err)
		}
	} else {
		f.Infof("Cloning Flutter from the git repository (https://github.com/flutter/flutter.git)")
		f.Infof("Selected branch/tag: %s", f.Config.Version)

		// repository name ('flutter') is in the path, will be checked out there
		cmd := f.CmdFactory.Create("git", []string{
			"clone",
			"https://github.com/flutter/flutter.git",
			flutterSDKPath,
			"--depth", "1",
			"--branch", f.Config.Version,
		}, nil)
		out, err := cmd.RunAndReturnTrimmedCombinedOutput()
		if err != nil {
			return fmt.Errorf("clone git repo for tag/branch: %s: %s", f.Config.Version, out)
		}
	}

	f.Printf("Adding flutter bin directory to $PATH")
	f.Debugf("PATH: %s", os.Getenv("PATH"))

	path := filepath.Join(flutterSDKPath, "bin")
	path += ":" + filepath.Join(flutterSDKPath, "bin", "cache", "dart-sdk", "bin")
	path += ":" + filepath.Join(flutterSDKPath, ".pub-cache", "bin")
	path += ":" + filepath.Join(os.Getenv("HOME"), ".pub-cache", "bin")
	path += ":" + os.Getenv("PATH")

	if err := os.Setenv("PATH", path); err != nil {
		return fmt.Errorf("set env: %s", err)
	}

	if err := tools.ExportEnvironmentWithEnvman("PATH", path); err != nil {
		return fmt.Errorf("export env with envman: %s", err)
	}

	f.Donef("Added to $PATH")
	f.Debugf("PATH: %s", os.Getenv("PATH"))

	if f.Config.IsDebug {
		flutterBinPath, err := exec.LookPath("flutter")
		if err != nil {
			return fmt.Errorf("get Flutter binary path")
		}
		f.Infof("Flutter binary path: %s", flutterBinPath)

		cmdOpts := command.Opts{
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}
		treeCmd := f.CmdFactory.Create("tree", []string{"-L", "3", sdkPathParent}, &cmdOpts)
		f.Donef("$ %s", treeCmd.PrintableCommandArgs())
		if err := treeCmd.Run(); err != nil {
			f.Warnf("run tree command: %s", err)
		}

		f.printDirOwner(flutterSDKPath)
	}

	f.Infof("Flutter version")
	cmdOpts := command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	versionCmd := f.CmdFactory.Create("flutter", []string{"--version"}, &cmdOpts)
	f.Donef("$ %s", versionCmd.PrintableCommandArgs())
	if err := versionCmd.Run(); err != nil {
		return fmt.Errorf("check flutter version: %s", err)
	}

	return nil
}

func (f *FlutterInstaller) printDirOwner(flutterSDKPath string) {
	cmdOpts := command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	dirOwnerCmd := f.CmdFactory.Create("ls", []string{"-al", flutterSDKPath}, &cmdOpts)
	f.Donef("$ %s", dirOwnerCmd.PrintableCommandArgs())
	if err := dirOwnerCmd.Run(); err != nil {
		f.Warnf("run ls: %s", err)
	}
}
