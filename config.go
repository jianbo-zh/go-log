package log

type Config struct {
	// Format overrides the format of the log output. Defaults to ColorizedOutput
	Format LogFormat

	// Level is the default minimum enabled logging level.
	Level LogLevel

	// SubsystemLevels are the default levels per-subsystem. When unspecified, defaults to Level.
	SubsystemLevels map[string]LogLevel

	// Stderr indicates whether logs should be written to stderr.
	Stderr bool

	// Stdout indicates whether logs should be written to stdout.
	Stdout bool

	// File is a path to a file that logs will be written to.
	File string

	// URL with schema supported by zap. Use zap.RegisterSink
	URL string

	// Labels is a set of key-values to apply to all loggers
	Labels map[string]string
}
