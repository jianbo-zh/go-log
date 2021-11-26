package log

import (
	"regexp"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger retrieves an event logger by name
func Logger(system string) *ZapEventLogger {
	if len(system) == 0 {
		setuplog := getLogger("setup-logger")
		setuplog.Error("Missing name parameter")
		system = "undefined"
	}

	logger := getLogger(system)
	skipLogger := logger.Desugar().WithOptions(zap.AddCallerSkip(1)).Sugar()

	return &ZapEventLogger{
		system:        system,
		SugaredLogger: *logger,
		skipLogger:    *skipLogger,
	}
}

// SetLogLevel changes the log level of a specific subsystem
// name=="*" changes all subsystems
func SetLogLevel(name, level string) error {
	lvl, err := LevelFromString(level)
	if err != nil {
		return err
	}

	loggerMutex.RLock()
	defer loggerMutex.RUnlock()

	// wildcard, change all
	if name == "*" {
		setAllLoggerLevel(lvl)
		return nil
	}

	// Check if we have a logger by that name
	if _, ok := levels[name]; !ok {
		return ErrNoSuchLogger
	}

	levels[name].SetLevel(zapcore.Level(lvl))

	return nil
}

// SetLogLevelRegex sets all loggers to level `l` that match expression `e`.
// An error is returned if `e` fails to compile.
func SetLogLevelRegex(e, l string) error {
	lvl, err := LevelFromString(l)
	if err != nil {
		return err
	}

	rem, err := regexp.Compile(e)
	if err != nil {
		return err
	}

	loggerMutex.Lock()
	defer loggerMutex.Unlock()

	for name := range loggers {
		if rem.MatchString(name) {
			levels[name].SetLevel(zapcore.Level(lvl))
		}
	}
	return nil
}

// SetPrimaryCore changes the primary logging core. If the SetupLogging was
// called then the previously configured core will be replaced.
func SetPrimaryCore(core zapcore.Core) {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()

	setPrimaryCore(core)
}

// GetSubsystems returns a slice containing the
// names of the current loggers
func GetSubsystems() []string {
	loggerMutex.RLock()
	defer loggerMutex.RUnlock()

	subs := make([]string, 0, len(loggers))

	for k := range loggers {
		subs = append(subs, k)
	}
	return subs
}

func getLogger(name string) *zap.SugaredLogger {
	loggerMutex.Lock()
	defer loggerMutex.Unlock()

	log, ok := loggers[name]
	if !ok {
		level, ok := levels[name]
		if !ok {
			level = zap.NewAtomicLevelAt(zapcore.Level(defaultLevel))
			levels[name] = level
		}
		log = zap.New(loggerCore).
			WithOptions(
				zap.IncreaseLevel(level),
				zap.AddCaller(),
			).
			Named(name).
			Sugar()

		loggers[name] = log
	}

	return log
}
