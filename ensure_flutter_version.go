package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bitrise-io/go-flutter/flutterproject"
	"github.com/bitrise-io/go-steputils/tools"
	"github.com/bitrise-io/go-utils/command"
	"github.com/bitrise-io/go-utils/command/git"
	"github.com/bitrise-io/go-utils/errorutil"
	"github.com/bitrise-io/go-utils/log"
)

type flutterVersion struct {
	version string
	channel string
}

func NewFlutterVersionFromFlutterOutput(input string) (flutterVersion, error) {
	version, err := matchFlutterOutputVersion(input)
	if err != nil {
		return flutterVersion{}, err
	}

	channel, err := matchFlutterOutputChannel(input)
	if err != nil {
		return flutterVersion{channel: channel}, err
	}

	return flutterVersion{
		channel: channel,
		version: version,
	}, nil
}

func NewFlutterVersionFromString(input string) (flutterVersion, error) {
	version, err := matchVersion(input)
	if err != nil {
		return flutterVersion{}, err
	}

	channel, err := matchChannel(input)
	if err != nil {
		return flutterVersion{channel: channel}, err
	}

	return flutterVersion{
		channel: channel,
		version: version,
	}, nil
}

type FlutterInstallType struct {
	Name              string
	CheckAvailability func() bool                                 // function to check if the tool is available
	IsAvailable       bool                                        // if the tool is available, this will be set to true later
	VersionsCommand   *command.Model                              // command to list available versions installed by the tool
	InstallCommand    func(version flutterVersion) *command.Model // function to install a specific version
	SetDefaultCommand func(version flutterVersion) *command.Model // function to set a specific version as default
	FullInstall       func(cfg *Config) error                     // function to perform a full install, if needed
	ConfigVersion     string                                      // version specfied in config file, if any
}

var FlutterInstallTypeFVM = FlutterInstallType{
	Name:              "fvm",
	CheckAvailability: func() bool { return command.New("fvm", "--version").Run() == nil },
	VersionsCommand:   command.New("fvm", "list"),
	InstallCommand: func(version flutterVersion) *command.Model {
		versionString := version.version
		if version.channel != "" {
			versionString += "@" + version.channel
		}
		return command.New("fvm", "install", versionString)
	},
	SetDefaultCommand: func(version flutterVersion) *command.Model {
		versionString := version.version
		if version.channel != "" {
			versionString += "@" + version.channel
		}
		return command.New("fvm", "global", versionString)
	},
}

var FlutterInstallTypeAsdf = FlutterInstallType{
	Name:              "asdf",
	CheckAvailability: func() bool { return command.New("asdf", "--version").Run() == nil },
	VersionsCommand:   command.New("asdf", "list", "flutter"),
	InstallCommand: func(version flutterVersion) *command.Model {
		versionString := version.version
		if version.channel != "" {
			versionString += "-" + version.channel
		}
		return command.New("asdf", "install", "flutter", versionString)
	},
	SetDefaultCommand: func(version flutterVersion) *command.Model {
		versionString := version.version
		if version.channel != "" {
			versionString += "-" + version.channel
		}
		return command.New("asdf", "global", "flutter", versionString)
	},
}

var FlutterInstallTypeManual = FlutterInstallType{
	Name:              "manual",
	CheckAvailability: func() bool { return true },
	VersionsCommand:   command.New("flutter", "--version"),
	FullInstall: func(cfg *Config) error {
		return downloadFlutterSDK(cfg)
	},
}

func EnsureFlutterVersion(cfg *Config, sdkVersions flutterproject.FlutterAndDartSDKVersions) error {
	var requiredVersion flutterVersion
	var installTypes []*FlutterInstallType
	if cfg.BundleSpecified && cfg.BundleURL != "" {
		log.Infof("Using Flutter version from bundle: %s", cfg.BundleURL)
		installTypes = []*FlutterInstallType{&FlutterInstallTypeManual, &FlutterInstallTypeFVM, &FlutterInstallTypeAsdf} // Manual install first, then FVM and ASDF as fallback
	} else {
		installTypes = []*FlutterInstallType{&FlutterInstallTypeFVM, &FlutterInstallTypeAsdf, &FlutterInstallTypeManual} // FVM and ASDF first, then manual install as fallback
	}

	if parsedVersion, err := NewFlutterVersionFromString(strings.TrimSpace(cfg.Version)); err == nil {
		requiredVersion = parsedVersion
	} else {
		// If the version is not specified or cannot be parsed, we try to get it from the project files
		log.Warnf("failed to parse required Flutter version: %s, error: %w", cfg.Version, err)
		if sdkVersions.PubspecFlutterVersion != nil {
			requiredVersion = flutterVersion{
				version: sdkVersions.PubspecFlutterVersion.String(),
			}
		} else if sdkVersions.FVMFlutterVersion != nil {
			requiredVersion = flutterVersion{
				version: sdkVersions.FVMFlutterVersion.String(),
				channel: sdkVersions.FVMFlutterChannel,
			}
		} else if sdkVersions.ASDFFlutterVersion != nil {
			requiredVersion = flutterVersion{
				version: sdkVersions.ASDFFlutterVersion.String(),
				channel: sdkVersions.ASDFFlutterChannel,
			}
		} else {
			return fmt.Errorf("no Flutter version specified in the configuration or project files")
		}
	}
	log.Infof("Required Flutter version: %s (%s)", requiredVersion.version, requiredVersion.channel)

	currentVersion, _, err := flutterVersionInfo()
	if err == nil && currentVersion.version == requiredVersion.version && currentVersion.channel == requiredVersion.channel {
		log.Infof("Flutter version %s (%s) is already installed", currentVersion.version, currentVersion.channel)
		return nil
	}

	installedAndDefault := false
	for _, installType := range installTypes {
		if installType.CheckAvailability == nil || !installType.CheckAvailability() {
			log.Warnf("Flutter install tool %s is not available, skipping", installType.Name)
			continue
		} else {
			installType.IsAvailable = true
		}

		success, err := setDefaultIfInstalled(installType, requiredVersion)
		if err != nil {
			log.Warnf("Failed to set Flutter version with %s, error: %s", installType.Name, err)
		}
		if success {
			installedAndDefault = true
			break
		}
	}

	if !installedAndDefault {
		log.Infof("Flutter version %s (%s) is not installed, installing...", requiredVersion.version, requiredVersion.channel)
		for _, installType := range installTypes {
			success, err := installAndSetDefault(installType, requiredVersion, cfg)
			if err != nil {
				log.Warnf("Failed to install Flutter version with %s, error: %s", installType.Name, err)
			}
			if success {
				break
			}
		}
	}

	currentVersion, _, err = flutterVersionInfo()
	if err == nil && currentVersion.version == requiredVersion.version && currentVersion.channel == requiredVersion.channel {
		log.Infof("Flutter version %s (%s) is installed and ready to use", currentVersion.version, currentVersion.channel)
		return nil
	}

	return fmt.Errorf("flutter version %s (%s) could not be installed or set as default", requiredVersion.version, requiredVersion.channel)
}

func installAndSetDefault(installType *FlutterInstallType, version flutterVersion, cfg *Config) (bool, error) {
	if !installType.IsAvailable {
		log.Warnf("Flutter install tool %s is not available, skipping", installType.Name)
		return false, nil
	}

	log.Donef("Installing Flutter version %s (%s) with %s", version.version, version.channel, installType.Name)
	if installType.FullInstall != nil {
		if err := installType.FullInstall(cfg); err != nil {
			log.Warnf("Failed to install Flutter version %s (%s) with %s, error: %s", version.version, version.channel, installType.Name, err)
			return false, fmt.Errorf("failed to install Flutter version with %s, error: %w", installType.Name, err)
		}
	} else {
		installCmd := installType.InstallCommand(version)
		log.Donef("$ %s", installCmd.PrintableCommandArgs())
		fmt.Println()
		if err := installCmd.Run(); err != nil {
			log.Warnf("Failed to install Flutter version %s (%s) with %s, error: %s", version.version, version.channel, installType.Name, err)
			return false, fmt.Errorf("failed to install Flutter version with %s, error: %w", installType.Name, err)
		}
	}

	log.Donef("Flutter version %s (%s) installed successfully with %s", version.version, version.channel, installType.Name)
	if installType.SetDefaultCommand != nil {
		log.Donef("Setting Flutter version to %s (%s) with %s", version.version, version.channel, installType.Name)
		setCmd := installType.SetDefaultCommand(version)
		log.Donef("$ %s", setCmd.PrintableCommandArgs())
		fmt.Println()

		if err := setCmd.Run(); err != nil {
			return false, fmt.Errorf("failed to set Flutter version with %s, error: %w", installType.Name, err)
		}
		log.Donef("Flutter version %s (%s) set as default with %s", version.version, version.channel, installType.Name)
	}
	return true, nil
}

func setDefaultIfInstalled(installType *FlutterInstallType, version flutterVersion) (bool, error) {
	out, err := installType.VersionsCommand.RunAndReturnTrimmedOutput()
	if err != nil {
		log.Warnf("Failed to list Flutter versions with %s, error: %s", installType.Name, err)
		return false, nil
	}

	if strings.Contains(out, version.version) {
		if installType.SetDefaultCommand != nil {
			log.Donef("Setting Flutter version to %s (%s) with %s", version.version, version.channel, installType.Name)
			setCmd := installType.SetDefaultCommand(version)
			log.Donef("$ %s", setCmd.PrintableCommandArgs())
			fmt.Println()

			if err := setCmd.Run(); err != nil {
				return false, fmt.Errorf("failed to set Flutter version with %s, error: %w", installType.Name, err)
			}
		}
		return true, nil
	}

	return false, nil
}

func downloadFlutterSDK(cfg *Config) error {
	fmt.Println()
	log.Infof("Downloading Flutter SDK")
	fmt.Println()

	sdkPathParent := filepath.Join(os.Getenv("HOME"), "flutter-sdk")
	flutterSDKPath := filepath.Join(sdkPathParent, "flutter")

	log.Printf("Cleaning SDK target path: %s", sdkPathParent)
	if err := os.RemoveAll(sdkPathParent); err != nil {
		return fmt.Errorf("failed to remove path(%s), error: %s", sdkPathParent, err)
	}

	if err := os.MkdirAll(sdkPathParent, 0770); err != nil {
		return fmt.Errorf("failed to create folder (%s), error: %s", sdkPathParent, err)
	}

	if cfg.BundleSpecified {
		fmt.Println()
		log.Infof("Downloading and unarchiving Flutter from installation bundle: %s", cfg.BundleURL)

		if err := downloadAndUnarchiveBundle(cfg.BundleURL, sdkPathParent); err != nil {
			return fmt.Errorf("failed to download and unarchive bundle, error: %s", err)
		}
	} else {
		log.Infof("Cloning Flutter from the git repository (https://github.com/flutter/flutter.git)")
		log.Infof("Selected branch/tag: %s", cfg.Version)

		// repository name ('flutter') is in the path, will be checked out there
		gitRepo, err := git.New(flutterSDKPath)
		if err != nil {
			return fmt.Errorf("failed to open git repo, error: %s", err)
		}

		if err := gitRepo.CloneTagOrBranch("https://github.com/flutter/flutter.git", cfg.Version).Run(); err != nil {
			return fmt.Errorf("failed to clone git repo for tag/branch: %s, error: %s", cfg.Version, err)
		}
	}

	log.Printf("Adding flutter bin directory to $PATH")
	log.Debugf("PATH: %s", os.Getenv("PATH"))

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

	log.Donef("Added to $PATH")
	log.Debugf("PATH: %s", os.Getenv("PATH"))

	if cfg.IsDebug {
		flutterBinPath, err := exec.LookPath("flutter")
		if err != nil {
			return fmt.Errorf("failed to get Flutter binary path")
		}
		log.Infof("Flutter binary path: %s", flutterBinPath)

		treeCmd := command.New("tree", "-L", "3", sdkPathParent).SetStdout(os.Stdout).SetStderr(os.Stderr)
		log.Donef("$ %s", treeCmd.PrintableCommandArgs())
		fmt.Println()
		if err := treeCmd.Run(); err != nil {
			log.Warnf("Failed to run tree command: %s", err)
		}

		printDirOwner(flutterSDKPath)
	}

	fmt.Println()
	log.Infof("Flutter version")
	versionCmd := command.New("flutter", "--version").SetStdout(os.Stdout).SetStderr(os.Stderr)
	log.Donef("$ %s", versionCmd.PrintableCommandArgs())
	fmt.Println()
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

func flutterVersionInfo() (flutterVersion, string, error) {
	fmt.Println()
	versionCmd := command.New("flutter", "--version")
	log.Donef("$ %s", versionCmd.PrintableCommandArgs())
	fmt.Println()

	out, err := versionCmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		if errorutil.IsExitStatusError(err) {
			return flutterVersion{}, out, fmt.Errorf("failed to get flutter version, error: %s, out: %s", err, out)
		}
		return flutterVersion{}, "", fmt.Errorf("failed to get flutter version, error: %s", err)
	}

	flutterVer, err := NewFlutterVersionFromFlutterOutput(out)

	return flutterVer, out, err
}

func matchFlutterOutputVersion(versionOutput string) (string, error) {
	versionRegexp := regexp.MustCompile(`(?im)^Flutter\s+(\S+?)\s+`)
	submatches := versionRegexp.FindStringSubmatch(versionOutput)
	if submatches == nil {
		return "", fmt.Errorf("failed to parse flutter version")
	}
	return submatches[1], nil
}

func matchFlutterOutputChannel(versionOutput string) (string, error) {
	channelRegexp := regexp.MustCompile(`(?im)\s+channel\s+(\S+?)\s+`)
	submatches := channelRegexp.FindStringSubmatch(versionOutput)
	if submatches == nil {
		return "", fmt.Errorf("failed to parse flutter channel")
	}
	return submatches[1], nil
}

func matchVersion(versionOutput string) (string, error) {
	versionRegexp := regexp.MustCompile(`\b[0-9]+\.[0-9]+\.[0-9]+(?:[-\.][A-Za-z0-9\.\-]+)?\b`)
	submatches := versionRegexp.FindStringSubmatch(versionOutput)
	if submatches == nil {
		return "", fmt.Errorf("failed to parse flutter version")
	}
	return submatches[1], nil
}

func matchChannel(versionOutput string) (string, error) {
	channelRegexp := regexp.MustCompile(`(?i)\b(stable|beta|main|dev)\b`)
	submatches := channelRegexp.FindStringSubmatch(versionOutput)
	if submatches == nil {
		return "", fmt.Errorf("failed to parse flutter channel")
	}
	return submatches[1], nil
}
