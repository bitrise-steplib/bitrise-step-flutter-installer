package main

import (
	"fmt"
	"strings"
)

func (f *FlutterInstaller) EnsureFlutterVersion() error {
	requiredVersion, err := f.NewFlutterVersionFromInputAndProject()
	if err != nil {
		return fmt.Errorf("fetch required Flutter version: %w", err)
	}
	f.Infof("Required Flutter: %s", f.NewVersionString(requiredVersion))

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
		} else {
			f.Debugf("Set Flutter %s default if already installed: %s", currentVersionString, err)
		}
	}

	for _, installType := range installTypes {
		if err := f.installAndSetDefault(installType, requiredVersion); err == nil {
			f.Donef("Installed and set default Flutter %s with %s", currentVersionString, installType.Name)
			return nil
		} else {
			f.Debugf("Install and set default Flutter %s: %s", currentVersionString, err)
		}
	}

	return fmt.Errorf("installing Flutter %s: could not be installed or set as default", currentVersionString)
}

// compareVersionToCurrent compares the required version to the current version.
// If strict is true, it checks if both version and channel match exactly (if not empty).
func (f *FlutterInstaller) compareVersionToCurrent(required flutterVersion, strict bool) (bool, flutterVersion) {
	currentVersion, err := f.NewFlutterVersionFromCurrent()
	if err != nil {
		f.Debugf("get current Flutter version: %s", err)
	} else if strict {
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
	if installType.ReleasesCommand != nil {
		releasesCmd := *installType.ReleasesCommand(required)
		if out, err := releasesCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
			return false, fmt.Errorf("list releases: %s", out)
		} else if contains, err := f.containsVersion(out, required); err != nil {
			return false, fmt.Errorf("check if releases contains version: %w", err)
		} else if !contains {
			return false, fmt.Errorf("not available")
		} else {
			f.Debugf("Flutter %s - %s is present in releases output", f.NewVersionString(required), installType.Name)
			return true, nil
		}
	}

	return false, fmt.Errorf("no releases listing command defined for tool %s", installType.Name)
}
func (f *FlutterInstaller) hasInstalled(installType *FlutterInstallType, required flutterVersion) (bool, error) {
	if installType.InstalledVersionsCommand != nil {
		installsCmd := *installType.InstalledVersionsCommand()
		if out, err := installsCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
			return false, fmt.Errorf("list instances: %s", out)
		} else if contains, err := f.containsVersion(out, required); err != nil {
			return false, fmt.Errorf("check if installs contains version: %w", err)
		} else if !contains {
			return false, fmt.Errorf("version: %s channel: %s is not available in installed instances output: %s", required.version, required.channel, out)
		} else {
			f.Debugf("Version: %s channel: %s is available in installed instances output", required.version, required.channel)
			return true, nil
		}
	}

	return false, fmt.Errorf("no installed versions command defined for %s", installType.Name)
}

func (f *FlutterInstaller) containsVersion(output string, required flutterVersion) (bool, error) {
	if output != "" {
		versions, err := NewFlutterVersionList(output)
		if err != nil {
			return false, fmt.Errorf("parse releases: %w", err)
		} else if len(versions) == 0 {
			return false, fmt.Errorf("no versions available in releases output: %s", output)
		} else {
			for _, v := range versions {
				if (required.version == "" || required.version == v.version) &&
					(required.channel == "" || required.channel == v.channel) {
					return true, nil
				}
			}
		}
	}

	return false, fmt.Errorf("output is empty or doesnt contain version")
}

func (f *FlutterInstaller) ensureSetupFinished() error {
	finsihSetupCmd := f.CmdFactory.Create("flutter", []string{"--version"}, nil)
	f.Donef("$ %s", finsihSetupCmd.PrintableCommandArgs())
	if out, err := finsihSetupCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
		return fmt.Errorf("check flutter version after setup: %s %s", err, out)
	} else {
		f.Debugf("Flutter version output after install: %s", out)
	}
	return nil
}

func (f *FlutterInstaller) installAndSetDefault(installType *FlutterInstallType, required flutterVersion) error {
	if installType.Install == nil {
		return fmt.Errorf("no install command defined")
	}

	f.Debugf("Installing version: %s channel: %s with %s", required.version, required.channel, installType.Name)

	if installType.ReleasesCommand != nil {
		if hasRelease, err := f.hasRelease(installType, required); err != nil {
			return fmt.Errorf("seaching for version in releases: %w", err)
		} else if !hasRelease {
			return fmt.Errorf("tool %s does not provide required version", installType.Name)
		}
	}

	if err := installType.Install(required); err != nil {
		return fmt.Errorf("install: %s", err)
	} else if err := f.ensureSetupFinished(); err != nil {
		f.Debugf("ensure setup is finished: %s", err)
	}

	if installType.SetDefault != nil {
		if err := installType.SetDefault(required); err != nil {
			return fmt.Errorf("set version default: %s", err)
		} else if err := f.ensureSetupFinished(); err != nil {
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
	} else {
		listCmd := *installType.InstalledVersionsCommand()
		f.Donef("$ %s", listCmd.PrintableCommandArgs())
		out, err := listCmd.RunAndReturnTrimmedOutput()
		f.Debugf("list Flutter versions with %s: %s %s", installType.Name, err, out)

		return fmt.Errorf("version does not match required version after installing with %s", installType.Name)
	}
}

func (f *FlutterInstaller) setDefaultIfInstalled(installType *FlutterInstallType, required flutterVersion) error {
	if hasRelease, err := f.hasInstalled(installType, required); err != nil {
		return fmt.Errorf("seaching for version in list of installed: %w", err)
	} else if !hasRelease {
		return fmt.Errorf("tool %s does not provide required version", installType.Name)
	}

	if installType.SetDefault != nil {
		if err := installType.SetDefault(required); err != nil {
			return fmt.Errorf("set version default: %s", err)
		} else if err := f.ensureSetupFinished(); err != nil {
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
	} else {
		return fmt.Errorf("version does not match required version after setting it default with %s", installType.Name)
	}
}
