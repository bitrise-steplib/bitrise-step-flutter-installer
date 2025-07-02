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
	f.Infof("Required version: %s channel: %s", requiredVersion.version, requiredVersion.channel)

	installed, currentVersion := f.compareVersionToCurrent(requiredVersion)
	if installed {
		f.Donef("Flutter version %s, channel: %s is already installed", requiredVersion.version, requiredVersion.channel)
		return nil
	}

	fvm, asdf, manual := f.NewFlutterInstallTypeFVM(), f.NewFlutterInstallTypeASDF(), f.NewFlutterInstallTypeManual()

	availableManagers := []*FlutterInstallType{}
	switch currentVersion.installType {
	case ASDFName:
		if asdf.IsAvailable {
			availableManagers = append(availableManagers, &asdf)
		}
		if fvm.IsAvailable {
			availableManagers = append(availableManagers, &fvm)
		}
	default:
		if fvm.IsAvailable {
			availableManagers = append(availableManagers, &fvm)
		}
		if asdf.IsAvailable {
			availableManagers = append(availableManagers, &asdf)
		}
	}

	installTypes := []*FlutterInstallType{}
	if strings.TrimSpace(f.Config.BundleURL) != "" {
		f.Infof("Using Flutter version from bundle: %s", f.Config.BundleURL)
		if manual.IsAvailable {
			installTypes = append(installTypes, &manual)
		}
		installTypes = append(installTypes, availableManagers...)
	} else {
		installTypes = append(installTypes, availableManagers...)
		if manual.IsAvailable {
			installTypes = append(installTypes, &manual)
		}
	}

	for _, installType := range installTypes {
		if err := f.setDefaultIfInstalled(installType, requiredVersion); err == nil {
			return nil
		} else {
			f.Debugf("set default if installed with %s: %s", installType.Name, err)
		}
	}

	for _, installType := range installTypes {
		if err := f.installAndSetDefault(installType, requiredVersion); err == nil {
			return nil
		} else {
			f.Debugf("install and set default with %s: %s", installType.Name, err)
		}
	}

	return fmt.Errorf("version: %s channel: %s could not be installed or set as default", requiredVersion.version, requiredVersion.channel)
}

func (f *FlutterInstaller) compareVersionToCurrent(required flutterVersion) (bool, flutterVersion) {
	currentVersion, _, err := f.NewFlutterVersionFromCurrent()
	if err != nil {
		f.Debugf("get current Flutter version: %s", err)
	} else if (required.version == "" || currentVersion.version == required.version) &&
		(required.channel == "" || currentVersion.channel == required.channel) {
		return true, currentVersion
	}
	return false, currentVersion
}

func (f *FlutterInstaller) hasRelease(installType *FlutterInstallType, required flutterVersion) (bool, error) {
	f.Debugf("Checking if version: %s channel: %s is available with %s", required.version, required.channel, installType.Name)
	if installType.ReleasesCommand != nil {
		releasesCmd := *installType.ReleasesCommand(required)
		f.Debugf("$ %s", releasesCmd.PrintableCommandArgs())
		if out, err := releasesCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
			return false, fmt.Errorf("list releases: %s", out)
		} else if contains, err := f.containsVersion(out, required); err != nil {
			return false, fmt.Errorf("check if releases contains version: %w", err)
		} else if !contains {
			return false, fmt.Errorf("version: %s channel: %s is not available in releases output: %s", required.version, required.channel, out)
		} else {
			f.Debugf("Version: %s channel: %s is available in releases output", required.version, required.channel)
			return true, nil
		}
	}

	return false, fmt.Errorf("no releases command defined for %s", installType.Name)
}
func (f *FlutterInstaller) hasInstalled(installType *FlutterInstallType, required flutterVersion) (bool, error) {
	f.Debugf("Checking if version: %s channel: %s is available with %s", required.version, required.channel, installType.Name)
	if installType.InstalledVersionsCommand != nil {
		installsCmd := installType.InstalledVersionsCommand
		f.Debugf("$ %s", installsCmd.PrintableCommandArgs())
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
		versions, err := NewFlutterVersions(output)
		if err != nil {
			return false, fmt.Errorf("parse releases: %w", err)
		} else if len(versions) == 0 {
			return false, fmt.Errorf("no versions available in releases output: %s", output)
		} else {
			f.Debugf("Available versions: %v", versions)
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

func (f *FlutterInstaller) installAndSetDefault(installType *FlutterInstallType, required flutterVersion) error {
	f.Debugf("Installing version: %s channel: %s with %s", required.version, required.channel, installType.Name)
	if installType.FullInstall != nil {
		if err := installType.FullInstall(); err != nil {
			return fmt.Errorf("full install: %w", err)
		}
	} else if installType.InstallCommand != nil {
		if hasRelease, err := f.hasRelease(installType, required); err != nil {
			return fmt.Errorf("check if release exists: %w", err)
		} else if !hasRelease {
			return fmt.Errorf("version: %s channel: %s is not available with %s", required.version, required.channel, installType.Name)
		}

		installCmd := *installType.InstallCommand(required)
		f.Debugf("$ %s", installCmd.PrintableCommandArgs())
		if out, err := installCmd.RunAndReturnTrimmedCombinedOutput(); err != nil {
			return fmt.Errorf("install: %s", out)
		}
	} else {
		return fmt.Errorf("no install command defined")
	}

	if installType.SetDefaultCommand != nil {
		setCmd := *installType.SetDefaultCommand(required)
		f.Debugf("$ %s", setCmd.PrintableCommandArgs())
		if out, err := setCmd.RunAndReturnTrimmedOutput(); err != nil {
			return fmt.Errorf("set version default: %s", out)
		}
	}

	if installed, _ := f.compareVersionToCurrent(required); installed {
		f.Donef("Version: %s channel: %s set as default with %s", required.version, required.channel, installType.Name)
		return nil
	} else {
		listCmd := installType.InstalledVersionsCommand
		f.Debugf("$ %s", listCmd.PrintableCommandArgs())
		out, err := listCmd.RunAndReturnTrimmedOutput()
		f.Debugf("list Flutter versions with %s: %s %s", installType.Name, err, out)

		return fmt.Errorf("version: %s channel: %s could not be installed or set as default with %s", required.version, required.channel, installType.Name)
	}
}

func (f *FlutterInstaller) setDefaultIfInstalled(installType *FlutterInstallType, required flutterVersion) error {
	f.Debugf("Checking if version: %s channel: %s is installed with %s", required.version, required.channel, installType.Name)

	if hasRelease, err := f.hasInstalled(installType, required); err != nil {
		return fmt.Errorf("check if installed exists: %w", err)
	} else if !hasRelease {
		return fmt.Errorf("version: %s channel: %s is not available with %s", required.version, required.channel, installType.Name)
	}

	setCmd := *installType.SetDefaultCommand(required)
	f.Debugf("$ %s", setCmd.PrintableCommandArgs())
	if out, err := setCmd.RunAndReturnTrimmedOutput(); err != nil {
		return fmt.Errorf("set version default: %s", out)
	}

	if installed, _ := f.compareVersionToCurrent(required); installed {
		f.Donef("Version: %s channel: %s set as default with %s", required.version, required.channel, installType.Name)
		return nil
	} else {
		return fmt.Errorf("version: %s channel: %s is not installed or cannot be set default", required.version, required.channel)
	}
}
