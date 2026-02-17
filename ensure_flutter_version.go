package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/bitrise-io/go-flutter/flutterproject"
	"github.com/bitrise-io/go-flutter/fluttersdk"
	"github.com/bitrise-io/go-utils/v2/fileutil"
	"github.com/bitrise-io/go-utils/v2/pathutil"
)

// EnssureFlutterVersion ensures that the required Flutter version is installed and set as default.
//
// It gets the required version from the input or project files, checks if it is already installed,
// and installs it using the available install types (FVM, ASDF, Manual).
func (f *FlutterInstaller) EnsureFlutterVersion() error {
	requiredVersion, err := f.NewFlutterVersionFromInputAndProject()
	if err != nil {
		return fmt.Errorf("fetch required Flutter version: %w", err)
	}
	f.Infof("Required Flutter: %s", f.NewVersionString(requiredVersion))

	// Channel-only input (e.g. "stable"): check if current version satisfies project constraints
	if requiredVersion.version == "" && requiredVersion.channel != "" {
		requiredVersion = f.resolveVersionFromConstraints(requiredVersion)
	}

	currentVersionString := f.NewVersionString(requiredVersion)
	installed, currentVersion := f.compareVersionToCurrent(requiredVersion, true)
	if installed {
		f.Donef("Flutter %s is already installed", currentVersionString)
		return nil
	}

	fvm, asdf, manual := f.NewFlutterInstallTypeFVM(), f.NewFlutterInstallTypeASDF(), f.NewFlutterInstallTypeManual()
	installTypes := []*FlutterInstallType{}
	switch currentVersion.installType {
	case ASDFName:
		if asdf.IsAvailable {
			installTypes = append(installTypes, &asdf)
		}
		if fvm.IsAvailable {
			installTypes = append(installTypes, &fvm)
		}
	default:
		if fvm.IsAvailable {
			installTypes = append(installTypes, &fvm)
		}
		if asdf.IsAvailable {
			installTypes = append(installTypes, &asdf)
		}
	}
	if manual.IsAvailable {
		installTypes = append(installTypes, &manual)
	}

	for _, installType := range installTypes {
		if err := f.setDefaultIfInstalled(installType, requiredVersion); err == nil {
			f.Donef("Flutter %s is already installed and set as default with %s", currentVersionString, installType.Name)
			return nil
		}
		f.Debugf("Set Flutter %s default if already installed: %s", currentVersionString, err)
	}

	for _, installType := range installTypes {
		if err := f.installAndSetDefault(installType, requiredVersion); err == nil {
			f.Donef("Installed and set default Flutter %s with %s", currentVersionString, installType.Name)
			return nil
		}
		f.Debugf("Install and set default Flutter %s: %s", currentVersionString, err)
	}

	return fmt.Errorf("installing Flutter %s: could not be installed or set as default", currentVersionString)
}

// resolveVersionFromConstraints checks if the current Flutter/Dart version satisfies project constraints.
// If it does, returns the required version unchanged (channel-only match will work).
// If it doesn't, finds the latest compatible version and returns it with a specific version set.
func (f *FlutterInstaller) resolveVersionFromConstraints(required flutterVersion) flutterVersion {
	proj, err := flutterproject.New("./", fileutil.NewFileManager(), pathutil.NewPathChecker(), fluttersdk.NewSDKVersionFinder())
	if err != nil {
		f.Debugf("Could not open project: %s", err)
		return required
	}

	sdkVersions, err := proj.FlutterAndDartSDKVersions()
	if err != nil {
		f.Debugf("Could not read SDK versions: %s", err)
		return required
	}

	// Collect constraints from pubspec.lock (preferred) or pubspec.yaml
	var flutterConstraint *semver.Constraints
	var dartConstraint *semver.Constraints

	if v := sdkVersions.PubspecLockFlutterVersion; v != nil && v.Constraint != nil {
		flutterConstraint = v.Constraint
	} else if v := sdkVersions.PubspecFlutterVersion; v != nil && v.Constraint != nil {
		flutterConstraint = v.Constraint
	}

	if v := sdkVersions.PubspecLockDartVersion; v != nil && v.Constraint != nil {
		dartConstraint = v.Constraint
	} else if v := sdkVersions.PubspecDartVersion; v != nil && v.Constraint != nil {
		dartConstraint = v.Constraint
	}

	// No constraints in project files → channel-only is fine
	if flutterConstraint == nil && dartConstraint == nil {
		return required
	}

	// Check if current version satisfies constraints
	currentVersion, err := f.NewFlutterVersionFromCurrent()
	if err == nil {
		currentSatisfies := true

		if flutterConstraint != nil && currentVersion.version != "" {
			if v, err := semver.NewVersion(currentVersion.version); err == nil && !flutterConstraint.Check(v) {
				f.Debugf("Current Flutter %s does not satisfy constraint %s", currentVersion.version, flutterConstraint)
				currentSatisfies = false
			}
		}

		if currentSatisfies && dartConstraint != nil && currentVersion.dartVersion != "" {
			dartVersionStr := currentVersion.dartVersion
			// Handle "3.9.0 (build 3.9.0-100.2.beta)" format
			if matches := regexp.MustCompile(`(.+) \(build (.+)\)`).FindStringSubmatch(dartVersionStr); len(matches) == 3 {
				dartVersionStr = matches[1]
			}
			if v, err := semver.NewVersion(dartVersionStr); err == nil && !dartConstraint.Check(v) {
				f.Debugf("Current Dart %s does not satisfy constraint %s", dartVersionStr, dartConstraint)
				currentSatisfies = false
			}
		}

		if currentSatisfies {
			f.Debugf("Current Flutter %s (Dart %s) satisfies project constraints", currentVersion.version, currentVersion.dartVersion)
			return required
		}
	}

	// Current version doesn't satisfy → find compatible version
	f.Infof("Current Flutter version does not satisfy project constraints, searching for compatible version...")
	version, channel, err := proj.FlutterSDKVersionToUse()
	if err == nil && version != "" {
		f.Infof("Found compatible Flutter %s (%s)", version, channel)
		result := required
		result.version = version
		if channel != "" {
			result.channel = channel
		}
		return result
	}

	f.Warnf("Could not find compatible Flutter version for project constraints")
	return required
}

// compareVersionToCurrent compares the required Flutter version to the current version.
// If strict is true, both version and channel must match exactly (if not empty).
func (f *FlutterInstaller) compareVersionToCurrent(required flutterVersion, strict bool) (bool, flutterVersion) {
	currentVersion, err := f.NewFlutterVersionFromCurrent()
	if err != nil {
		f.Debugf("get current Flutter version: %s", err)
		return false, currentVersion
	}

	if strict {
		if (required.version == "" || currentVersion.version == required.version) &&
			(required.channel == "" || currentVersion.channel == required.channel) {
			return true, currentVersion
		}
	} else {
		if required.version != "" && currentVersion.version == required.version {
			return true, currentVersion
		} else if required.channel != "" && currentVersion.channel == required.channel {
			return true, currentVersion
		}
	}
	return false, currentVersion
}

func (f *FlutterInstaller) hasRelease(installType *FlutterInstallType, required flutterVersion) (bool, error) {
	if installType.ReleasesCommand == nil {
		return false, fmt.Errorf("no releases command defined for tool %s", installType.Name)
	}

	releasesCmd := *installType.ReleasesCommand(required)
	out, err := releasesCmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return false, fmt.Errorf("list releases: %s", out)
	}

	contains, err := f.containsVersion(out, required)
	if err != nil {
		return false, fmt.Errorf("check if releases contains version: %w", err)
	}
	if !contains {
		return false, fmt.Errorf("not available")
	}

	f.Debugf("Flutter %s - %s is present in releases output", f.NewVersionString(required), installType.Name)
	return true, nil
}

func (f *FlutterInstaller) hasInstalled(installType *FlutterInstallType, required flutterVersion) (bool, error) {
	if installType.InstalledVersionsCommand == nil {
		return false, fmt.Errorf("no installed versions command defined for tool %s", installType.Name)
	}

	installsCmd := *installType.InstalledVersionsCommand()
	out, err := installsCmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return false, fmt.Errorf("list instances: %s", out)
	}

	contains, err := f.containsVersion(out, required)
	if err != nil {
		return false, fmt.Errorf("check if installs contains version: %w", err)
	}
	if !contains {
		return false, fmt.Errorf("version: %s channel: %s is not available in installed instances output: %s", required.version, required.channel, out)
	}

	f.Debugf("Version: %s channel: %s is available in installed instances output", required.version, required.channel)
	return true, nil
}

// containsVersion checks if the output of installed/released versions contains the required version and channel.
func (f *FlutterInstaller) containsVersion(output string, required flutterVersion) (bool, error) {
	if output == "" {
		return false, fmt.Errorf("output is empty")
	}

	versions, err := NewFlutterVersionList(output)
	if err != nil {
		return false, fmt.Errorf("parse releases: %w", err)
	}
	if len(versions) == 0 {
		return false, fmt.Errorf("no versions available in releases output: %s", output)
	}

	for _, v := range versions {
		if (required.version == "" || required.version == v.version) &&
			(required.channel == "" || required.channel == v.channel) {
			return true, nil
		}
	}

	return false, fmt.Errorf("output does not contain version")
}

// ensureSetupFinished makes sure that the Dart SDK is set up correctly after installation.
// This can be done by calling `flutter --version` which initializes the Dart SDK, if needed.
func (f *FlutterInstaller) ensureSetupFinished() error {
	finsihSetupCmd := f.CmdFactory.Create("flutter", []string{"--version"}, nil)
	f.Donef("$ %s", finsihSetupCmd.PrintableCommandArgs())
	out, err := finsihSetupCmd.RunAndReturnTrimmedCombinedOutput()
	if err != nil {
		return fmt.Errorf("check flutter version after setup: %s %s", err, out)
	}
	f.Debugf("Flutter version output after install: %s", out)

	return nil
}

// installAndSetDefault installs the required Flutter version using the specified install type.
//
// Before installing, it checks if the version is available in releases (if applicable).
// After installation, it sets the version as default (if applicable).
// It checks installation success by comparing the installed version to the required version.
func (f *FlutterInstaller) installAndSetDefault(installType *FlutterInstallType, required flutterVersion) error {
	if installType.Install == nil {
		return fmt.Errorf("no install command defined")
	}

	f.Debugf("Installing version: %s channel: %s with %s", required.version, required.channel, installType.Name)

	if installType.ReleasesCommand != nil {
		hasRelease, err := f.hasRelease(installType, required)
		if err != nil {
			return fmt.Errorf("seaching for version in releases: %w", err)
		}
		if !hasRelease {
			return fmt.Errorf("tool %s does not provide required version", installType.Name)
		}
	}

	if err := installType.Install(required); err != nil {
		return fmt.Errorf("install: %s", err)
	}
	if err := f.ensureSetupFinished(); err != nil {
		f.Debugf("ensure setup is finished: %s", err)
	}

	if installType.SetDefault != nil {
		if err := installType.SetDefault(required); err != nil {
			return fmt.Errorf("set version default: %s", err)
		}
		if err := f.ensureSetupFinished(); err != nil {
			f.Debugf("ensure setup is finished: %s", err)
		}
	}

	requiredTrimmed := flutterVersion{
		version:     strings.TrimPrefix(required.version, "v"),
		channel:     required.channel,
		installType: installType.Name,
	}
	if installed, _ := f.compareVersionToCurrent(requiredTrimmed, false); installed {
		return nil
	}

	return fmt.Errorf("version does not match required version after installing with %s", installType.Name)
}

// setDefaultIfInstalled checks if the required Flutter version is already installed using the specified install type.
//
// If it is installed, it sets the version as default (if applicable).
// It checks success by comparing the installed version to the required version.
func (f *FlutterInstaller) setDefaultIfInstalled(installType *FlutterInstallType, required flutterVersion) error {
	hasRelease, err := f.hasInstalled(installType, required)
	if err != nil {
		return fmt.Errorf("seaching for version in list of installed: %w", err)
	}
	if !hasRelease {
		return fmt.Errorf("tool %s does not provide required version", installType.Name)
	}

	if installType.SetDefault != nil {
		if err := installType.SetDefault(required); err != nil {
			return fmt.Errorf("set version default: %s", err)
		}
		if err := f.ensureSetupFinished(); err != nil {
			f.Debugf("ensure setup is finished: %s", err)
		}
	}

	requiredTrimmed := flutterVersion{
		version:     strings.TrimPrefix(required.version, "v"),
		channel:     required.channel,
		installType: installType.Name,
	}
	if installed, _ := f.compareVersionToCurrent(requiredTrimmed, true); installed {
		return nil
	}

	return fmt.Errorf("version does not match required version after setting it default with %s", installType.Name)
}
