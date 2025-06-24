package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/bitrise-io/go-flutter/flutterproject"
)

func EnsureFlutterVersion(cfg *Config, sdkVersions *flutterproject.FlutterAndDartSDKVersions) error {
	requiredVersion, err := fetchFlutterVersion(cfg, sdkVersions)
	if err != nil {
		return fmt.Errorf("fetch required Flutter version: %w", err)
	}
	logger.Infof("Required Flutter version: %s (%s)", requiredVersion.version, requiredVersion.channel)

	installed, currentVersion := comapareVersionToCurrent(requiredVersion)
	if installed {
		logger.Infof("Flutter version %s (%s) is already installed", requiredVersion.version,
			requiredVersion.channel)
		return nil
	}

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

	for _, installType := range installTypes {
		logger.Debugf("Checking availability of Flutter install tool: %s", installType.Name)
		if installType.CheckAvailability == nil || !installType.CheckAvailability() {
			logger.Debugf("Flutter install tool %s is not available, skipping", installType.Name)
			continue
		} else {
			installType.IsAvailable = true
		}

		err := setDefaultIfInstalled(installType, requiredVersion)
		if err != nil {
			logger.Debugf("set Flutter version with %s: %s", installType.Name, err)
		} else if installed, _ := comapareVersionToCurrent(requiredVersion); installed {
			logger.Infof("Flutter version %s (%s) is installed and set as default with %s", requiredVersion.version, requiredVersion.channel, installType.Name)
			return nil
		}
	}

	logger.Infof("Flutter version %s (%s) is not installed, installing...", requiredVersion.version, requiredVersion.channel)
	for _, installType := range installTypes {
		if !installType.IsAvailable {
			logger.Debugf("Flutter install tool %s is not available, skipping", installType.Name)
			continue
		}

		err := installAndSetDefault(installType, requiredVersion, cfg)
		if err != nil {
			logger.Debugf("%s", err)
		} else if installed, _ = comapareVersionToCurrent(requiredVersion); installed {
			logger.Infof("Flutter version %s (%s) installed and set as default with %s", requiredVersion.version, requiredVersion.channel, installType.Name)
			return nil
		}
	}

	return fmt.Errorf("flutter version %s (%s) could not be installed or set as default", requiredVersion.version, requiredVersion.channel)
}

func comapareVersionToCurrent(required flutterVersion) (bool, flutterVersion) {
	currentVersion, _, err := flutterVersionInfo()
	if err == nil &&
		(required.version == "" || currentVersion.version == required.version) &&
		(required.channel == "" || currentVersion.channel == required.channel) {
		return true, currentVersion
	}
	return false, currentVersion
}

func fetchFlutterVersion(cfg *Config, sdkVersions *flutterproject.FlutterAndDartSDKVersions) (flutterVersion, error) {
	if cfg.BundleSpecified {
		parsedVersion, err := NewFlutterVersion(strings.TrimSpace(cfg.BundleURL))
		if err == nil {
			return parsedVersion, nil
		}
	}
	parsedVersion, err := NewFlutterVersion(strings.TrimSpace(cfg.Version))
	if err == nil {
		return parsedVersion, nil
	}

	logger.Warnf("parse required Flutter version: %s: %w", cfg.Version, err)

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

func installAndSetDefault(installType *FlutterInstallType, version flutterVersion, cfg *Config) error {
	logger.Debugf("Installing Flutter version %s (%s) with %s", version.version, version.channel, installType.Name)
	if installType.FullInstall != nil {
		if err := installType.FullInstall(cfg); err != nil {
			return fmt.Errorf("install Flutter version (%s %s) with %s full install: %w", version.version, version.channel, installType.Name, err)
		}
	} else {
		installCmd := installType.InstallCommand(version)
		logger.Debugf("$ %s", installCmd.PrintableCommandArgs())
		if out, err := installCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
			return fmt.Errorf("install Flutter version (%s %s) with %s: %w, out: %s", version.version, version.channel, installType.Name, err, out)
		}
	}

	logger.Donef("Flutter version %s %s installed successfully with %s", version.version, version.channel, installType.Name)
	if installType.SetDefaultCommand != nil {
		logger.Debugf("Setting Flutter version to %s %s with %s", version.version, version.channel, installType.Name)
		setCmd := *installType.SetDefaultCommand(version)
		logger.Debugf("$ %s", setCmd.PrintableCommandArgs)

		if out, err := setCmd.RunAndReturnTrimmedOutput(); err != nil {
			return fmt.Errorf("set Flutter version with %s: %s", installType.Name, out)
		}
	}
	return nil
}

func setDefaultIfInstalled(installType *FlutterInstallType, version flutterVersion) error {
	out, err := installType.VersionsCommand.RunAndReturnTrimmedOutput()
	if err != nil {
		logger.Debugf("list Flutter versions with %s: %s", installType.Name, err)
		return nil
	}
	logger.Debugf("Listing Flutter versions with %s: %s", installType.Name, out)

	if strings.Contains(out, version.version) {
		if installType.SetDefaultCommand != nil {
			logger.Debugf("Setting Flutter version to %s %s with %s", version.version, version.channel, installType.Name)
			setCmd := *installType.SetDefaultCommand(version)
			logger.Debugf("$ %s", setCmd.PrintableCommandArgs())

			if err := setCmd.Run(); err != nil {
				return fmt.Errorf("set Flutter version with %s: %w", installType.Name, err)
			}
		}
		return nil
	}

	return nil
}

func flutterVersionInfo() (flutterVersion, string, error) {
	logger.Println()
	versionCmd := cmdFactory.Create("flutter", []string{"--version", "--machine"}, nil)
	logger.Donef("$ %s", versionCmd.PrintableCommandArgs())
	logger.Println()

	out, err := versionCmd.RunAndReturnTrimmedCombinedOutput()
	logger.Debugf("Flutter version output: %s", out)
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return flutterVersion{}, out, fmt.Errorf("get flutter version: %s, out: %s", err, out)
		}
		return flutterVersion{}, "", fmt.Errorf("get flutter version: %w", err)
	}

	flutterVer, err := NewFlutterVersion(out)

	return flutterVer, out, err
}
