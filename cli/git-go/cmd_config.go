package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	format "github.com/go-git/go-git/v6/plumbing/format/config"
)

func cmdConfig(args []string) int {
	var (
		scope     string // "local", "global", "system"
		getBool   bool
		getInt    bool
		doGet     bool
		doUnset   bool
		doList    bool
		doAdd     bool
		key       string
		value     string
		hasValue  bool
	)

	positional := []string{}
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--local":
			scope = "local"
		case "--global":
			scope = "global"
		case "--system":
			scope = "system"
		case "--bool":
			getBool = true
		case "--int":
			getInt = true
		case "--get":
			doGet = true
		case "--unset":
			doUnset = true
		case "--list", "-l":
			doList = true
		case "--add":
			doAdd = true
		case "--default":
			// skip value
			i++
		case "--type":
			// skip value (e.g., --type=bool)
			i++
		case "--null", "-z":
			// accepted, ignored for now
		default:
			if strings.HasPrefix(a, "--type=") {
				tp := strings.TrimPrefix(a, "--type=")
				switch tp {
				case "bool":
					getBool = true
				case "int":
					getInt = true
				}
			} else if strings.HasPrefix(a, "--default=") {
				// accepted, ignored
			} else if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
			// Unknown flags: ignore gracefully.
		}
		i++
	}

	_ = getInt

	if scope == "global" || scope == "system" {
		fmt.Fprintf(os.Stderr, "warning: go-git only supports --local config; --%s is not implemented\n", scope)
	}

	if doList {
		return configList()
	}

	// Parse positional args: git config [--flags] <key> [<value>]
	if doGet && len(positional) > 0 {
		key = positional[0]
	} else if doUnset && len(positional) > 0 {
		key = positional[0]
	} else if len(positional) >= 2 {
		key = positional[0]
		value = positional[1]
		hasValue = true
	} else if len(positional) == 1 {
		key = positional[0]
		if !doGet {
			doGet = true
		}
	}

	if key == "" {
		fmt.Fprintln(os.Stderr, "error: key does not contain a section: ")
		return 2
	}

	if doUnset {
		return configUnset(key)
	}
	if hasValue && !doGet {
		if doAdd {
			return configAdd(key, value)
		}
		return configSet(key, value)
	}
	return configGet(key, getBool)
}

// parseConfigKey splits "section.key" or "section.subsection.key" into parts.
func parseConfigKey(key string) (section, subsection, name string) {
	parts := strings.SplitN(key, ".", 3)
	switch len(parts) {
	case 2:
		return parts[0], "", parts[1]
	case 3:
		return parts[0], parts[1], parts[2]
	default:
		return key, "", ""
	}
}

func configPath() string {
	return filepath.Join(gitDir(), "config")
}

func readRawConfig() (*format.Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return format.New(), nil
		}
		return nil, err
	}
	cfg := format.New()
	if err := format.NewDecoder(bytes.NewReader(data)).Decode(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func writeRawConfig(cfg *format.Config) error {
	var buf bytes.Buffer
	if err := format.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	return os.WriteFile(configPath(), buf.Bytes(), 0o644)
}

func configGet(key string, asBool bool) int {
	// -c overrides take precedence over on-disk config.
	if globalConfigOverrides != nil {
		if val, ok := globalConfigOverrides[key]; ok {
			return printConfigValue(val, key, asBool)
		}
	}

	cfg, err := readRawConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: unable to read config: %s\n", err)
		return 128
	}

	section, subsection, name := parseConfigKey(key)

	var val string
	var found bool
	for _, s := range cfg.Sections {
		if !s.IsName(section) {
			continue
		}
		if subsection == "" {
			if s.HasOption(name) {
				val = s.Option(name)
				found = true
			}
		} else {
			for _, ss := range s.Subsections {
				if ss.IsName(subsection) && ss.HasOption(name) {
					val = ss.Option(name)
					found = true
				}
			}
		}
	}

	if !found {
		return 1
	}

	return printConfigValue(val, key, asBool)
}

func printConfigValue(val, key string, asBool bool) int {
	if asBool {
		switch strings.ToLower(val) {
		case "true", "yes", "on", "1", "":
			fmt.Println("true")
		case "false", "no", "off", "0":
			fmt.Println("false")
		default:
			fmt.Fprintf(os.Stderr, "fatal: bad boolean config value '%s' for '%s'\n", val, key)
			return 128
		}
	} else {
		fmt.Println(val)
	}
	return 0
}

func configSet(key, value string) int {
	cfg, err := readRawConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: unable to read config: %s\n", err)
		return 128
	}

	section, subsection, name := parseConfigKey(key)
	cfg.SetOption(section, subsection, name, value)

	if err := writeRawConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: unable to write config: %s\n", err)
		return 128
	}
	return 0
}

func configAdd(key, value string) int {
	cfg, err := readRawConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: unable to read config: %s\n", err)
		return 128
	}

	section, subsection, name := parseConfigKey(key)
	cfg.AddOption(section, subsection, name, value)

	if err := writeRawConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: unable to write config: %s\n", err)
		return 128
	}
	return 0
}

func configUnset(key string) int {
	cfg, err := readRawConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: unable to read config: %s\n", err)
		return 128
	}

	section, subsection, name := parseConfigKey(key)

	for _, s := range cfg.Sections {
		if !s.IsName(section) {
			continue
		}
		if subsection == "" {
			s.RemoveOption(name)
		} else {
			for _, ss := range s.Subsections {
				if ss.IsName(subsection) {
					ss.RemoveOption(name)
				}
			}
		}
	}

	if err := writeRawConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: unable to write config: %s\n", err)
		return 128
	}
	return 0
}

func configList() int {
	cfg, err := readRawConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: unable to read config: %s\n", err)
		return 128
	}

	for _, s := range cfg.Sections {
		for _, o := range s.Options {
			fmt.Printf("%s.%s=%s\n", strings.ToLower(s.Name), o.Key, o.Value)
		}
		for _, ss := range s.Subsections {
			for _, o := range ss.Options {
				fmt.Printf("%s.%s.%s=%s\n", strings.ToLower(s.Name), ss.Name, o.Key, o.Value)
			}
		}
	}

	// Append -c overrides (they take precedence, listed last like real git).
	for k, v := range globalConfigOverrides {
		fmt.Printf("%s=%s\n", k, v)
	}

	return 0
}
