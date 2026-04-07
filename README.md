# qvm — Qt Version Manager

A single-binary CLI tool for installing and managing Qt SDK versions on Windows.

Linux and macOS are not currently supported despite occasional references.

> [!WARNING]
> This tool is mostly AI generated for my own needs. The code has not been thoroughly reviewed. Use at your own risk! The interface will most likely change too.

```shell
qvm install 6.8.3
Resolving archives for Qt 6.8.3 (win64_msvc2022_64)...
Downloading [1/12] qtbase-windows-x86_64.7z  124 MB/s  34%
...
Qt 6.8.3 (win64_msvc2022_64) installed to C:\Qt\6.8.3\win64_msvc2022_64

qvm prefix 6.8.3
C:\Qt\6.8.3\win64_msvc2022_64

$Env:Qt6_DIR=$(qvm prefix 6.8.3)
```

## Features

- Install any Qt 6.x version with a single command (Qt 5 is not supported).
- Install Qt add-on modules, documentation, examples, sources, and debug symbols.
- Parallel downloads with retry, resumable interrupted transfers, and progress reporting.
- Auto-detect the best compiler/ABI for the current machine.
- Mirror management — auto-probe and switch to the fastest Qt mirror.
- Fuzzy search for module names.
- JSON output for scripting and automation.
- Metadata caching to minimize network requests.
- Concurrent-install safe (registry file locking).

## Installation

### From source

```shell
go install github.com/trollixx/qvm/cmd/qvm@latest
```

### Pre-built binaries

Download from the [Releases](https://github.com/trollixx/qvm/releases) page.

## Quick Start

```shell
# List available Qt versions
qvm list-remote

# Install Qt 6.8.3 (auto-detects compiler/ABI)
qvm install 6.8.3

# Install with specific modules
qvm install 6.8.3 --modules qtcharts,qtwebengine

# Show what is installed
qvm list

# Get the install path (useful with CMake)
qvm prefix 6.8.3

# Run health checks
qvm doctor
```

## Commands

### `install` (`i`)

Install a Qt version.

```shell
qvm install <version> [flags]
```

#### Flags

| Flag | Description |
| --- | --- |
| `--arch`, `-a` | Compiler/ABI target (e.g. `win64_msvc2022_64`, `gcc_64`, `macos`). Auto-detected if omitted. |
| `--target` | Qt platform: `desktop` (default), `android`, `wasm`, `ios`, `winrt` |
| `--modules`, `-m` | Comma-separated add-on modules (e.g. `qtcharts,qtwebengine`). The `qt` prefix is optional (`charts` and `qtcharts` both work). |
| `--docs` | Install documentation for all selected modules |
| `--examples` | Install examples for all selected modules |
| `--sources` | Install Qt source code |
| `--debug-symbols` | Install debug symbol files |
| `--host` | Host platform override (e.g. `windows_arm64`, `linux_arm64`). Auto-detected if omitted. |
| `--force` | Re-install even if already present |
| `--dir` | Override Qt installation directory |
| `--dry-run` | Resolve and print archives (with sizes) without downloading |
| `--quiet`, `-q` | Suppress progress output |

If a Qt version is already installed, qvm will only download and install the
delta — any new modules or extras you have added — unless `--force` is given.

#### Examples

```shell
# Base Qt only
qvm install 6.8.3

# With modules and docs
qvm install 6.8.3 --modules qtcharts,qtmultimedia --docs

# Specific ABI
qvm install 6.8.3 --arch win64_mingw

# Android target
qvm install 6.8.3 --target android

# WASM build with all extras
qvm install 6.8.3 --target wasm --modules qtcharts --docs --examples --sources
```

---

### `list` (`ls`)

List installed Qt versions. Fast and offline (no network access).

```shell
qvm list [flags]
```

| Flag | Description |
| --- | --- |
| `--format`, `-f` | Output format: `text` (default) or `json` |

---

### `list-remote` (`ls-remote`)

List Qt versions available in the repository, with installed versions marked.

```shell
qvm list-remote [<version-filter>] [flags]
```

| Flag | Description |
| --- | --- |
| `--format`, `-f` | Output format: `text` (default) or `json` |
| `--host` | Host platform override (e.g. `windows_arm64`, `linux_arm64`) |

```shell
qvm list-remote                # all available, grouped by major
qvm list-remote 6              # all 6.x versions
qvm list-remote 6.8            # all 6.8.x versions
qvm list-remote 6.8.3          # detailed view: archs, modules, extras
qvm list-remote --format json  # machine-readable output
```

---

### `uninstall` (`remove`)

Uninstall a Qt version.

```shell
qvm uninstall <version> [flags]
```

| Flag | Description |
| --- | --- |
| `--arch`, `-a` | Remove only a specific ABI (for multi-arch installations) |
| `--yes`, `-y` | Skip confirmation prompt |

---

### `prefix`

Print the installation directory for a Qt version.

```shell
qvm prefix <version> [--arch <arch>]
```

Ideal for use in build scripts:

```shell
cmake -DCMAKE_PREFIX_PATH=$(qvm prefix 6.8.3) ..
```

If multiple ABIs are installed for the same version, qvm picks the host's
default arch automatically; use `--arch` to disambiguate explicitly.

---

### `info` (`show`)

Show detailed information about an installed Qt version.

```shell
qvm info <version> [flags]
```

| Flag | Description |
| --- | --- |
| `--arch`, `-a` | Specify ABI for multi-arch installations |
| `--format`, `-f` | `text` (default) or `json` |

---

### `search` (`s`)

Fuzzy-search available add-on module names.

```shell
qvm search <query>
```

```shell
qvm search charts                  # finds qtcharts
qvm search webengine
qvm search --format json multimedia
```

---

### `doctor`

Run environment health checks.

```shell
qvm doctor
```

Checks include config readability, metadata cache freshness, disk space, Qt
installation integrity (qmake works), and compiler availability (MSVC on
Windows).

---

### `config`

Get, set, or list configuration values.

```shell
qvm config list
qvm config get <key>
qvm config set <key> <value>
qvm config path
```

#### Available keys

| Key | Default | Description |
| --- | --- | --- |
| `install.dir` | `C:\Qt` / `~/Qt` | Qt installation root |
| `repository.url` | `https://download.qt.io/` | Primary Qt mirror |
| `repository.mirrors` | - | Fallback mirror URLs (comma-separated) |
| `repository.blacklist` | - | Mirrors to exclude (comma-separated) |
| `download.concurrency` | `4` | Parallel download threads |
| `download.timeout_seconds` | `300` | HTTP timeout |

```shell
qvm config set install.dir /opt/Qt
qvm config set download.concurrency 8
qvm config set repository.mirrors https://ftp.jaist.ac.jp/pub/qtproject/,https://mirrors.dotsrc.org/qtproject/
```

---

### `cache`

Manage download and metadata caches.

```shell
qvm cache list
qvm cache clean [flags]
```

| Flag | Description |
| --- | --- |
| `--incomplete` | Remove only partial downloads, keep complete archives |
| `--metadata` | Remove cached repository metadata |
| `--all` | Remove everything |
| `--yes`, `-y` | Skip confirmation |

---

### `mirror`

Probe and manage Qt repository mirrors.

```shell
qvm mirror list
qvm mirror refresh
qvm mirror select --auto
qvm mirror select <url>
```

- **`list`** — Probe all cached mirrors and display latency. Marks the current primary (`*`) and fastest (`←`).
- **`refresh`** — Fetch the latest mirror list from Qt.
- **`select --auto`** — Probe all mirrors and switch to the fastest reachable one.
- **`select <url>`** — Test a specific URL and switch to it if reachable.

## Platform Support

| OS | Architectures | Default ABI |
| --- | --- | --- |
| Windows | x86_64 | `win64_msvc2022_64` |
| Linux | x86_64, aarch64 | `gcc_64` |
| macOS | x86_64, arm64 | `macos` |

### Common ABI values

| Platform | ABI | Compiler |
| --- | --- | --- |
| Windows | `win64_msvc2022_64` | MSVC 2022 64-bit |
| Windows | `win64_msvc2019_64` | MSVC 2019 64-bit |
| Windows | `win64_mingw` | MinGW-w64 64-bit |
| Windows | `win64_llvm_mingw` | LLVM/Clang MinGW 64-bit |
| Linux | `gcc_64` | GCC x86_64 |
| Linux | `linux_gcc_arm64` | GCC AArch64 |
| macOS | `macos` | Clang (universal / x86_64) |

The `install` command auto-detects the recommended ABI for the current machine. Use `--arch` to override.

## File Locations

qvm follows XDG Base Directory conventions on Linux and macOS, and standard Windows app directories on Windows.

| File | Windows | Linux / macOS |
| --- | --- | --- |
| Config | `%APPDATA%\qvm\config.toml` | `~/.config/qvm/config.toml` |
| Registry | `%LOCALAPPDATA%\qvm\registry.json` | `~/.local/state/qvm/registry.json` |
| Download cache | `%LOCALAPPDATA%\qvm\cache\downloads\` | `~/.cache/qvm/downloads/` |
| Metadata cache | `%LOCALAPPDATA%\qvm\cache\metadata\` | `~/.cache/qvm/metadata/` |

## Building from Source

```shell
git clone https://github.com/trollixx/qvm.git
cd qvm
go build ./cmd/qvm
```

Run tests:

```shell
go test ./...
```

## License

[MIT](LICENSE)
