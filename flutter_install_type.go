package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/v2/command"
)

type FlutterInstallType struct {
	Name              string
	CheckAvailability func() bool                                   // function to check if the tool is available
	IsAvailable       bool                                          // if the tool is available, this will be set to true later
	VersionsCommand   command.Command                               // command to list available versions installed by the tool
	InstallCommand    func(version flutterVersion) command.Command  // function to install a specific version
	SetDefaultCommand func(version flutterVersion) *command.Command // function to set a specific version as default (if applicable)
	FullInstall       func(cfg *Config) error                       // function to perform a full install, if needed
}

var FlutterInstallTypeFVM = FlutterInstallType{
	Name:              "fvm",
	CheckAvailability: func() bool { return cmdFactory.Create("fvm", []string{"--version"}, nil).Run() == nil },
	VersionsCommand:   cmdFactory.Create("fvm", []string{"api", "list", "--skip-size-calculation"}, nil),
	InstallCommand: func(version flutterVersion) command.Command {
		commandString := `"export CI=true && fvm install ` + fvmCreateVersionString(version) + ` --setup"`
		return cmdFactory.Create("bash", []string{"-c", commandString}, nil)
	},
	SetDefaultCommand: func(version flutterVersion) *command.Command {
		commandString := `"export CI=true && fvm global ` + fvmCreateVersionString(version) + `"`
		cmd := cmdFactory.Create("bash", []string{"-c", commandString}, nil)
		return &cmd
	},
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

var FlutterInstallTypeAsdf = FlutterInstallType{
	Name:              "asdf",
	CheckAvailability: func() bool { return cmdFactory.Create("asdf", []string{"--version"}, nil).Run() == nil },
	VersionsCommand:   cmdFactory.Create("asdf", []string{"list", "flutter"}, nil),
	InstallCommand: func(version flutterVersion) command.Command {
		commandString := `"export CI=true && asdf install flutter ` + asdfCreateVersionString(version) + `"`
		return cmdFactory.Create("bash", []string{"-c", commandString}, nil)
	},
	SetDefaultCommand: func(version flutterVersion) *command.Command {
		commandString := `"export CI=true && asdf global flutter ` + asdfCreateVersionString(version) + `"`
		cmd := cmdFactory.Create("bash", []string{"-c", commandString}, nil)
		return &cmd
	},
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

var FlutterInstallTypeManual = FlutterInstallType{
	Name:              "manual",
	CheckAvailability: func() bool { return true },
	VersionsCommand:   cmdFactory.Create("flutter", []string{"--version"}, nil),
	FullInstall: func(cfg *Config) error {
		return downloadFlutterSDK(cfg)
	},
}

func downloadFlutterSDK(cfg *Config) error {
	logger.Println()
	logger.Infof("Downloading Flutter SDK")
	logger.Println()

	sdkPathParent := filepath.Join(os.Getenv("HOME"), "flutter-sdk")
	flutterSDKPath := filepath.Join(sdkPathParent, "flutter")

	logger.Printf("Cleaning SDK target path: %s", sdkPathParent)
	if err := os.RemoveAll(sdkPathParent); err != nil {
		return fmt.Errorf("remove path(%s): %s", sdkPathParent, err)
	}

	if err := os.MkdirAll(sdkPathParent, 0770); err != nil {
		return fmt.Errorf("create folder (%s): %s", sdkPathParent, err)
	}

	if cfg.BundleSpecified {
		logger.Println()
		logger.Infof("Downloading and unarchiving Flutter from installation bundle: %s", cfg.BundleURL)

		if err := downloadAndUnarchiveBundle(cfg.BundleURL, sdkPathParent); err != nil {
			return fmt.Errorf("download and unarchive bundle: %s", err)
		}
	} else {
		logger.Infof("Cloning Flutter from the git repository (https://github.com/flutter/flutter.git)")
		logger.Infof("Selected branch/tag: %s", cfg.Version)

		// repository name ('flutter') is in the path, will be checked out there
		cmd := cmdFactory.Create("git", []string{
			"clone",
			"https://github.com/flutter/flutter.git",
			flutterSDKPath,
			"--depth", "1",
			"--branch", cfg.Version,
		}, nil)
		out, err := cmd.RunAndReturnTrimmedCombinedOutput()
		if err != nil {
			return fmt.Errorf("clone git repo for tag/branch: %s: %s", cfg.Version, out)
		}
	}

	logger.Printf("Adding flutter bin directory to $PATH")
	logger.Debugf("PATH: %s", os.Getenv("PATH"))

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

	logger.Donef("Added to $PATH")
	logger.Debugf("PATH: %s", os.Getenv("PATH"))

	if cfg.IsDebug {
		flutterBinPath, err := exec.LookPath("flutter")
		if err != nil {
			return fmt.Errorf("get Flutter binary path")
		}
		logger.Infof("Flutter binary path: %s", flutterBinPath)

		cmdOpts := command.Opts{
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}
		treeCmd := cmdFactory.Create("tree", []string{"-L", "3", sdkPathParent}, &cmdOpts)
		logger.Donef("$ %s", treeCmd.PrintableCommandArgs())
		logger.Println()
		if err := treeCmd.Run(); err != nil {
			logger.Warnf("run tree command: %s", err)
		}

		printDirOwner(flutterSDKPath)
	}

	logger.Println()
	logger.Infof("Flutter version")
	cmdOpts := command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	versionCmd := cmdFactory.Create("flutter", []string{"--version"}, &cmdOpts)
	logger.Donef("$ %s", versionCmd.PrintableCommandArgs())
	logger.Println()
	if err := versionCmd.Run(); err != nil {
		return fmt.Errorf("check flutter version: %s", err)
	}

	return nil
}

func printDirOwner(flutterSDKPath string) {
	cmdOpts := command.Opts{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	dirOwnerCmd := cmdFactory.Create("ls", []string{"-al", flutterSDKPath}, &cmdOpts)
	logger.Donef("$ %s", dirOwnerCmd.PrintableCommandArgs())
	logger.Println()
	if err := dirOwnerCmd.Run(); err != nil {
		logger.Warnf("run ls: %s", err)
	}
}
