package tel

import (
	"context"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"github.com/rs/zerolog"
)

var (
	logger atomic.Pointer[zerolog.Logger]
	exitFn atomic.Pointer[func(int)]
)

func init() {
	defaultLogger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	logger.Store(&defaultLogger)
	zerolog.DefaultContextLogger = &defaultLogger
	fn := os.Exit
	exitFn.Store(&fn)
}

// LoggerOptions configures the process-global zerolog logger.
//
// goalign:ignore
type LoggerOptions struct {
	Level string
	JSON  bool
}

// InitLogger configures structured logging. Safe to call at startup (and again to reconfigure).
func InitLogger(opts LoggerOptions) {
	lvl, err := zerolog.ParseLevel(strings.TrimSpace(opts.Level))
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	var out io.Writer = os.Stdout
	if !opts.JSON {
		out = zerolog.ConsoleWriter{Out: os.Stdout}
	}
	l := zerolog.New(out).With().Timestamp().Logger()
	SetLogger(l)
}

// applyLoggerFromConfig maps Config.LogLevel / LogEncode onto InitLogger.
func applyLoggerFromConfig(cfg Config) {
	json := true
	switch strings.ToLower(strings.TrimSpace(cfg.LogEncode)) {
	case "console", "text":
		json = false
	}
	InitLogger(LoggerOptions{Level: cfg.LogLevel, JSON: json})
}

// ConfigureLogger applies Config.LogLevel / LogEncode to the process logger.
// Call once at process startup before concurrent logging (not from NewWithConfig).
func ConfigureLogger(cfg Config) {
	applyLoggerFromConfig(cfg)
}

// Logger returns the process-global logger.
func Logger() zerolog.Logger {
	return *logger.Load()
}

func getLogger() *zerolog.Logger {
	return logger.Load()
}

// SetLogger replaces the process-global logger and DefaultContextLogger.
func SetLogger(l zerolog.Logger) {
	logger.Store(&l)
	zerolog.DefaultContextLogger = &l
}

// Ctx returns the logger from ctx, falling back to DefaultContextLogger.
func Ctx(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}

// SetExitFunc overrides the function called after Fatal logs (for tests).
func SetExitFunc(fn func(int)) {
	exitFn.Store(&fn)
}

func Debug() *zerolog.Event {
	return getLogger().Debug()
}

func Info() *zerolog.Event {
	return getLogger().Info()
}

func Warn() *zerolog.Event {
	return getLogger().Warn()
}

func Error() *zerolog.Event {
	return getLogger().Error()
}

// Fatal logs at fatal level and invokes the configured exit function (default os.Exit).
func Fatal() *FatalEvent {
	return &FatalEvent{e: getLogger().WithLevel(zerolog.FatalLevel)}
}

// FatalEvent mirrors zerolog's chainable API but exits via SetExitFunc instead of os.Exit directly.
type FatalEvent struct {
	e *zerolog.Event
}

func (f *FatalEvent) Err(err error) *FatalEvent {
	f.e = f.e.Err(err)

	return f
}

func (f *FatalEvent) Str(key, val string) *FatalEvent {
	f.e = f.e.Str(key, val)

	return f
}

func (f *FatalEvent) Msg(msg string) {
	f.e.Msg(msg)
	(*exitFn.Load())(1)
}

func (f *FatalEvent) Msgf(format string, v ...any) {
	f.e.Msgf(format, v...)
	(*exitFn.Load())(1)
}
