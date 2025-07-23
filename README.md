# roffs

**A FUSE-based read-only virtual filesystem that exposes a filtered view of a real directory**

This project provides a read-only FUSE filesystem that filters and displays files from a source directory based on configurable rules.

## Features

- Expression-based filtering using [expr-lang](https://github.com/expr-lang/expr)
- Optional file sorting and limiting
- Configurable via config file or `-rules` command-line flag
- Supports mounting a filtered view of an existing directory

## Usage

```bash
rofs-filtered -source /path/to/real/data /mnt/filtered
```

### Command-Line Flags

- `-source`: **Required**. Path to the real directory that will be mounted as read-only.
- `-config`: Optional. Path to the config file (default: `/etc/rofs-filtered.rc`).
- `-rules`: Optional. Inline rule expression that overrides the config file (see below).

### Rule Syntax

The rules define a pipeline using `|` syntax. Each rule can contain:

- `filter <expr>`: Expression to filter files. Variables available: `name`, `ext`, `size`, `isDir`
- `sort <field> <asc|desc>`: Sorts by `name` or `size`
- `limit <N>`: Restricts to the first N entries after filtering and sorting

You can provide multiple rules using multiple `-rules` flags or multiple lines in a config file. The union of all rule matches will be shown in the mounted filesystem.

### Examples

#### From config file `/etc/rofs-filtered.rc`
```txt
filter name =~ "file-\d+" | sort size desc | limit 5
filter ext == ".log" && size > 1048576
```

#### Using `-rules` inline:
```bash
rofs-filtered -source /data -rules 'filter name =~ "file-\d+" | sort size desc | limit 3' /mnt/filtered
```

To add multiple `-rules` inline:
```bash
rofs-filtered -source /data \
  -rules 'filter name =~ "file-\d+" | sort size desc | limit 3' \
  -rules 'filter ext == ".log" && size > 1048576' \
  /mnt/filtered
```
