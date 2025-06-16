package main

import (
	"encoding/json"
	"errors"
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
)

type flutterVersion struct {
	version     string
	channel     string
	installType *FlutterInstallType
}

func NewFlutterVersionFromFlutterMachineOutput(input string) (flutterVersion, error) {
	var data map[string]any
	if err := json.Unmarshal([]byte(input), &data); err == nil {
		// JSON output from `flutter --version --machine`
		version := ""
		channel := ""
		var installType *FlutterInstallType
		if v, ok := data["frameworkVersion"].(string); ok && v != "" {
			version = v
		} else if v, ok := data["flutterVersion"].(string); ok && v != "" {
			version = v
		}
		if c, ok := data["channel"].(string); ok && c != "" {
			channel = c
		}
		if version == "" && channel == "" {
			return flutterVersion{}, fmt.Errorf("failed to find flutter version and channel in JSON output")
		}
		if m, ok := data["flutterRoot"].(string); ok && m != "" {
			if strings.Contains(m, FlutterInstallTypeFVM.Name) {
				installType = &FlutterInstallTypeFVM
			} else if strings.Contains(m, FlutterInstallTypeAsdf.Name) {
				installType = &FlutterInstallTypeAsdf
			}
		}

		return flutterVersion{
			version:     version,
			channel:     channel,
			installType: installType,
		}, nil
	}

	return NewFlutterVersionFromString(input)
}

func NewFlutterVersionFromString(input string) (flutterVersion, error) {
	version, versionErr := matchVersion(input)
	channel, channelErr := matchChannel(input)

	if versionErr != nil && channelErr != nil {
		return flutterVersion{}, fmt.Errorf("failed to parse flutter version or channel")
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

func EnsureFlutterVersion(cfg *Config, sdkVersions *flutterproject.FlutterAndDartSDKVersions) error {
	requiredVersion, err := fetchFlutterVersion(cfg, sdkVersions)
	if err != nil {
		return fmt.Errorf("failed to fetch required Flutter version, error: %w", err)
	}
	logger.Infof("Required Flutter version: %s (%s)", requiredVersion.version, requiredVersion.channel)

	currentVersion, _, err := flutterVersionInfo()
	if err == nil &&
		(requiredVersion.version == "" || currentVersion.version == requiredVersion.version) &&
		(requiredVersion.channel == "" || currentVersion.channel == requiredVersion.channel) {
		logger.Infof("Flutter version %s (%s) is already installed", currentVersion.version, currentVersion.channel)
		return nil
	}
	logger.Debugf("Current Flutter version: %s %s %s", currentVersion.version, currentVersion.channel, currentVersion.installType)

	var primaryManager = &FlutterInstallTypeFVM
	var secondaryManager = &FlutterInstallTypeAsdf
	if currentVersion.installType == &FlutterInstallTypeAsdf {
		primaryManager = &FlutterInstallTypeAsdf
		secondaryManager = &FlutterInstallTypeFVM
	}
	var installTypes []*FlutterInstallType
	if cfg.BundleSpecified && cfg.BundleURL != "" {
		logger.Infof("Using Flutter version from bundle: %s", cfg.BundleURL)
		installTypes = []*FlutterInstallType{&FlutterInstallTypeManual, primaryManager, secondaryManager} // Manual install first, then FVM and ASDF as fallback
	} else {
		installTypes = []*FlutterInstallType{primaryManager, secondaryManager, &FlutterInstallTypeManual} // FVM and ASDF first, then manual install as fallback
	}

	installedAndDefault := false
	for _, installType := range installTypes {
		logger.Debugf("Checking availability of Flutter install tool: %s", installType.Name)
		if installType.CheckAvailability == nil || !installType.CheckAvailability() {
			logger.Debugf("Flutter install tool %s is not available, skipping", installType.Name)
			continue
		} else {
			installType.IsAvailable = true
		}

		success, err := setDefaultIfInstalled(installType, requiredVersion)
		if err != nil {
			logger.Debugf("Failed to set Flutter version with %s, error: %s", installType.Name, err)
		}
		if success {
			installedAndDefault = true
			break
		}
	}

	if !installedAndDefault {
		logger.Infof("Flutter version %s (%s) is not installed, installing...", requiredVersion.version, requiredVersion.channel)
		for _, installType := range installTypes {
			if !installType.IsAvailable {
				logger.Debugf("Flutter install tool %s is not available, skipping", installType.Name)
				continue
			}
			success, err := installAndSetDefault(installType, requiredVersion, cfg)
			if err != nil {
				logger.Debugf("Failed to install Flutter version with %s, error: %s", installType.Name, err)
			}
			if success {
				break
			}
		}
	}

	currentVersion, _, err = flutterVersionInfo()
	if err == nil && currentVersion.version == requiredVersion.version && currentVersion.channel == requiredVersion.channel {
		logger.Infof("Flutter version %s (%s) is installed and ready to use", currentVersion.version, currentVersion.channel)
		return nil
	}

	return fmt.Errorf("flutter version %s (%s) could not be installed or set as default", requiredVersion.version, requiredVersion.channel)
}

func fetchFlutterVersion(cfg *Config, sdkVersions *flutterproject.FlutterAndDartSDKVersions) (flutterVersion, error) {
	parsedVersion, err := NewFlutterVersionFromString(strings.TrimSpace(cfg.Version))
	if err == nil {
		return parsedVersion, nil
	}

	logger.Warnf("failed to parse required Flutter version: %s, error: %w", cfg.Version, err)

	if sdkVersions != nil {
		// If the version is not specified or cannot be parsed, we try to get it from the project files
		if sdkVersions.PubspecFlutterVersion != nil {
			return flutterVersion{
				version: sdkVersions.PubspecFlutterVersion.String(),
			}, nil
		} else if sdkVersions.FVMFlutterVersion != nil {
			var channel string
			if sdkVersions.FVMFlutterChannel != "" {
				channel = sdkVersions.FVMFlutterChannel
			}
			return flutterVersion{
				version: sdkVersions.FVMFlutterVersion.String(),
				channel: channel,
			}, nil
		} else if sdkVersions.ASDFFlutterVersion != nil {
			var channel string
			if sdkVersions.ASDFFlutterChannel != "" {
				channel = sdkVersions.ASDFFlutterChannel
			}
			return flutterVersion{
				version: sdkVersions.ASDFFlutterVersion.String(),
				channel: channel,
			}, nil
		}
	}

	return flutterVersion{}, fmt.Errorf("no Flutter version specified in the configuration or project files")
}

func installAndSetDefault(installType *FlutterInstallType, version flutterVersion, cfg *Config) (bool, error) {
	logger.Debugf("Installing Flutter version %s (%s) with %s", version.version, version.channel, installType.Name)
	if installType.FullInstall != nil {
		if err := installType.FullInstall(cfg); err != nil {
			logger.Debugf("Failed to install Flutter version %s (%s) with %s, error: %s", version.version, version.channel, installType.Name, err)
			return false, fmt.Errorf("failed to install Flutter version with %s, error: %w", installType.Name, err)
		}
	} else {
		installCmd := installType.InstallCommand(version)
		logger.Debugf("$ %s", installCmd.PrintableCommandArgs())
		if err := installCmd.Run(); err != nil {
			logger.Debugf("Failed to install Flutter version %s (%s) with %s, error: %w", version.version, version.channel, installType.Name, err)
			return false, fmt.Errorf("failed to install Flutter version with %s, error: %w", installType.Name, err)
		}
	}

	logger.Donef("Flutter version %s (%s) installed successfully with %s", version.version, version.channel, installType.Name)
	if installType.SetDefaultCommand != nil {
		logger.Debugf("Setting Flutter version to %s (%s) with %s", version.version, version.channel, installType.Name)
		setCmd := installType.SetDefaultCommand(version)
		logger.Debugf("$ %s", setCmd.PrintableCommandArgs())

		if err := setCmd.Run(); err != nil {
			return false, fmt.Errorf("failed to set Flutter version with %s, error: %w", installType.Name, err)
		}
		logger.Donef("Flutter version %s (%s) set as default with %s", version.version, version.channel, installType.Name)
	}
	return true, nil
}

func setDefaultIfInstalled(installType *FlutterInstallType, version flutterVersion) (bool, error) {
	out, err := installType.VersionsCommand.RunAndReturnTrimmedOutput()
	if err != nil {
		logger.Debugf("Failed to list Flutter versions with %s, error: %s", installType.Name, err)
		return false, nil
	}
	logger.Debugf("Listing Flutter versions with %s: %s", installType.Name, out)

	if strings.Contains(out, version.version) {
		if installType.SetDefaultCommand != nil {
			logger.Debugf("Setting Flutter version to %s (%s) with %s", version.version, version.channel, installType.Name)
			setCmd := installType.SetDefaultCommand(version)
			logger.Debugf("$ %s", setCmd.PrintableCommandArgs())

			if err := setCmd.Run(); err != nil {
				return false, fmt.Errorf("failed to set Flutter version with %s, error: %w", installType.Name, err)
			}
		}
		return true, nil
	}

	return false, nil
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

func flutterVersionInfo() (flutterVersion, string, error) {
	logger.Println()
	versionCmd := command.New("flutter", "--version", "--machine")
	logger.Donef("$ %s", versionCmd.PrintableCommandArgs())
	logger.Println()

	out, err := versionCmd.RunAndReturnTrimmedCombinedOutput()
	logger.Debugf("Flutter version output: %s", out)
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return flutterVersion{}, out, fmt.Errorf("failed to get flutter version, error: %s, out: %s", err, out)
		}
		return flutterVersion{}, "", fmt.Errorf("failed to get flutter version, error: %w", err)
	}

	flutterVer, err := NewFlutterVersionFromFlutterMachineOutput(out)

	return flutterVer, out, err
}

func matchVersion(versionOutput string) (string, error) {
	versionRegexp := regexp.MustCompile(`\b[0-9]+\.[0-9]+\.[0-9]+(?:[-\.][A-Za-z0-9\.\-]+)?\b`)
	lines := strings.Split(versionOutput, "\n")
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), "dart") {
			continue
		}
		match := versionRegexp.FindString(line)
		if match != "" {
			return match, nil
		}
	}
	return "", fmt.Errorf("failed to parse flutter version")
}

func matchChannel(versionOutput string) (string, error) {
	channelRegexp := regexp.MustCompile(`(?i)\b(stable|beta|main|master)\b`)
	channel := channelRegexp.FindString(versionOutput)
	if channel == "" {
		return "", fmt.Errorf("failed to parse flutter channel")
	}
	return channel, nil
}
