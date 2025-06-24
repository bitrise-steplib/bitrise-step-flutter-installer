package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/command/git"
)

type FlutterInstallType struct {
	Name              string
	CheckAvailability func() bool                                 // function to check if the tool is available
	IsAvailable       bool                                        // if the tool is available, this will be set to true later
	VersionsCommand   *command.Model                              // command to list available versions installed by the tool
	InstallCommand    func(version flutterVersion) *command.Model // function to install a specific version
	SetDefaultCommand func(version flutterVersion) *command.Model // function to set a specific version as default
	FullInstall       func(cfg *Config) error                     // function to perform a full install, if needed
}

var FlutterInstallTypeFVM = FlutterInstallType{
	Name:              "fvm",
	CheckAvailability: func() bool { return command.New("fvm", "--version").Run() == nil },
	VersionsCommand:   command.New("fvm", "api", "list", "--skip-size-calculation"),
	InstallCommand: func(version flutterVersion) *command.Model {
		return command.New("export", "CI=true", "&&", "fvm", "install", fvmCreateVersionString(version), "--setup")
	},
	SetDefaultCommand: func(version flutterVersion) *command.Model {
		return command.New("export", "CI=true", "&&", "fvm", "global", fvmCreateVersionString(version))
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
	CheckAvailability: func() bool { return command.New("asdf", "--version").Run() == nil },
	VersionsCommand:   command.New("asdf", "list", "flutter"),
	InstallCommand: func(version flutterVersion) *command.Model {
		return command.New("export", "CI=true", "&&", "asdf", "install", "flutter", asdfCreateVersionString(version))
	},
	SetDefaultCommand: func(version flutterVersion) *command.Model {
		return command.New("export", "CI=true", "&&", "asdf", "global", "flutter", asdfCreateVersionString(version))
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
	VersionsCommand:   command.New("flutter", "--version"),
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
		return fmt.Errorf("failed to remove path(%s), error: %s", sdkPathParent, err)
	}

	if err := os.MkdirAll(sdkPathParent, 0770); err != nil {
		return fmt.Errorf("failed to create folder (%s), error: %s", sdkPathParent, err)
	}

	if cfg.BundleSpecified {
		logger.Println()
		logger.Infof("Downloading and unarchiving Flutter from installation bundle: %s", cfg.BundleURL)

		if err := downloadAndUnarchiveBundle(cfg.BundleURL, sdkPathParent); err != nil {
			return fmt.Errorf("failed to download and unarchive bundle, error: %s", err)
		}
	} else {
		logger.Infof("Cloning Flutter from the git repository (https://github.com/flutter/flutter.git)")
		logger.Infof("Selected branch/tag: %s", cfg.Version)

		// repository name ('flutter') is in the path, will be checked out there
		gitRepo, err := git.New(flutterSDKPath)
		if err != nil {
			return fmt.Errorf("failed to open git repo, error: %s", err)
		}

		if err := gitRepo.CloneTagOrBranch("https://github.com/flutter/flutter.git", cfg.Version).Run(); err != nil {
			return fmt.Errorf("failed to clone git repo for tag/branch: %s, error: %s", cfg.Version, err)
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
		return fmt.Errorf("failed to set env, error: %s", err)
	}

	if err := tools.ExportEnvironmentWithEnvman("PATH", path); err != nil {
		return fmt.Errorf("failed to export env with envman, error: %s", err)
	}

	logger.Donef("Added to $PATH")
	logger.Debugf("PATH: %s", os.Getenv("PATH"))

	if cfg.IsDebug {
		flutterBinPath, err := exec.LookPath("flutter")
		if err != nil {
			return fmt.Errorf("failed to get Flutter binary path")
		}
		logger.Infof("Flutter binary path: %s", flutterBinPath)

		treeCmd := command.New("tree", "-L", "3", sdkPathParent).SetStdout(os.Stdout).SetStderr(os.Stderr)
		logger.Donef("$ %s", treeCmd.PrintableCommandArgs())
		logger.Println()
		if err := treeCmd.Run(); err != nil {
			logger.Warnf("Failed to run tree command: %s", err)
		}

		printDirOwner(flutterSDKPath)
	}

	logger.Println()
	logger.Infof("Flutter version")
	versionCmd := command.New("flutter", "--version").SetStdout(os.Stdout).SetStderr(os.Stderr)
	logger.Donef("$ %s", versionCmd.PrintableCommandArgs())
	logger.Println()
	if err := versionCmd.Run(); err != nil {
		return fmt.Errorf("failed to check flutter version, error: %s", err)
	}

	if cfg.IsDebug {
		if err := runFlutterDoctor(); err != nil {
			return err
		}
	}

	return nil
}
