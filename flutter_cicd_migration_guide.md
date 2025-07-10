# Flutter CI/CD Migration Guide: From Bitbucket/CircleCI to GitHub

## Executive Summary

When moving your Flutter app from Bitbucket/CircleCI to GitHub, you have several excellent options for CI/CD. **GitHub Actions** is the direct replacement for CircleCI and offers powerful automation capabilities. Additionally, **makefiles** can serve as a lightweight alternative for local builds and simpler deployment scenarios.

## 1. GitHub Actions - The Direct CircleCI Replacement

### Overview
GitHub Actions is GitHub's native CI/CD platform that provides:
- **Free tier**: 2,000 minutes/month for private repos, unlimited for public repos
- **Native integration** with GitHub repositories
- **Cross-platform support**: Linux, Windows, and macOS runners
- **Marketplace ecosystem** with thousands of pre-built actions

### Key Advantages
- **Cost-effective**: More generous free tier than most alternatives
- **Seamless integration**: Built into GitHub, no third-party setup
- **Flutter-specific actions**: Dedicated Flutter setup actions available
- **Scalable**: Supports parallel builds and complex workflows
- **Security**: Built-in secrets management

### Basic Flutter GitHub Actions Workflow

```yaml
name: Flutter CI/CD

on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  build-and-test:
    runs-on: ubuntu-latest

    steps:
    - name: Checkout Repository
      uses: actions/checkout@v4

    - name: Setup Flutter
      uses: subosito/flutter-action@v2
      with:
        flutter-version: 'stable'
        channel: 'stable'

    - name: Install Dependencies
      run: flutter pub get

    - name: Run Code Analysis
      run: flutter analyze

    - name: Run Tests
      run: flutter test

    - name: Build Android APK
      run: flutter build apk --release

    - name: Build iOS (macOS runner required)
      run: flutter build ios --no-codesign
      if: runner.os == 'macOS'

    - name: Upload Artifacts
      uses: actions/upload-artifact@v3
      with:
        name: flutter-builds
        path: |
          build/app/outputs/flutter-apk/app-release.apk
          build/ios/ipa/*.ipa
```

### Advanced Features

#### 1. **Signed Android Builds**
```yaml
- name: Decode Keystore
  run: echo "${{ secrets.ANDROID_KEYSTORE }}" | base64 --decode > android/app/keystore.jks

- name: Build Signed APK
  run: |
    flutter build apk --release \
      --dart-define=KEYSTORE_PATH=android/app/keystore.jks \
      --dart-define=KEYSTORE_ALIAS=${{ secrets.KEYSTORE_ALIAS }} \
      --dart-define=KEYSTORE_PASSWORD=${{ secrets.KEYSTORE_PASSWORD }}
```

#### 2. **Automatic Store Deployment**
```yaml
- name: Deploy to Google Play
  uses: r0adkll/upload-google-play@v1
  with:
    serviceAccountJson: ${{ secrets.GOOGLE_PLAY_SERVICE_ACCOUNT }}
    packageName: com.example.yourapp
    releaseFiles: build/app/outputs/flutter-apk/app-release.apk
```

#### 3. **Multi-Platform Matrix Builds**
```yaml
strategy:
  matrix:
    os: [ubuntu-latest, macos-latest]
    flutter-version: ['3.16.0', 'stable']
```

### Security Best Practices
- Store sensitive data (keystores, API keys) in **GitHub Secrets**
- Use **base64 encoding** for binary files like keystores
- Implement **environment-specific** configurations
- Use **OIDC tokens** for cloud provider authentication

## 2. Makefile Alternative - Lightweight Local Builds

### Overview
Makefiles provide a simpler, script-based approach to Flutter builds that can:
- **Automate local builds** without cloud dependency
- **Standardize build commands** across team members
- **Integrate with any CI/CD** system including GitHub Actions
- **Reduce costs** by minimizing cloud build minutes

### Sample Flutter Makefile

```makefile
# Flutter Build Automation Makefile

.PHONY: clean install analyze test build-android build-ios deploy help

# Default Flutter channel
FLUTTER_CHANNEL ?= stable
BUILD_NUMBER ?= 1
VERSION_NAME ?= 1.0.0

help: ## Show this help message
	@echo "Flutter Build Automation"
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

clean: ## Clean build artifacts
	flutter clean
	flutter pub get

install: ## Install dependencies
	flutter pub get
	flutter pub upgrade

analyze: ## Run static analysis
	flutter analyze
	dart format --set-exit-if-changed .

test: ## Run tests with coverage
	flutter test --coverage
	lcov --list coverage/lcov.info

build-android: clean install ## Build Android APK
	flutter build apk --release \
		--build-number=$(BUILD_NUMBER) \
		--build-name=$(VERSION_NAME)

build-android-bundle: clean install ## Build Android App Bundle
	flutter build appbundle --release \
		--build-number=$(BUILD_NUMBER) \
		--build-name=$(VERSION_NAME)

build-ios: clean install ## Build iOS (requires macOS)
	flutter build ios --release --no-codesign \
		--build-number=$(BUILD_NUMBER) \
		--build-name=$(VERSION_NAME)

build-web: clean install ## Build for web
	flutter build web --release

deploy-firebase: build-web ## Deploy to Firebase Hosting
	firebase deploy --only hosting

deploy-github-pages: build-web ## Deploy to GitHub Pages
	cd build/web && \
	git init && \
	git add . && \
	git commit -m "Deploy $(VERSION_NAME)" && \
	git push -f origin main

pretty: ## Format code and run build_runner
	dart format .
	flutter packages pub run build_runner build --delete-conflicting-outputs

ci: clean analyze test build-android ## Run complete CI pipeline locally
```

### Usage Examples
```bash
# Basic operations
make clean install
make analyze test
make build-android

# Custom version builds
make build-android BUILD_NUMBER=42 VERSION_NAME=2.1.0

# Complete CI pipeline
make ci

# Deploy to various platforms
make deploy-firebase
make deploy-github-pages
```

### Integration with GitHub Actions
```yaml
- name: Run Makefile Build
  run: make ci

- name: Custom Build
  run: make build-android BUILD_NUMBER=${{ github.run_number }}
```

## 3. Hybrid Approach - Best of Both Worlds

### Local Development with Makefiles
- **Fast iteration** during development
- **Consistent commands** across team members
- **Offline capability** for basic builds
- **Cost reduction** for frequent builds

### GitHub Actions for CI/CD
- **Automated testing** on pull requests
- **Multi-platform builds** (iOS requires macOS runners)
- **Secure deployment** to app stores
- **Integration testing** and quality gates

### Sample Hybrid Workflow
```yaml
name: Hybrid Flutter CI/CD

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  quick-checks:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    - uses: subosito/flutter-action@v2
    - name: Run Quick Checks
      run: make analyze test

  full-build:
    needs: quick-checks
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    steps:
    - uses: actions/checkout@v4
    - uses: subosito/flutter-action@v2
    - name: Build for Platform
      run: |
        if [ "$RUNNER_OS" == "Linux" ]; then
          make build-android
        elif [ "$RUNNER_OS" == "macOS" ]; then
          make build-ios
        fi
```

## 4. Cost Comparison

### GitHub Actions Pricing
- **Free tier**: 2,000 minutes/month (private repos)
- **Linux runners**: $0.008/minute
- **macOS runners**: $0.08/minute (10x more expensive)
- **Windows runners**: $0.016/minute

### Alternative Platforms
- **Bitrise**: Mobile-focused, $50+/month for teams
- **Codemagic**: Flutter-specialized, $28+/month per user
- **CircleCI**: $30+/month for private repos
- **Jenkins**: Self-hosted, infrastructure costs only

### Cost Optimization Strategies
1. **Use makefiles for local builds** to reduce cloud minutes
2. **Conditional iOS builds** only when iOS code changes
3. **Parallel jobs** to reduce total build time
4. **Caching dependencies** to speed up builds
5. **Public repositories** get unlimited GitHub Actions minutes

## 5. Migration Recommendations

### Immediate Migration Path
1. **Start with GitHub Actions** using the basic workflow above
2. **Migrate secrets** from CircleCI to GitHub Secrets
3. **Update webhook configurations** in your deployment targets
4. **Test thoroughly** with pull requests before going live

### Long-term Optimization
1. **Implement makefiles** for development standardization
2. **Add platform-specific conditionals** to reduce costs
3. **Set up automated deployments** to app stores
4. **Monitor usage** and optimize for cost efficiency

### Team Adoption Strategy
1. **Train developers** on GitHub Actions syntax
2. **Standardize makefile commands** across projects
3. **Document workflows** and troubleshooting guides
4. **Establish code review process** for workflow changes

## 6. Conclusion

**GitHub Actions is the clear successor to CircleCI** for Flutter projects moving to GitHub, offering:
- Native integration and generous free tier
- Robust Flutter ecosystem support
- Comprehensive security and deployment features

**Makefiles complement GitHub Actions** by providing:
- Fast local development cycles
- Cost-effective build automation
- Team standardization across projects
- Offline build capabilities

**Recommended approach**: Use GitHub Actions for CI/CD automation and makefiles for local development standardization. This hybrid approach maximizes both developer productivity and cost efficiency while maintaining robust deployment pipelines.

The migration from CircleCI to GitHub Actions is straightforward, and the addition of makefiles provides excellent local development experience and cost optimization opportunities.