package log

type LogFormat int

const (
	FormatColorizedOutput LogFormat = iota
	FormatPlaintextOutput
	FormatJSONOutput
)
