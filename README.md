# roffs

**A FUSE-based read-only virtual filesystem that exposes a filtered view of a real directory**

This project provides a read-only FUSE filesystem that filters and displays files from a source directory based on configurable rules.

## Features

- Expression-based filtering using [expr-lang](https://github.com/expr-lang/expr)
- Optional file sorting and limiting
- Configurable via config file or one/more `-rules` command-line flags
- Supports mounting a filtered view of an existing directory

## Installation

Prerequisites: Go 1.18+ and a working FUSE setup (e.g., libfuse on Linux, macFUSE on macOS).

To install the `roffs` binary with Go modules:
```bash
go install github.com/gustavosbarreto/roffs@latest
```

This will place `roffs` in your `$GOPATH/bin` (or `$HOME/go/bin`). Ensure it's in your `PATH`.

Alternatively, build from source:
```bash
git clone https://github.com/gustavosbarreto/roffs.git
cd roffs
go build -o roffs
```

## Usage

```bash
roffs [flags] SOURCE TARGET
```

### Flags

- `-c, --config string`   Path to a config file (required if no `--rules` provided).
- `-r, --rules string`    Inline rule expression; can be specified multiple times (overrides `--config`).

### Rule Syntax

The rules define a pipeline using `|` syntax. Each rule can contain:

- `filter <expr>`: Expression to filter files. Variables available: `name`, `ext`, `size`, `isDir`
- `sort <field> <asc|desc>`: Sorts by `name` or `size`
- `limit <N>`: Restricts to the first N entries after filtering and sorting

Multiple rule pipelines can be specified as separate lines in the config file. When using inline `--rules`, you can provide multiple pipelines by repeating the flag.

### Examples

#### Config file format
Each line in the config file defines a pipeline. Lines starting with `#` or blank lines are ignored.
Example `config.txt`:
```txt
filter name =~ "file-\d+" | sort size desc | limit 5
filter ext == ".log" && size > 1048576
```

#### Using inline rules
```bash
roffs -r 'filter name =~ "file-\d+" | sort size desc | limit 3' ./data /mnt/filtered
```

#### Multiple inline rules
```bash
roffs \
  -r 'filter ext == ".go"' \
  -r 'filter size > 1024' \
  ./data /mnt/filtered
```
