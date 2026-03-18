# Cross-Platform Smoke Test Plan

This document outlines the smoke tests that should be run on each supported platform (Linux, macOS, Windows) to validate the muster CLI builds and runs correctly.

## Test Environment Requirements

- **Linux**: Ubuntu 22.04+ or similar distribution, Go 1.23+
- **macOS**: macOS 12+ (Monterey or later), Go 1.23+
- **Windows**: Windows 10/11, Go 1.23+, PowerShell 5.1+ or PowerShell Core

## Smoke Test Checklist

Run these tests on each platform:

### 1. Build Succeeds

```bash
make build
```

**Expected result**:
- Command exits with code 0
- Binary is created in `dist/` directory
- Binary name follows platform conventions:
  - Linux/macOS: `dist/muster` (no extension)
  - Windows: `dist/muster.exe`

### 2. Tests Pass

```bash
make test
```

**Expected result**:
- Command exits with code 0
- All test packages pass
- No race conditions detected

### 3. Binary Executes

**Linux/macOS:**
```bash
./dist/muster version
```

**Windows:**
```powershell
.\dist\muster.exe version
```

**Expected result**:
- Command exits with code 0
- Prints version information to stdout

### 4. Version Output Contains Metadata

Verify the output from step 3 includes:

- **Version**: Git tag or commit hash (e.g., `v0.1.0` or commit SHA)
- **Commit**: Short commit hash (7 characters)
- **Date**: Build timestamp in ISO 8601 format (e.g., `2026-03-17T10:30:00Z`)

Example output:
```
muster version v0.1.0
commit: abc1234
built: 2026-03-17T10:30:00Z
```

### 5. Binary Naming Conventions

Verify binary naming follows platform conventions:

- **Linux/macOS**: Executable has no file extension, executable bit set
- **Windows**: Executable has `.exe` extension

## Notes

- These tests cannot be automated across all platforms from a single Linux environment
- Run manually on each target platform before releasing
- CI/CD pipelines should automate these tests where possible (e.g., GitHub Actions matrix builds)
- The verify.sh script (`/.dev-agent/scripts/verify.sh`) validates the build but does not test the binary execution or version output
