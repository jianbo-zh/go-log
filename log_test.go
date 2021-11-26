package log

import (
	"testing"
)

func TestLogger(t *testing.T) {

	log := Logger("abcd")
	SetLogLevel("abcd", "debug")

	log.Debug("hello", "word")
	log.Debugw("hello world",
		"hello", "world",
	)
}
