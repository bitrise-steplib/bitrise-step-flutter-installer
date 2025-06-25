package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/bitrise-io/go-flutter/flutterproject"
)

func (f *FlutterInstaller) EnsureFlutterVersion(sdkVersions *flutterproject.FlutterAndDartSDKVersions) error {
	requiredVersion, err := f.fetchFlutterVersion(sdkVersions)
	if err != nil {
		return fmt.Errorf("fetch required Flutter version: %w", err)
	}
	f.Infof("Required Flutter version: %s channel: %s)", requiredVersion.version, requiredVersion.channel)

	installed, currentVersion := f.comapareVersionToCurrent(requiredVersion)
	if installed {
		f.Infof("Flutter version %s (%s) is already installed", requiredVersion.version,
			requiredVersion.channel)
		return nil
	}

	fvm, asdf, manual := f.NewFlutterInstallTypeFVM(), f.NewFlutterInstallTypeASDF(), f.NewFlutterInstallTypeManual()

	var primaryManager = fvm
	var secondaryManager = asdf
	if currentVersion.installType == ASDFName {
		primaryManager = asdf
		secondaryManager = fvm
	}
	var installTypes []*FlutterInstallType
	if f.Config.BundleSpecified && f.Config.BundleURL != "" {
		f.Infof("Using Flutter version from bundle: %s", f.Config.BundleURL)
		installTypes = []*FlutterInstallType{&manual, &primaryManager, &secondaryManager} // Manual install first, then FVM and ASDF as fallback
	} else {
		installTypes = []*FlutterInstallType{&primaryManager, &secondaryManager, &manual} // FVM and ASDF first, then manual install as fallback
	}

	for _, installType := range installTypes {
		if installType.CheckAvailability == nil || !installType.CheckAvailability() {
			f.Debugf("Flutter install tool %s is not available, skipping", installType.Name)
			continue
		} else {
			installType.IsAvailable = true
		}

		err := f.setDefaultIfInstalled(installType, requiredVersion)
		if err != nil {
			f.Debugf("set Flutter version with %s: %s", installType.Name, err)
		} else if installed, _ := f.comapareVersionToCurrent(requiredVersion); installed {
			f.Infof("Flutter version %s (%s) is installed and set as default with %s", requiredVersion.version, requiredVersion.channel, installType.Name)
			return nil
		}
	}

	f.Infof("Installing Flutter version: %s channel: %s...", requiredVersion.version, requiredVersion.channel)
	for _, installType := range installTypes {
		if !installType.IsAvailable {
			continue
		}

		err := f.installAndSetDefault(installType, requiredVersion)
		if err != nil {
			f.Debugf("%s", err)
		} else if installed, _ = f.comapareVersionToCurrent(requiredVersion); installed {
			f.Infof("Flutter version %s (%s) installed and set as default with %s", requiredVersion.version, requiredVersion.channel, installType.Name)
			return nil
		}
	}

	return fmt.Errorf("flutter version %s (%s) could not be installed or set as default", requiredVersion.version, requiredVersion.channel)
}

func (f *FlutterInstaller) comapareVersionToCurrent(required flutterVersion) (bool, flutterVersion) {
	currentVersion, _, err := f.flutterVersionInfo()
	if err == nil &&
		(required.version == "" || currentVersion.version == required.version) &&
		(required.channel == "" || currentVersion.channel == required.channel) {
		return true, currentVersion
	}
	return false, currentVersion
}

func (f *FlutterInstaller) fetchFlutterVersion(sdkVersions *flutterproject.FlutterAndDartSDKVersions) (flutterVersion, error) {
	if f.Config.BundleSpecified {
		parsedVersion, err := NewFlutterVersion(strings.TrimSpace(f.Config.BundleURL))
		if err == nil {
			return parsedVersion, nil
		}
	}
	parsedVersion, err := NewFlutterVersion(strings.TrimSpace(f.Config.Version))
	if err == nil {
		return parsedVersion, nil
	}

	f.Warnf("parse required Flutter version: %s: %w", f.Config.Version, err)

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

func (f *FlutterInstaller) installAndSetDefault(installType *FlutterInstallType, version flutterVersion) error {
	f.Debugf("Installing Flutter version %s (%s) with %s", version.version, version.channel, installType.Name)
	if installType.FullInstall != nil {
		if err := installType.FullInstall(); err != nil {
			return fmt.Errorf("install Flutter version (%s %s) with %s full install: %w", version.version, version.channel, installType.Name, err)
		}
	} else {
		installCmd := installType.InstallCommand(version)
		f.Debugf("$ %s", installCmd.PrintableCommandArgs())
		if out, err := installCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
			return fmt.Errorf("install Flutter version (%s %s) with %s: %w, out: %s", version.version, version.channel, installType.Name, err, out)
		}
	}

	f.Donef("Flutter version %s %s installed successfully with %s", version.version, version.channel, installType.Name)
	if installType.SetDefaultCommand != nil {
		f.Debugf("Setting Flutter version to %s %s with %s", version.version, version.channel, installType.Name)
		setCmd := *installType.SetDefaultCommand(version)
		f.Debugf("$ %s", setCmd.PrintableCommandArgs)

		if out, err := setCmd.RunAndReturnTrimmedOutput(); err != nil {
			return fmt.Errorf("set Flutter version with %s: %s", installType.Name, out)
		}
	}
	return nil
}

func (f *FlutterInstaller) setDefaultIfInstalled(installType *FlutterInstallType, version flutterVersion) error {
	out, err := installType.VersionsCommand.RunAndReturnTrimmedOutput()
	if err != nil {
		f.Debugf("list Flutter versions with %s: %s", installType.Name, err)
		return nil
	}
	f.Debugf("Listing Flutter versions with %s: %s", installType.Name, out)

	if strings.Contains(out, version.version) {
		if installType.SetDefaultCommand != nil {
			f.Debugf("Setting Flutter version to %s %s with %s", version.version, version.channel, installType.Name)
			setCmd := *installType.SetDefaultCommand(version)
			f.Debugf("$ %s", setCmd.PrintableCommandArgs())

			if err := setCmd.Run(); err != nil {
				return fmt.Errorf("set Flutter version with %s: %w", installType.Name, err)
			}
		}
		return nil
	}

	return nil
}

func (f *FlutterInstaller) flutterVersionInfo() (flutterVersion, string, error) {
	versionCmd := f.CmdFactory.Create("flutter", []string{"--version", "--machine"}, nil)
	f.Donef("$ %s", versionCmd.PrintableCommandArgs())

	out, err := versionCmd.RunAndReturnTrimmedCombinedOutput()
	f.Debugf("Flutter version output: %s", out)
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
