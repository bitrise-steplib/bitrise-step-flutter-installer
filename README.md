# Flutter Install

[![Step changelog](https://shields.io/github/v/release/bitrise-steplib/bitrise-step-flutter-installer?include_prereleases&label=changelog&color=blueviolet)](https://github.com/bitrise-steplib/bitrise-step-flutter-installer/releases)

Install Flutter SDK.

<details>
<summary>Description</summary>

This Step git clones the selected branch or tag of the official Flutter repository and runs the initial setup of the Flutter SDK.
Use this step *before* the cache-pull step to make sure caching works correctly.

### Configuring the Step
1. In the **Flutter SDK git repository version** input set the tag or branch of the Flutter. The default value is `stable` which clones and installs the latest stable Flutter branch.
2. In the **Update to the latest version** input select `false` to use a preinstalled Flutter version or `true` to update Flutter SDK to the latest version released in the [build release channel](https://github.com/flutter/flutter/wiki/Flutter-build-release-channels). By default, this input is set to `true`.
4. Enable **Print debug information** to run `flutter doctor` to see if there are any missing platform dependencies for setting up Flutter.

### Troubleshooting
If you prefer to install Flutter from an installation bundle instead of the git repository, use the **Flutter SDK installation bundle URL** input. Insert the URL of the preferred [bundle](https://flutter.dev/docs/development/tools/sdk/releases), for example, `https://storage.googleapis.com/flutter_infra/releases/dev/windows/flutter_windows_v1.14.5-dev.zip`. If the input is filled out correctly, it overrides the value set in the **Flutter SDK git repository version** input.

### Useful links
- [About Flutter build release channels](https://github.com/flutter/flutter/wiki/Flutter-build-release-channels)
- [Available version tags](https://github.com/flutter/flutter/releases)
- [Available branches](https://github.com/flutter/flutter/branches)

### Related Steps
- [Flutter Test](https://www.bitrise.io/integrations/steps/flutter-test)
- [Flutter Build](https://www.bitrise.io/integrations/steps/flutter-build)
</details>

## 🧩 Get started

Add this step directly to your workflow in the [Bitrise Workflow Editor](https://devcenter.bitrise.io/steps-and-workflows/steps-and-workflows-index/).

You can also run this step directly with [Bitrise CLI](https://github.com/bitrise-io/bitrise).

## ⚙️ Configuration

<details>
<summary>Inputs</summary>

| Key | Description | Flags | Default |
| --- | --- | --- | --- |
| `version` | Use this input to install from the git repository by specifying a tag or branch.  Use this input for the stable channel, as the stable channel can be preinstalled.  If the input Flutter SDK installation bundle URL is specified, this input is ignored.  To find the available version tags see this list: [https://github.com/flutter/flutter/releases](https://github.com/flutter/flutter/releases)  To see the the avilable branches visit: [https://github.com/flutter/flutter/branches](https://github.com/flutter/flutter/branches) |  | `stable` |
| `is_debug` | If enabled will run flutter doctor and print value of PATH eniroment variable. |  | `false` |
</details>

<details>
<summary>Outputs</summary>
There are no outputs defined in this step
</details>

## 🙋 Contributing

We welcome [pull requests](https://github.com/bitrise-steplib/bitrise-step-flutter-installer/pulls) and [issues](https://github.com/bitrise-steplib/bitrise-step-flutter-installer/issues) against this repository.

For pull requests, work on your changes in a forked repository and use the Bitrise CLI to [run step tests locally](https://devcenter.bitrise.io/bitrise-cli/run-your-first-build/).

Learn more about developing steps:

- [Create your own step](https://devcenter.bitrise.io/contributors/create-your-own-step/)
- [Testing your Step](https://devcenter.bitrise.io/contributors/testing-and-versioning-your-steps/)
