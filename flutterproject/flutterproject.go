package flutterproject

import "github.com/Masterminds/semver/v3"

type Project struct {
	rootDir    string
	fileOpener FileOpener
}

func New(rootDir string, fileOpener FileOpener) Project {
	return Project{
		rootDir:    rootDir,
		fileOpener: fileOpener,
	}
}

type FlutterAndDartSDKVersions struct {
	FVMFlutterVersion         *semver.Version
	ASDFFlutterVersion        *semver.Version
	PubspecFlutterVersion     *VersionConstraint
	PubspecDartVersion        *VersionConstraint
	PubspecLockFlutterVersion *VersionConstraint
	PubspecLockDartVersion    *VersionConstraint
}

func (p Project) FlutterAndDartSDKVersions() (FlutterAndDartSDKVersions, error) {
	sdkVersions := FlutterAndDartSDKVersions{}

	fvmFlutterVersion, err := NewFVMVersionReader(p.fileOpener).ReadSDKVersion(p.rootDir)
	if err != nil {
		return FlutterAndDartSDKVersions{}, err
	} else {
		sdkVersions.FVMFlutterVersion = fvmFlutterVersion
	}

	asdfFlutterVersion, err := NewASDFVersionReader(p.fileOpener).ReadSDKVersions(p.rootDir)
	if err != nil {
		return FlutterAndDartSDKVersions{}, err
	} else {
		sdkVersions.ASDFFlutterVersion = asdfFlutterVersion
	}

	pubspecLockFlutterVersion, pubspecLockDartVersion, err := NewPubspecLockVersionReader(p.fileOpener).ReadSDKVersions(p.rootDir)
	if err != nil {
		return FlutterAndDartSDKVersions{}, err
	} else {
		sdkVersions.PubspecLockFlutterVersion = pubspecLockFlutterVersion
		sdkVersions.PubspecLockDartVersion = pubspecLockDartVersion
	}

	pubspecFlutterVersion, pubspecDartVersion, err := NewPubspecVersionReader(p.fileOpener).ReadSDKVersions(p.rootDir)
	if err != nil {
		return FlutterAndDartSDKVersions{}, err
	} else {
		sdkVersions.PubspecFlutterVersion = pubspecFlutterVersion
		sdkVersions.PubspecDartVersion = pubspecDartVersion
	}

	return sdkVersions, nil
}
