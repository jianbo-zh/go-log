package log

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	envLoggingLvl = "GOLOG_LOG_LEVEL"
	envLoggingFmt = "GOLOG_LOG_FMT"

	envLoggingFile = "GOLOG_FILE" // /path/to/file
	envLoggingURL  = "GOLOG_URL"  // url that will be processed by sink in the zap

	envLoggingOutput = "GOLOG_OUTPUT"     // possible values: stdout|stderr|file combine multiple values with '+'
	envLoggingLabels = "GOLOG_LOG_LABELS" // comma-separated key-value pairs, i.e. "app=example_app,dc=sjc-1"
)

// ErrNoSuchLogger is returned when the util pkg is asked for a non existant logger
var ErrNoSuchLogger = errors.New("error: No such logger")

var loggerMutex sync.RWMutex // guards access to global logger state

// loggers is the set of loggers in the system
var loggers = make(map[string]*zap.SugaredLogger)
var levels = make(map[string]zap.AtomicLevel)

// primaryFormat is the format of the primary core used for logging
var primaryFormat LogFormat = FormatColorizedOutput

// defaultLevel is the default log level
var defaultLevel LogLevel = LevelError

// primaryCore is the primary logging core
var primaryCore zapcore.Core

// loggerCore is the base for all loggers created by this package
var loggerCore = &lockedMultiCore{}

func init() {
	SetupLogging(configFromEnv())
}

// setupLogging will initialize the logger backend and set the flags.
// TODO calling this in `init` pushes all configuration to env variables
// - move it out of `init`? then we need to change all the code (js-ipfs, go-ipfs) to call this explicitly
// - have it look for a config file? need to define what that is
func SetupLogging(cfg Config) {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()

	primaryFormat = cfg.Format
	defaultLevel = cfg.Level

	outputPaths := []string{}

	if cfg.Stderr {
		outputPaths = append(outputPaths, "stderr")
	}
	if cfg.Stdout {
		outputPaths = append(outputPaths, "stdout")
	}

	// check if we log to a file
	if len(cfg.File) > 0 {
		if path, err := normalizePath(cfg.File); err != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve log path '%q', logging to %s\n", cfg.File, outputPaths)
		} else {
			outputPaths = append(outputPaths, path)
		}
	}
	if len(cfg.URL) > 0 {
		outputPaths = append(outputPaths, cfg.URL)
	}

	outputs, _, err := zap.Open(outputPaths...)
	if err != nil {
		panic(fmt.Sprintf("unable to open logging output: %v", err))
	}

	newPrimaryCore := newCore(primaryFormat, outputs, LevelDebug) // the main core needs to log everything.

	for k, v := range cfg.Labels {
		newPrimaryCore = newPrimaryCore.With([]zap.Field{zap.String(k, v)})
	}

	setPrimaryCore(newPrimaryCore)
	setAllLoggerLevel(defaultLevel)

	for name, level := range cfg.SubsystemLevels {
		if leveler, ok := levels[name]; ok {
			leveler.SetLevel(zapcore.Level(level))
		} else {
			levels[name] = zap.NewAtomicLevelAt(zapcore.Level(level))
		}
	}
}

// configFromEnv returns a Config with defaults populated using environment variables.
func configFromEnv() Config {
	cfg := Config{
		Format:          FormatColorizedOutput,
		Stderr:          true,
		Level:           LevelError,
		SubsystemLevels: map[string]LogLevel{},
		Labels:          map[string]string{},
	}

	format := os.Getenv(envLoggingFmt)

	var noExplicitFormat bool

	switch format {
	case "color":
		cfg.Format = FormatColorizedOutput
	case "nocolor":
		cfg.Format = FormatPlaintextOutput
	case "json":
		cfg.Format = FormatJSONOutput
	default:
		if format != "" {
			fmt.Fprintf(os.Stderr, "ignoring unrecognized log format '%s'\n", format)
		}
		noExplicitFormat = true
	}

	lvl := os.Getenv(envLoggingLvl)
	if lvl != "" {
		for _, kvs := range strings.Split(lvl, ",") {
			kv := strings.SplitN(kvs, "=", 2)
			lvl, err := LevelFromString(kv[len(kv)-1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error setting log level %q: %s\n", kvs, err)
				continue
			}
			switch len(kv) {
			case 1:
				cfg.Level = lvl
			case 2:
				cfg.SubsystemLevels[kv[0]] = lvl
			}
		}
	}

	cfg.File = os.Getenv(envLoggingFile)
	// Disable stderr logging when a file is specified
	// https://github.com/ipfs/go-log/issues/83
	if cfg.File != "" {
		cfg.Stderr = false
	}

	cfg.URL = os.Getenv(envLoggingURL)
	output := os.Getenv(envLoggingOutput)
	outputOptions := strings.Split(output, "+")
	for _, opt := range outputOptions {
		switch opt {
		case "stdout":
			cfg.Stdout = true
		case "stderr":
			cfg.Stderr = true
		case "file":
			if cfg.File == "" {
				fmt.Fprint(os.Stderr, "please specify a GOLOG_FILE value to write to")
			}
		case "url":
			if cfg.URL == "" {
				fmt.Fprint(os.Stderr, "please specify a GOLOG_URL value to write to")
			}
		}
	}

	// Check that neither of the requested Std* nor the file are TTYs
	// At this stage (configFromEnv) we do not have a uniform list to examine yet
	if noExplicitFormat &&
		!(cfg.Stdout && isTerm(os.Stdout)) &&
		!(cfg.Stderr && isTerm(os.Stderr)) &&
		// check this last: expensive
		!(cfg.File != "" && pathIsTerm(cfg.File)) {
		cfg.Format = FormatPlaintextOutput
	}

	labels := os.Getenv(envLoggingLabels)
	if labels != "" {
		labelKVs := strings.Split(labels, ",")
		for _, label := range labelKVs {
			kv := strings.Split(label, "=")
			if len(kv) != 2 {
				fmt.Fprint(os.Stderr, "invalid label k=v: ", label)
				continue
			}
			cfg.Labels[kv[0]] = kv[1]
		}
	}

	return cfg
}

func isTerm(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

func pathIsTerm(p string) bool {
	// !!!no!!! O_CREAT, if we fail - we fail
	f, err := os.OpenFile(p, os.O_WRONLY, 0)
	if f != nil {
		defer f.Close() // nolint:errcheck
	}
	return err == nil && isTerm(f)
}

func setPrimaryCore(core zapcore.Core) {
	if primaryCore != nil {
		loggerCore.ReplaceCore(primaryCore, core)
	} else {
		loggerCore.AddCore(core)
	}
	primaryCore = core
}

func setAllLoggerLevel(lvl LogLevel) {
	for _, l := range levels {
		l.SetLevel(zapcore.Level(lvl))
	}
}
