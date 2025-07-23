package main

import (
	"bazil.org/fuse"
	fusefs "bazil.org/fuse/fs"
	"context"
	"fmt"
	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/spf13/cobra"
	stdfs "io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var (
   sourcePath string
   configPath string
   ruleExprs  []string
)
var rootCmd = &cobra.Command{
   Use:   "roffs [flags] SOURCE TARGET",
   Short: "Mount a read-only filesystem with filtering rules",
   Args:  cobra.ExactArgs(2),
   PreRunE: func(cmd *cobra.Command, args []string) error {
       if configPath == "" && len(ruleExprs) == 0 {
           return fmt.Errorf("either --config or --rules must be provided")
       }
       return nil
   },
   RunE: func(cmd *cobra.Command, args []string) error {
       // assign source and target
       sourcePath = args[0]
       mountpoint := args[1]
       // parse inline rules or config file
       if len(ruleExprs) > 0 {
           for _, expr := range ruleExprs {
               if err := parseInlineRules(expr); err != nil {
                   return fmt.Errorf("failed to parse inline rules: %w", err)
               }
           }
       } else {
           if err := parseConfig(configPath); err != nil {
               return fmt.Errorf("failed to parse config: %w", err)
           }
       }
       // mount filesystem
       c, err := fuse.Mount(mountpoint, fuse.ReadOnly())
       if err != nil {
           return err
       }
       defer c.Close()
       if err := fusefs.Serve(c, FS{}); err != nil {
           return err
       }
       return nil
   },
}

func init() {
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	rootCmd.Flags().StringArrayVarP(&ruleExprs, "rules", "r", []string{}, "Inline rule expression(s) (overrides config file)")
}
func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type Rule struct {
	Filter    string
	SortBy    string
	SortOrder string
	Limit     int
	Program   *vm.Program
	Files     map[string]bool
}

type Config struct {
	ExtPriority   map[string]string
	FilteredTypes map[stdfs.FileMode]bool
	Rules         []*Rule
}

var config Config

func parseConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	config.ExtPriority = make(map[string]string)
	config.FilteredTypes = make(map[stdfs.FileMode]bool)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		err := parseRulePipeline(line)
		if err != nil {
			return err
		}
	}
	return nil
}

func parseInlineRules(rules string) error {
	return parseRulePipeline(rules)
}

func parseRulePipeline(line string) error {
	parts := strings.Split(line, "|")
	var current *Rule

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "filter":
			current = &Rule{Files: make(map[string]bool)}
			exprStr := strings.Join(fields[1:], " ")
			current.Filter = exprStr
			program, err := expr.Compile(exprStr, expr.Env(map[string]interface{}{
				"name":  "",
				"size":  int64(0),
				"ext":   "",
				"isDir": false,
			}))
			if err != nil {
				return err
			}
			current.Program = program
			config.Rules = append(config.Rules, current)
		case "sort":
			if current != nil && len(fields) >= 3 {
				current.SortBy = fields[1]
				current.SortOrder = fields[2]
			}
		case "limit":
			if current != nil && len(fields) >= 2 {
				if n, err := strconv.Atoi(fields[1]); err == nil {
					current.Limit = n
				}
			}
		}
	}
	return nil
}

type FS struct{}

func (FS) Root() (fusefs.Node, error) {
	return &Dir{path: "/"}, nil
}

type Dir struct {
	path string
}

func (d *Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	info, err := os.Lstat(realPath(d.path))
	if err != nil {
		return err
	}
	a.Mode = info.Mode()
	return nil
}

func (d *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	entries, err := os.ReadDir(realPath(d.path))
	if err != nil {
		return nil, err
	}

	type fileInfo struct {
		name  string
		size  int64
		ext   string
		isDir bool
	}
	var all []fileInfo
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		all = append(all, fileInfo{
			name:  e.Name(),
			size:  info.Size(),
			ext:   filepath.Ext(e.Name()),
			isDir: info.IsDir(),
		})
	}

	for _, rule := range config.Rules {
		var matched []fileInfo
		for _, fi := range all {
			env := map[string]interface{}{
				"name":  fi.name,
				"size":  fi.size,
				"ext":   fi.ext,
				"isDir": fi.isDir,
			}
			ok, err := expr.Run(rule.Program, env)
			if err == nil && ok == true {
				matched = append(matched, fi)
			}
		}
		if rule.SortBy == "name" {
			sort.Slice(matched, func(i, j int) bool {
				if rule.SortOrder == "desc" {
					return matched[i].name > matched[j].name
				}
				return matched[i].name < matched[j].name
			})
		} else if rule.SortBy == "size" {
			sort.Slice(matched, func(i, j int) bool {
				if rule.SortOrder == "desc" {
					return matched[i].size > matched[j].size
				}
				return matched[i].size < matched[j].size
			})
		}
		if rule.Limit > 0 && len(matched) > rule.Limit {
			matched = matched[:rule.Limit]
		}
		for _, fi := range matched {
			rule.Files[fi.name] = true
		}
	}

	var out []fuse.Dirent
	for _, fi := range all {
		if !shouldShow(fi.name) {
			continue
		}
		ent := fuse.Dirent{Name: fi.name}
		if fi.isDir {
			ent.Type = fuse.DT_Dir
		} else {
			ent.Type = fuse.DT_File
		}
		out = append(out, ent)
	}
	return out, nil
}

func (d *Dir) Lookup(ctx context.Context, name string) (fusefs.Node, error) {
	if !shouldShow(name) {
		return nil, fuse.ENOENT
	}
	full := filepath.Join(d.path, name)
	info, err := os.Lstat(realPath(full))
	if err != nil {
		return nil, fuse.ENOENT
	}
	if info.IsDir() {
		return &Dir{path: full}, nil
	}
	return &File{path: full}, nil
}

type File struct {
	path string
}

func (f *File) Attr(ctx context.Context, a *fuse.Attr) error {
	info, err := os.Lstat(realPath(f.path))
	if err != nil {
		return err
	}
	a.Mode = info.Mode()
	a.Size = uint64(info.Size())
	return nil
}

func (f *File) ReadAll(ctx context.Context) ([]byte, error) {
	return os.ReadFile(realPath(f.path))
}

func realPath(path string) string {
	return filepath.Join(sourcePath, path)
}

func shouldShow(name string) bool {
	for _, rule := range config.Rules {
		if rule.Files[name] {
			return true
		}
	}
	return false
}
