package tracker

import (
	"github.com/bitrise-io/go-flutter/flutterproject"
	"github.com/bitrise-io/go-utils/v2/analytics"
	"github.com/bitrise-io/go-utils/v2/env"
	"github.com/bitrise-io/go-utils/v2/log"
)

type StepTracker struct {
	tracker analytics.Tracker
	logger  log.Logger
}

func NewStepTracker(logger log.Logger, envRepo env.Repository) StepTracker {
	return StepTracker{
		tracker: analytics.NewDefaultTracker(logger, envRepo),
		logger:  logger,
	}
}

func (t *StepTracker) LogSDKVersions(projectSDKVersions flutterproject.FlutterAndDartSDKVersions) {
	p := projectSDKVersionsToProperties(projectSDKVersions)
	t.tracker.Enqueue("step_flutter_installer_project_sdk_versions", p)
}

func (t *StepTracker) Wait() {
	t.tracker.Wait()
}

func projectSDKVersionsToProperties(projectSDKVersions flutterproject.FlutterAndDartSDKVersions) analytics.Properties {
	p := analytics.Properties{}

	if projectSDKVersions.FVMFlutterVersion != nil {
		p["flutter_sdk_fvm_config_json"] = projectSDKVersions.FVMFlutterVersion.String()
	}
	if projectSDKVersions.ASDFFlutterVersion != nil {
		p["flutter_sdk_tool_versions"] = projectSDKVersions.ASDFFlutterVersion.String()
	}
	if projectSDKVersions.PubspecLockFlutterVersion != nil {
		p["flutter_sdk_pubspec_lock"] = projectSDKVersions.PubspecLockFlutterVersion.String()
	}
	if projectSDKVersions.PubspecFlutterVersion != nil {
		p["flutter_sdk_pubspec_yaml"] = projectSDKVersions.PubspecFlutterVersion.String()
	}
	if projectSDKVersions.PubspecLockDartVersion != nil {
		p["dart_sdk_pubspec_lock"] = projectSDKVersions.PubspecLockDartVersion.String()
	}
	if projectSDKVersions.PubspecDartVersion != nil {
		p["dart_sdk_pubspec_yaml"] = projectSDKVersions.PubspecDartVersion.String()
	}

	return p
}
