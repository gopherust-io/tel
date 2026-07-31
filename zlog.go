package tel

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
)

var (
	logger            atomic.Pointer[zerolog.Logger]
	exitFn            atomic.Pointer[func(int)]
	consoleColorsOnce sync.Once
)

func init() {
	zerolog.CallerMarshalFunc = marshalCaller
	zerolog.ErrorStackMarshaler = marshalErrorStack //nolint:reassign // configure process-wide zerolog stack marshaler
	defaultLogger := newLogger(os.Stdout)
	logger.Store(&defaultLogger)
	zerolog.DefaultContextLogger = &defaultLogger
	fn := os.Exit
	exitFn.Store(&fn)
}

func marshalCaller(pc uintptr, _ string, line int) string {
	name := "???"
	if fn := runtime.FuncForPC(pc); fn != nil {
		name = fn.Name()
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
	}

	return name + ":" + strconv.Itoa(line)
}

type uintptrStackTracer interface {
	StackTrace() []uintptr
}

// stackFrame is a single call-site entry in Err() stack output.
type stackFrame struct {
	Func   string `json:"func"`
	Source string `json:"source"`
	Line   int    `json:"line"`
}

func marshalErrorStack(err error) interface{} {
	if err == nil {
		return nil
	}
	if frames := stackFromTracer(err); len(frames) > 0 {
		return frames
	}

	return captureRuntimeStack()
}

func stackFromTracer(err error) []stackFrame {
	for err != nil {
		if st, ok := err.(uintptrStackTracer); ok {
			return framesFromPCs(st.StackTrace())
		}
		err = errors.Unwrap(err)
	}

	return nil
}

func captureRuntimeStack() []stackFrame {
	var pcs [64]uintptr
	// Skip Callers + captureRuntimeStack.
	n := runtime.Callers(2, pcs[:])
	if n == 0 {
		return nil
	}

	return framesFromPCs(pcs[:n])
}

func framesFromPCs(pcs []uintptr) []stackFrame {
	if len(pcs) == 0 {
		return nil
	}
	frames := runtime.CallersFrames(pcs)
	out := make([]stackFrame, 0, len(pcs))
	for {
		frame, more := frames.Next()
		if frame.Function != "" && !skipStackFrame(frame.Function) {
			name := frame.Function
			if i := strings.LastIndex(name, "/"); i >= 0 {
				name = name[i+1:]
			}
			file := frame.File
			if i := strings.LastIndex(file, "/"); i >= 0 {
				file = file[i+1:]
			}
			out = append(out, stackFrame{
				Func:   name,
				Source: file,
				Line:   frame.Line,
			})
		}
		if !more {
			break
		}
	}

	return out
}

func skipStackFrame(fn string) bool {
	switch {
	case strings.Contains(fn, "github.com/rs/zerolog"):
		return true
	case strings.Contains(fn, "marshalErrorStack"):
		return true
	case strings.Contains(fn, "captureRuntimeStack"):
		return true
	case strings.Contains(fn, "stackFromTracer"):
		return true
	case strings.Contains(fn, "framesFromPCs"):
		return true
	case strings.Contains(fn, "tel.(*FatalEvent).Err"):
		return true
	default:
		return false
	}
}

func newLogger(out io.Writer) zerolog.Logger {
	return zerolog.New(out).With().Timestamp().Caller().Stack().Logger()
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
		applyConsoleLevelColors()
		out = zerolog.ConsoleWriter{Out: os.Stdout}
	}
	SetLogger(newLogger(out))
}

func applyConsoleLevelColors() {
	consoleColorsOnce.Do(func() {
		zerolog.LevelColors[zerolog.DebugLevel] = 34
		zerolog.LevelColors[zerolog.InfoLevel] = 32
		zerolog.LevelColors[zerolog.WarnLevel] = 33
		zerolog.LevelColors[zerolog.ErrorLevel] = 31
		zerolog.LevelColors[zerolog.FatalLevel] = 35
		zerolog.LevelColors[zerolog.PanicLevel] = 35
	})
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
	// Skip FatalEvent.Msg/Msgf so caller points at the user site.
	return &FatalEvent{e: getLogger().WithLevel(zerolog.FatalLevel).CallerSkipFrame(1)}
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
