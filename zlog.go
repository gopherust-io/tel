package tel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
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
	Level  string
	JSON   bool
	Pretty bool // indent JSON lines (only when JSON is true)
}

// InitLogger configures structured logging. Safe to call at startup (and again to reconfigure).
func InitLogger(opts LoggerOptions) {
	lvl, err := zerolog.ParseLevel(strings.TrimSpace(opts.Level))
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(lvl)

	var out io.Writer = os.Stdout
	switch {
	case !opts.JSON:
		applyConsoleLevelColors()
		out = zerolog.ConsoleWriter{
			Out:           os.Stdout,
			FieldsExclude: []string{"stack"},
			FormatExtra:   formatConsoleStackExtra,
		}
	case opts.Pretty:
		out = prettyJSONWriter{w: os.Stdout}
	}
	SetLogger(newLogger(out))
}

// formatConsoleStackExtra appends a multi-line stack under the console log line.
func formatConsoleStackExtra(evt map[string]interface{}, buf *bytes.Buffer) error {
	v, ok := evt["stack"]
	if !ok {
		return nil
	}
	frames, ok := parseStackFrames(v)
	if !ok {
		return nil
	}
	buf.WriteByte('\n')
	buf.WriteString("stack:")
	for _, frame := range frames {
		buf.WriteByte('\n')
		buf.WriteString("  ")
		buf.WriteString(frame.Func)
		buf.WriteByte(' ')
		buf.WriteString(frame.Source)
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(frame.Line))
	}

	return nil
}

func parseStackFrames(value interface{}) ([]stackFrame, bool) {
	switch t := value.(type) {
	case []stackFrame:
		return t, len(t) > 0
	case []byte:
		return unmarshalStackFrames(t)
	case []interface{}:
		return stackFramesFromMaps(t)
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}

		return unmarshalStackFrames(raw)
	}
}

func unmarshalStackFrames(raw []byte) ([]stackFrame, bool) {
	var frames []stackFrame
	if json.Unmarshal(raw, &frames) != nil || len(frames) == 0 {
		return nil, false
	}

	return frames, true
}

func stackFramesFromMaps(items []interface{}) ([]stackFrame, bool) {
	frames := make([]stackFrame, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		frame := stackFrame{}
		if s, ok := raw["func"].(string); ok {
			frame.Func = s
		}
		if s, ok := raw["source"].(string); ok {
			frame.Source = s
		}
		switch line := raw["line"].(type) {
		case float64:
			frame.Line = int(line)
		case json.Number:
			n, _ := line.Int64()
			frame.Line = int(n)
		case int:
			frame.Line = line
		}
		if frame.Func != "" {
			frames = append(frames, frame)
		}
	}

	return frames, len(frames) > 0
}

// prettyJSONWriter indents each JSON log line for local readability.
type prettyJSONWriter struct {
	w io.Writer
}

func (p prettyJSONWriter) Write(data []byte) (int, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, bytes.TrimSpace(data), "", "  "); err != nil {
		return p.w.Write(data)
	}
	buf.WriteByte('\n')
	if _, err := p.w.Write(buf.Bytes()); err != nil {
		return 0, err
	}

	return len(data), nil
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

// applyLoggerFromConfig maps Config.LogLevel / LogEncode onto InitLogger and
// attaches non-empty service resource fields to the process logger.
func applyLoggerFromConfig(cfg Config) {
	opts := LoggerOptions{Level: cfg.LogLevel, JSON: true}
	switch strings.ToLower(strings.TrimSpace(cfg.LogEncode)) {
	case "console", "text":
		opts.JSON = false
	case "pretty", "json_pretty", "json-pretty":
		opts.Pretty = true
	}
	InitLogger(opts)
	l := withServiceFields(Logger(), cfg)
	if cfg.MaxMessagesPerSecond > 0 || strings.TrimSpace(cfg.MaxLevelMessagesPerSecond) != "" {
		limits := parseLevelMessageLimits(cfg.MaxLevelMessagesPerSecond)
		l = l.Sample(NewRateSampler(cfg.MaxMessagesPerSecond, limits))
	}
	SetLogger(l)
}

func withServiceFields(l zerolog.Logger, cfg Config) zerolog.Logger {
	c := l.With()
	if s := strings.TrimSpace(cfg.Service); s != "" {
		c = c.Str(FieldService, s)
	}
	if s := resolvePod(cfg); s != "" {
		c = c.Str(FieldPod, s)
	}
	if s := strings.TrimSpace(cfg.Namespace); s != "" {
		c = c.Str(FieldNamespace, s)
	}
	if s := strings.TrimSpace(cfg.Environment); s != "" {
		c = c.Str(FieldEnvironment, s)
	}
	if s := strings.TrimSpace(cfg.Version); s != "" {
		c = c.Str(FieldVersion, s)
	}

	return c.Logger()
}

// resolvePod returns cfg.Pod, else POD_NAME/HOSTNAME env, else os.Hostname.
func resolvePod(cfg Config) string {
	if s := strings.TrimSpace(cfg.Pod); s != "" {
		return s
	}
	if s := strings.TrimSpace(os.Getenv("POD_NAME")); s != "" {
		return s
	}
	if s := strings.TrimSpace(os.Getenv("HOSTNAME")); s != "" {
		return s
	}
	host, err := os.Hostname()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(host)
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

// Ctx returns a logger for ctx. When ctx carries a valid OTel span and/or
// WithFields bag, a child logger is built once; otherwise the context/process logger is reused.
func Ctx(ctx context.Context) *zerolog.Logger {
	sc := trace.SpanContextFromContext(ctx)
	fields := fieldsFromCtx(ctx)
	if !sc.IsValid() && len(fields) == 0 {
		return zerolog.Ctx(ctx)
	}
	c := Logger().With()
	if sc.IsValid() {
		c = c.Str(FieldTraceID, sc.TraceID().String()).Str(FieldSpanID, sc.SpanID().String())
	}
	if len(fields) > 0 {
		c = applyFields(c, fields)
	}
	l := c.Logger()

	return &l
}

// contextWithTraceLogger stores a span/fields-enriched logger on ctx for zerolog.Ctx.
func contextWithTraceLogger(ctx context.Context) context.Context {
	sc := trace.SpanContextFromContext(ctx)
	fields := fieldsFromCtx(ctx)
	if !sc.IsValid() && len(fields) == 0 {
		return ctx
	}
	c := Logger().With()
	if sc.IsValid() {
		c = c.Str(FieldTraceID, sc.TraceID().String()).Str(FieldSpanID, sc.SpanID().String())
	}
	if len(fields) > 0 {
		c = applyFields(c, fields)
	}

	return c.Logger().WithContext(ctx)
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

func DebugCtx(ctx context.Context) *zerolog.Event {
	return Ctx(ctx).Debug()
}

func InfoCtx(ctx context.Context) *zerolog.Event {
	return Ctx(ctx).Info()
}

func WarnCtx(ctx context.Context) *zerolog.Event {
	return Ctx(ctx).Warn()
}

func ErrorCtx(ctx context.Context) *zerolog.Event {
	return Ctx(ctx).Error()
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
