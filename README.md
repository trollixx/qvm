# qvm — Qt Version Manager

A single-binary CLI tool for installing and managing Qt SDK versions and tools on Windows.

Linux and macOS are not currently supported despite occasional references.

> [!WARNING]
> This tool is mostly AI generated for my own needs. The code has not been thoroughly reviewed. Use at your own risk! The interface will most likely change too.

```shell
qvm install qt@6.8.3
Resolving archives for Qt 6.8.3 (win64_msvc2022_64)...
Downloading [1/12] qtbase-windows-x86_64.7z  124 MB/s  34%
...
Qt 6.8.3 (win64_msvc2022_64) installed to C:\Qt\6.8.3\win64_msvc2022_64

qvm path qt@6.8.3
C:\Qt\6.8.3\win64_msvc2022_64

$Env:Qt6_DIR=$(qvm path qt@6.8.3)
```

## Features

- Install any Qt version (5.x / 6.x) with a single command.
- Install Qt add-on modules, documentation, examples, sources, and debug info.
- Install Qt tools (Qt Creator).
- Parallel downloads with retry and progress reporting.
- Auto-detect the best compiler/ABI for the current machine.
- Mirror management - auto-probe and switch to the fastest Qt mirror.
- Fuzzy search for module and tool names.
- JSON output for scripting and automation.
- Metadata caching to minimize network requests.

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
qvm list --all qt

# Install Qt 6.8.3 (auto-detects compiler/ABI)
qvm install qt@6.8.3

# Install with specific modules
qvm install qt@6.8.3 --modules qtcharts,qtwebengine

# Install Qt Creator
qvm install qtcreator@15.0.0

# Show what is installed
qvm list

# Get the install path (useful with CMake)
qvm path qt@6.8.3

# Run health checks
qvm doctor
```

## Commands

### `install` (`i`)

Install a Qt version or tool.

```shell
qvm install qt@<version> [flags]
qvm install <tool>[@<version>] [flags]
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
| `--debug-info` | Install debug information files |
| `--host` | Host platform override (e.g. `windows_arm64`, `linux_arm64`). Auto-detected if omitted. |
| `--force` | Re-install even if already present |
| `--dir` | Override Qt installation directory |
| `--dry-run` | Resolve and print archives without downloading |
| `--quiet`, `-q` | Suppress progress output |

#### Examples

```shell
# Base Qt only
qvm install qt@6.8.3

# With modules and docs
qvm install qt@6.8.3 --modules qtcharts,qtmultimedia --docs

# Specific ABI
qvm install qt@6.8.3 --arch win64_mingw

# Android target
qvm install qt@6.8.3 --target android

# WASM build with all extras
qvm install qt@6.8.3 --target wasm --modules qtcharts --docs --examples --sources

# Tool install
qvm install qtcreator@15.0.0
```

---

### `list` (`ls`)

List installed Qt versions and tools, or browse what is available remotely.

```shell
qvm list [qt|tools] [qt@<version>] [flags]
```

| Flag | Description |
| --- | --- |
| `--all` | Show available (remote) versions instead of installed |
| `--format`, `-f` | Output format: `text` (default) or `json` |

```shell
qvm list                     # all installed
qvm list qt                  # installed Qt only
qvm list tools               # installed tools only
qvm list qt@6.8.3            # details for a specific version
qvm list --all               # all available in the repository
qvm list --all qt            # all available Qt versions
qvm list --format json       # JSON output for scripting
```

---

### `uninstall` (`remove`)

Uninstall a Qt version or tool.

```shell
qvm uninstall qt@<version> [flags]
qvm uninstall <tool>@<version> [flags]
```

| Flag | Description |
| --- | --- |
| `--arch`, `-a` | Remove only a specific ABI (for multi-arch Qt installations) |
| `--yes`, `-y` | Skip confirmation prompt |

---

### `path`

Print the installation directory for a Qt version or tool.

```shell
qvm path qt@<version> [--arch <arch>]
qvm path <tool>[@<version>]
```

Ideal for use in build scripts:

```shell
cmake -DCMAKE_PREFIX_PATH=$(qvm path qt@6.8.3) ..
```

---

### `info` (`show`)

Show detailed information about an installed version or tool.

```shell
qvm info qt@<version> [flags]
qvm info <tool>[@<version>] [flags]
```

| Flag | Description |
| --- | --- |
| `--arch`, `-a` | Specify ABI for multi-arch installations |
| `--format`, `-f` | `text` (default) or `json` |

---

### `search` (`s`)

Fuzzy-search available module and tool names.

```shell
qvm search <query>
```

```shell
qvm search charts      # finds qtcharts
qvm search crtea       # finds qtcreator
qvm search --format json webengine
```

---

### `doctor`

Run environment health checks.

```shell
qvm doctor
```

Checks include config readability, metadata cache freshness, disk space, Qt and tool installation integrity, and compiler availability (MSVC on Windows, clang on macOS, GCC on Linux).

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
| `install.tools_dir` | `{install.dir}/Tools` | Tools installation directory |
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

- **`list`** - Probe all cached mirrors and display latency. Marks the current primary (`*`) and fastest (`←`).
- **`refresh`** - Fetch the latest mirror list from Qt.
- **`select --auto`** - Probe all mirrors and switch to the fastest reachable one.
- **`select <url>`** - Test a specific URL and switch to it if reachable.

## Platform Support

| OS | Architectures | Default ABI |
| --- | --- | --- |
| Windows | x86_64, x86 | `win64_msvc2022_64` |
| Linux | x86_64, aarch64 | `gcc_64` |
| macOS | x86_64, arm64 | `macos` |

### Common ABI values

| Platform | ABI | Compiler |
| --- | --- | --- |
| Windows | `win64_msvc2022_64` | MSVC 2022 64-bit |
| Windows | `win64_msvc2019_64` | MSVC 2019 64-bit |
| Windows | `win64_mingw` | MinGW-w64 64-bit |
| Windows | `win32_msvc2022` | MSVC 2022 32-bit |
| Linux | `gcc_64` | GCC x86_64 |
| Linux | `gcc_arm64` | GCC AArch64 |
| macOS | `macos` | Clang (universal / x86_64) |
| macOS | `macos_arm64` | Clang ARM64 |

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
