package cli

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/config"
)

func newConfigCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Get or set qvm configuration values",
		Commands: []*cli.Command{
			{
				Name:      "get",
				Usage:     "Get a configuration value",
				ArgsUsage: "<key>",
				Action:    runConfigGet,
			},
			{
				Name:      "set",
				Usage:     "Set a configuration value",
				ArgsUsage: "<key> <value>",
				Action:    runConfigSet,
			},
			{
				Name:   "list",
				Usage:  "List all configuration values",
				Action: runConfigList,
			},
			{
				Name:   "path",
				Usage:  "Print the path to the config file",
				Action: runConfigPath,
			},
		},
		// If no sub-command, show list.
		Action: runConfigList,
	}
}

func runConfigGet(ctx context.Context, cmd *cli.Command) error {
	_ = ctx

	key := cmd.Args().Get(0)
	if key == "" {
		return fmt.Errorf("argument required; run 'qvm config get --help' for usage")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	val, err := configGet(cfg, key)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "%v\n", val)
	return nil
}

func runConfigSet(ctx context.Context, cmd *cli.Command) error {
	_ = ctx

	key := cmd.Args().Get(0)
	value := cmd.Args().Get(1)
	if key == "" || value == "" {
		return fmt.Errorf("key and value required; run 'qvm config set --help' for usage")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	err = configSet(cfg, key, value)
	if err != nil {
		return err
	}

	err = config.Save(cfg)
	if err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Set %s = %s\n", key, value)
	return nil
}

func runConfigList(ctx context.Context, cmd *cli.Command) error {
	_ = ctx
	_ = cmd

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	pairs := configList(cfg)
	for _, kv := range pairs {
		fmt.Fprintf(os.Stdout, "%-40s = %v\n", kv[0], kv[1])
	}
	return nil
}

// configKey maps a dot-separated config key to a chain of struct field names.
// The config struct uses nested anonymous structs, so we use a manual map.
var configKeyMap = map[string][]string{ //nolint:gochecknoglobals // package-level config key registry
	"install.dir":              {"Install", "Dir"},
	"install.tools_dir":        {"Install", "ToolsDir"},
	"repository.url":           {"Repository", "URL"},
	"repository.mirrors":       {"Repository", "Mirrors"},
	"repository.blacklist":     {"Repository", "Blacklist"},
	"download.concurrency":     {"Download", "Concurrency"},
	"download.timeout_seconds": {"Download", "TimeoutSeconds"},
}

func configGet(cfg *config.Config, key string) (any, error) {
	fields, ok := configKeyMap[normalizeConfigKey(key)]
	if !ok {
		return nil, fmt.Errorf("unknown config key %q; valid keys: %s", key, validKeys())
	}

	v := reflect.ValueOf(cfg).Elem()
	for _, f := range fields {
		v = v.FieldByName(f)
		if !v.IsValid() {
			return nil, fmt.Errorf("internal: field %s not found", f)
		}
	}
	return v.Interface(), nil
}

func configSet(cfg *config.Config, key, value string) error {
	fields, ok := configKeyMap[normalizeConfigKey(key)]
	if !ok {
		return fmt.Errorf("unknown config key %q; valid keys: %s", key, validKeys())
	}

	v := reflect.ValueOf(cfg).Elem()
	for _, f := range fields {
		v = v.FieldByName(f)
		if !v.IsValid() {
			return fmt.Errorf("internal: field %s not found", f)
		}
	}

	if !v.CanSet() {
		return fmt.Errorf("config field %q is not settable", key)
	}

	switch v.Kind() {
	case reflect.String:
		v.SetString(value)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer value %q for key %q: %w", value, key, err)
		}
		v.SetInt(n)
	case reflect.Slice:
		// For slices (e.g. mirrors), split by comma.
		parts := strings.Split(value, ",")
		trimmed := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				trimmed = append(trimmed, p)
			}
		}
		v.Set(reflect.ValueOf(trimmed))
	default:
		return fmt.Errorf("unsupported config field type %s for key %q", v.Kind(), key)
	}
	return nil
}

func configList(cfg *config.Config) [][2]string {
	keys := make([]string, 0, len(configKeyMap))
	for key := range configKeyMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var pairs [][2]string
	for _, key := range keys {
		val, err := configGet(cfg, key)
		if err != nil {
			continue
		}
		var valStr string
		rv := reflect.ValueOf(val)
		if rv.Kind() == reflect.Slice {
			strs := make([]string, rv.Len())
			for i := range rv.Len() {
				strs[i] = fmt.Sprintf("%v", rv.Index(i).Interface())
			}
			valStr = strings.Join(strs, ", ")
		} else {
			valStr = fmt.Sprintf("%v", val)
		}
		pairs = append(pairs, [2]string{key, valStr})
	}
	return pairs
}

func runConfigPath(ctx context.Context, cmd *cli.Command) error {
	_ = ctx
	_ = cmd

	p, err := config.Path()
	if err != nil {
		return fmt.Errorf("determining config path: %w", err)
	}
	fmt.Fprintln(os.Stdout, p)
	return nil
}

// normalizeConfigKey lowercases and converts hyphens to underscores so that
// both "download.timeout_seconds" and "download.timeout-seconds" work.
func normalizeConfigKey(key string) string {
	return strings.ReplaceAll(strings.ToLower(key), "-", "_")
}

func validKeys() string {
	keys := make([]string, 0, len(configKeyMap))
	for k := range configKeyMap {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
