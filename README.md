# qvm

[![CI](https://img.shields.io/github/actions/workflow/status/trollixx/qvm/ci.yaml?branch=main&label=ci&style=flat-square)](https://github.com/trollixx/qvm/actions/workflows/ci.yaml)
[![Coverage](https://img.shields.io/codecov/c/github/trollixx/qvm?style=flat-square)](https://app.codecov.io/gh/trollixx/qvm)
[![Release](https://img.shields.io/github/v/release/trollixx/qvm?style=flat-square)](https://github.com/trollixx/qvm/releases)
[![License](https://img.shields.io/github/license/trollixx/qvm?style=flat-square)](LICENSE)

Install Qt from the command line on Windows and macOS. One binary: no Qt account, no Python, no 7-Zip.

```sh
qvm install 6.8.3
qvm exec 6.8.3 -- cmake -B build
```

## Why

Qt's prebuilt SDKs come from its online repository, and the usual ways to get them each have a catch:

- **Qt Online Installer**: needs a Qt account.
- **aqtinstall**: needs Python.
- **install-qt-action**: GitHub Actions only, and runs aqtinstall.
- **vcpkg**: builds Qt from source, which can take hours.

qvm downloads the same archives as the online installer, straight from `download.qt.io` or a mirror, and needs nothing else installed.

Linux isn't supported; use your distribution's Qt packages.

## Install

Download the binary for your platform from the [latest release](https://github.com/trollixx/qvm/releases/latest): `qvm-windows-amd64.exe`, `qvm-windows-arm64.exe`, `qvm-macos-arm64`, or `qvm-macos-amd64`. Rename it to `qvm.exe` (`qvm` on macOS, then `chmod +x qvm`) and put it on your `PATH`. With the `gh` CLI:

```sh
gh release download --repo trollixx/qvm --pattern qvm-windows-amd64.exe --output qvm.exe
```

Or build it with Go:

```sh
go install github.com/trollixx/qvm/cmd/qvm@latest
```

## Usage

Find a version and install it:

```sh
qvm list-remote                    # versions in Qt's repository, LTS releases marked
qvm list-remote 6.8.3              # one version's ABIs, add-on modules, and extras
qvm install 6.8.3 --modules charts,webengine
```

Then build against it:

```sh
qvm exec 6.8.3 -- cmake -B build   # one command with this Qt on PATH and CMAKE_PREFIX_PATH
qvm prefix 6.8.3                   # or print its directory for your own scripts
```

`qvm use 6.8.3` makes it the default, so `exec`, `env`, `prefix`, and `info` can leave the version out: `qvm exec qmake -query`.

| Command | What it does |
| --- | --- |
| `qvm install <version>` | Install Qt, add-on modules, and extras |
| `qvm uninstall <version>` | Remove an install (`--arch` for one ABI) |
| `qvm list` | List installed versions, offline |
| `qvm list-remote [<version>]` | List versions in Qt's repository, or one version's contents |
| `qvm search <query>` | Fuzzy-search add-on module names |
| `qvm exec [<version> --] <cmd> [args...]` | Run one command with Qt on `PATH` and `CMAKE_PREFIX_PATH` |
| `qvm env [<version>]` | Print commands that set the same in your shell |
| `qvm prefix [<version>]` | Print an install's directory |
| `qvm use [<version>]` | Set or print the default version |
| `qvm info [<version>]` | Show an install's modules, extras, and size |
| `qvm doctor` | Check the config, caches, disk space, and installs |
| `qvm config` | Get or set configuration values |
| `qvm mirror` | Probe Qt's mirrors and switch to the fastest |
| `qvm cache` | List or clean downloaded archives and metadata |

`qvm <command> --help` lists every flag. `list`, `list-remote`, `info`, and `search` print JSON with `--format json`.

## Installing Qt

`qvm install <version>` installs Qt's essential modules for this machine. Running it again with more modules or extras downloads only what's missing.

| Flag | |
| --- | --- |
| `--modules`, `-m` | Add-on modules, comma-separated; the `qt` prefix is optional |
| `--no-deps` | Don't add the modules those depend on |
| `--docs`, `--examples`, `--sources`, `--debug-symbols` | Extras, for every selected module |
| `--arch`, `-a` | ABI, e.g. `win64_mingw`; see the defaults below |
| `--target` | `desktop` (default), `android`, `ios`, or `wasm`; all but `desktop` need `--arch` |
| `--host` | Another host's build, e.g. `windows_x86` |
| `--dir` | Install root, instead of `install.dir` |
| `--dry-run` | List the archives without downloading them |
| `--force` | Reinstall |
| `--quiet`, `-q` | No progress output |

Modules come with the modules they depend on: `qthttpserver` brings `qtwebsockets`, `qtquick3d` brings `qtshadertools` and `qtquicktimeline`, and `qtwebengine` brings `qtpositioning` and `qtwebchannel`.

Without `--arch`, you get the ABI for this machine:

| Machine | Default ABI |
| --- | --- |
| Windows x64 | `win64_msvc2022_64`; before Qt 6.8, `win64_msvc2019_64`, which Visual Studio 2022 can use too |
| Windows on ARM | `win64_msvc2022_arm64`, even from `qvm-windows-amd64.exe` |
| macOS | `clang_64`, a universal x86_64 and arm64 build |

Qt has Windows ARM64 builds from 6.8 on. For older versions, `--host windows_x86` installs the x64 build, which runs under emulation. `qvm list-remote <version>` lists the other ABIs, such as `win64_mingw` and `win64_llvm_mingw`.

Desktop is the most tested target. Android, iOS, and WebAssembly builds go through the same pipeline, so any archive Qt publishes for them should install.

## Shell setup

`qvm env` prints commands that prepend the install to `CMAKE_PREFIX_PATH` and its `bin` directory to `PATH`, keeping what's already there. Run it with the line for your shell, or put that line in your shell's profile.

PowerShell:

```powershell
qvm env 6.8.3 | Out-String | Invoke-Expression
```

cmd (in a batch file, use `%%i` instead of `%i`):

```bat
for /f "delims=" %i in ('qvm env 6.8.3 --shell cmd') do @%i
```

Nushell:

```nu
load-env (qvm env 6.8.3 --shell nu | from json)
```

bash or zsh, including Git Bash and MSYS2 on Windows:

```sh
eval "$(qvm env 6.8.3 --shell bash)"
```

fish:

```fish
qvm env 6.8.3 --shell fish | source
```

On Windows, the bash and fish output converts `PATH` to POSIX paths with `cygpath`, which ships with Git for Windows and MSYS2.

## Configuration

`qvm config list` shows every value, and `qvm config set <key> <value>` changes one.

| Key | Default | |
| --- | --- | --- |
| `install.dir` | `C:\Qt`, or `~/Qt` on macOS | Where versions are installed |
| `qt.default` | | Default version, set by `qvm use` |
| `repository.url` | `https://download.qt.io/` | Mirror to download from |
| `repository.mirrors` | | Fallback mirrors, comma-separated |
| `repository.blacklist` | | Mirrors for `qvm mirror` to skip, comma-separated |
| `download.concurrency` | `4` | Parallel downloads |
| `download.timeout_seconds` | `300` | HTTP timeout |

`qvm mirror refresh` fetches Qt's mirror list, `qvm mirror list` probes each one, and `qvm mirror select --auto` sets `repository.url` to the fastest.

qvm keeps its files here:

| File | Windows | macOS |
| --- | --- | --- |
| Config | `%LOCALAPPDATA%\qvm\config.toml` | `~/Library/Application Support/qvm/config.toml` |
| Registry | `%LOCALAPPDATA%\qvm\registry.json` | `~/Library/Application Support/qvm/registry.json` |
| Download cache | `%LOCALAPPDATA%\qvm\downloads\` | `~/Library/Caches/qvm/downloads/` |
| Metadata cache | `%LOCALAPPDATA%\qvm\metadata\` | `~/Library/Caches/qvm/metadata/` |

## How it works

qvm reads the package metadata (`Updates.xml`) of Qt's online repository, the same one the online installer uses, and caches it. From it, qvm works out the archives for the version, ABI, and modules you asked for, including the modules they depend on.

It downloads the archives in parallel, retrying failures and resuming partial downloads, and checks each against the SHA-1 file Qt publishes next to it. It extracts them with a built-in 7-Zip decoder, then writes `bin/qt.conf` so the install works from its directory, and records it in the registry. `list`, `info`, `prefix`, `exec`, and `env` only read the registry, so they work offline.

Each install locks its target directory, so parallel `qvm install` runs of different versions or ABIs don't wait for each other.

## Requirements

- Windows (x64 or ARM64) or macOS
- Qt 6; Qt 5 isn't supported
- A compiler for the ABI you install, such as Visual Studio or the Xcode Command Line Tools; on Windows, `qvm doctor` checks for MSVC

## Building from source

```sh
git clone https://github.com/trollixx/qvm.git
cd qvm
go build ./cmd/qvm
go test ./...
```

`qvm --version` reports the module version that the Go toolchain stamps into `go install` and VCS builds; release builds set it with `-ldflags`.

## License

[MIT](LICENSE)
