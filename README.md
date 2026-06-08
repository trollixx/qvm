# qvm: Qt Version Manager

A single-binary CLI tool for installing and managing Qt SDK versions on Windows and macOS.

Linux is not supported; use your distribution's Qt packages instead.

> [!WARNING]
> This tool is mostly AI generated for my own needs. The code has not been thoroughly reviewed. Use at your own risk! The interface will most likely change too.

```console
$ qvm install 6.8.3
Resolving archives for Qt 6.8.3 (win64_msvc2022_64)...
Downloading [1/12] qtbase-windows-x86_64.7z  124 MB/s  34%
...
Qt 6.8.3 (win64_msvc2022_64) installed to C:\Qt\6.8.3\win64_msvc2022_64

$ qvm prefix 6.8.3
C:\Qt\6.8.3\win64_msvc2022_64
```

Point your build at it (PowerShell):

```powershell
$env:Qt6_DIR = qvm prefix 6.8.3
```

## Features

- Install any Qt 6.x version with a single command (Qt 5 is not supported).
- Install Qt add-on modules, documentation, examples, sources, and debug symbols.
- Automatic module dependency resolution (e.g. `qthttpserver` pulls in `qtwebsockets`).
- Parallel downloads with retry, resumable interrupted transfers, and progress reporting.
- Activate a Qt version in any shell with `qvm env` (PowerShell, cmd, bash, zsh, fish, nu).
- Run one-off commands in a Qt environment with `qvm exec` (e.g. `qvm exec 6.8.3 -- qmake`).
- Set a default version with `qvm use`, so `env`, `exec`, `prefix`, and `info` need no version argument.
- Auto-detect the best compiler/ABI for the current machine.
- Mirror management: auto-probe and switch to the fastest Qt mirror.
- Fuzzy search for module names.
- JSON output for scripting and automation.
- Metadata caching to minimize network requests.
- Concurrent-install safe: parallel `qvm install` runs serialize per install directory, so installs of different versions or ABIs proceed independently.

## Installation

### From source

```shell
go install github.com/trollixx/qvm/cmd/qvm@latest
```

### Pre-built binaries

Download from the [Releases](https://github.com/trollixx/qvm/releases) page.

## Quick start

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
| `--arch`, `-a` | Compiler/ABI target (e.g. `win64_msvc2022_64`, `clang_64`). Auto-detected if omitted. |
| `--target` | Qt platform: `desktop` (default), `android`, `ios`, `wasm`. WASM variants live under `desktop/` and must be selected explicitly with `--arch` (e.g. `wasm_singlethread`); see the note below. WinRT is not supported (removed in Qt 6). |
| `--modules`, `-m` | Comma-separated add-on modules (e.g. `qtcharts,qtwebengine`). The `qt` prefix is optional (`charts` and `qtcharts` both work). |
| `--no-deps` | Do not auto-install modules the requested modules depend on |
| `--docs` | Install documentation for all selected modules |
| `--examples` | Install examples for all selected modules |
| `--sources` | Install Qt source code |
| `--debug-symbols` | Install debug symbol files |
| `--host` | Host platform override (e.g. `windows_arm64`, `mac_x64`). Auto-detected if omitted. |
| `--force` | Re-install even if already present |
| `--dir` | Override Qt installation directory |
| `--dry-run` | Resolve and print archives (with sizes) without downloading |
| `--quiet`, `-q` | Suppress progress output |

If a Qt version is already installed, qvm downloads and installs only the
delta (any new modules or extras you have added), unless `--force` is given.

Modules that the requested modules depend on (as declared in Qt's repository
metadata) are installed automatically. For example, `qthttpserver` requires
`qtwebsockets`, and `qtquick3d` requires `qtshadertools` and `qtquicktimeline`.
Already-installed dependencies are never re-downloaded. Pass `--no-deps` to
install only the modules you named.

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
```

> Desktop is the most thoroughly exercised target. Android and iOS resolve under
> their own repository paths, but the rest of the pipeline is shared, so any
> archive Qt publishes for the chosen target will install. WASM packages live
> under `desktop/` as separate arch variants; qvm does not list them
> automatically, so pass the variant yourself with `--arch`.

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
| `--host` | Host platform override (e.g. `windows_arm64`, `mac_x64`) |

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
qvm prefix [<version>] [flags]
```

| Flag | Description |
| --- | --- |
| `--arch`, `-a` | Select a specific ABI (for multi-arch installations) |

Ideal for use in build scripts:

```shell
cmake -DCMAKE_PREFIX_PATH=$(qvm prefix 6.8.3) ..
```

If multiple ABIs are installed for the same version, qvm picks the host's
default arch automatically; use `--arch` to disambiguate explicitly.

---

### `env`

Print shell commands that activate a Qt version in the current shell.

```shell
qvm env [<version>] [flags]
```

| Flag | Description |
| --- | --- |
| `--arch`, `-a` | Select a specific ABI (for multi-arch installations) |
| `--shell` | Output format: `powershell` (`pwsh`), `cmd` (`bat`), `bash` (`sh`, `zsh`), `fish`, `nu` (`nushell`). Defaults to `powershell` on Windows, `bash` elsewhere. |

Two variables are set: the install prefix is prepended to `CMAKE_PREFIX_PATH`
and `<prefix>/bin` to `PATH` (so `qmake`, `qtdiag`, `windeployqt`/`macdeployqt`,
`qmlls`, etc. resolve). Existing values are preserved, never clobbered, so Qt
composes with vcpkg or other prefixes already in your environment.

Evaluate the output in your shell:

```shell
# PowerShell
qvm env 6.10.2 | Out-String | Invoke-Expression

# bash / zsh
eval "$(qvm env 6.10.2 --shell bash)"

# fish
qvm env 6.10.2 --shell fish | source

# nushell
load-env (qvm env 6.10.2 --shell nu | from json)

# cmd
for /f "delims=" %i in ('qvm env 6.10.2 --shell cmd') do @%i
```

---

### `exec`

Run a single command with a Qt version's environment, without modifying the
current shell.

```shell
qvm exec <command> [args...]
qvm exec [<version>] -- <command> [args...]
```

| Flag | Description |
| --- | --- |
| `--arch`, `-a` | Select a specific ABI (for multi-arch installations) |

The same variables as `qvm env` are applied, and the command itself is
resolved against the modified `PATH`, so `qvm exec 6.8.3 -- qmake` runs that
Qt's `qmake` even if another Qt is first on your shell's `PATH`. Stdin/stdout
are passed through untouched and the child's exit code becomes qvm's exit
code, so it composes with pipes, redirection, and CI scripts.

The Qt version is positional and must come before `--`. Without `--`, every
argument is the command and the default from `qvm use` applies. The version is
never guessed from an argument's shape, so `qvm exec 6.8.3 qmake` looks for a
command named `6.8.3`; write `qvm exec 6.8.3 -- qmake` to select a version. A
leading `-a`/`--arch` flag is accepted with or without `--`.

```shell
qvm exec 6.8.3 -- qmake -query QT_VERSION
qvm exec 6.8.3 -- cmake -S . -B build
qvm exec 6.8.3 -- windeployqt build\app.exe
```

---

### `use`

Set the default Qt version. `env`, `exec`, `prefix`, and `info` use it when no
version argument is given.

```shell
qvm use [<version>] [flags]
```

| Flag | Description |
| --- | --- |
| `--unset` | Clear the default version |

```shell
qvm use 6.8.3        # set the default (must be installed)
qvm use              # print the current default
qvm use --unset      # clear it
qvm exec -- qmake    # now runs the default Qt's qmake
```

The default is stored in the config file (`qt.default`) and marked with `*` in
`qvm list`. Note that setting it does not modify any shell environment; pair
it with `qvm env` or `qvm exec` to actually build against the default version.

---

### `info` (`show`)

Show detailed information about an installed Qt version.

```shell
qvm info [<version>] [flags]
```

| Flag | Description |
| --- | --- |
| `--arch`, `-a` | Select a specific ABI (for multi-arch installations) |
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
| `qt.default` | - | Default Qt version for `env`/`exec`/`prefix`/`info` (set via `qvm use`) |
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

| Subcommand | Description |
| --- | --- |
| `list` | Probe all cached mirrors and display latency. Marks the current primary (`*`) and fastest (`←`). |
| `refresh` | Fetch the latest mirror list from Qt |
| `select --auto` | Probe all mirrors and switch to the fastest reachable one |
| `select <url>` | Test a specific URL and switch to it if reachable |

## Platform support

| OS | Architectures | Default ABI |
| --- | --- | --- |
| Windows | x86_64, arm64 | `win64_msvc2022_64` |
| macOS | x86_64, arm64 (universal) | `clang_64` |

### Common ABI values

| Platform | ABI | Compiler |
| --- | --- | --- |
| Windows | `win64_msvc2022_64` | MSVC 2022 64-bit |
| Windows | `win64_msvc2019_64` | MSVC 2019 64-bit |
| Windows | `win64_mingw` | MinGW-w64 64-bit |
| Windows | `win64_llvm_mingw` | LLVM/Clang MinGW 64-bit |
| macOS | `clang_64` | Clang (universal: x86_64 + arm64) |

The `install` command auto-detects the recommended ABI for the current machine. Use `--arch` to override.

## File locations

qvm uses standard Windows app directories on Windows, and the macOS Application
Support / Caches conventions on macOS.

| File | Windows | macOS |
| --- | --- | --- |
| Config | `%LOCALAPPDATA%\qvm\config.toml` | `~/Library/Application Support/qvm/config.toml` |
| Registry | `%LOCALAPPDATA%\qvm\registry.json` | `~/Library/Application Support/qvm/registry.json` |
| Download cache | `%LOCALAPPDATA%\qvm\downloads\` | `~/Library/Caches/qvm/downloads/` |
| Metadata cache | `%LOCALAPPDATA%\qvm\metadata\` | `~/Library/Caches/qvm/metadata/` |

## Building from source

```shell
git clone https://github.com/trollixx/qvm.git
cd qvm
go build ./cmd/qvm
```

`qvm --version` reports the module version stamped by the Go toolchain (for
`go install` and VCS builds); release builds may override it via `-ldflags`.

Run tests:

```shell
go test ./...
```

## License

[MIT](LICENSE)
