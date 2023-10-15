package log

import (
	"context"
	"io"
	"log/slog"
	"os"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Constants for log formats
const (
	Text = iota // Log format is TEXT.
	Json        // Log format is JSON.
)

// Format represents the log format type.
type Format int

// Options defines configuration options for the logger.
type Options struct {

	// Indicates whether logging is enabled.
	Disable bool `json:"disable" yaml:"disable"`

	// Log format, currently supports Text: 0 and JSON: 1, with Text as the default.
	Format Format `json:"format" yaml:"format"`

	// Log level, with supported values LevelDebug: 4, LevelInfo: 0, LevelWarn: 4, and LevelError: 8.
	Level int `json:"level" yaml:"level"`

	// Filename is the file to write logs to.  Backup log files will be retained
	// in the same directory. If empty, logs will not be written to a file.
	Filename string `json:"filename" yaml:"filename"`

	// MaxSize is the maximum size in megabytes of the log file before it gets
	// rotated. It defaults to 100 megabytes.
	MaxSize int `json:"maxsize" yaml:"maxsize"`

	// MaxAge is the maximum number of days to retain old log files based on the
	// timestamp encoded in their filename.  Note that a day is defined as 24
	// hours and may not exactly correspond to calendar days due to daylight
	// savings, leap seconds, etc. The default is not to remove old log files
	// based on age.
	MaxAge int `json:"maxage" yaml:"maxage"`

	// MaxBackups is the maximum number of old log files to retain.  The default
	// is to retain all old log files (though MaxAge may still cause them to get
	// deleted.)
	MaxBackups int `json:"maxbackups" yaml:"maxbackups"`

	// Compress determines if the rotated log files should be compressed
	// using gzip. The default is not to perform compression.
	Compress bool `json:"compress" yaml:"compress"`
}

// Options defines configuration options for the logger.
func DefaultOptions() *Options {
	return &Options{
		MaxSize:    100,
		MaxAge:     30,
		MaxBackups: 1,
		Format:     Text,
	}
}

// New creates a new Logger based on the provided options.
func New(opt *Options) *Logger {
	if opt == nil {
		opt = DefaultOptions()
	}

	var writer io.Writer
	writer = os.Stdout

	if len(opt.Filename) != 0 {
		fileWriter := &lumberjack.Logger{
			Filename:   opt.Filename,
			MaxSize:    opt.MaxSize,
			MaxBackups: opt.MaxBackups,
			MaxAge:     opt.MaxAge,
			Compress:   opt.Compress,
		}
		writer = io.MultiWriter(os.Stdout, fileWriter)
	}

	return &Logger{
		writer: writer,
		Logger: slog.New(NewHandler(opt, writer)),
		opt:    opt,
	}
}

// Logger is a wrapper for slog.Logger.
type Logger struct {
	*slog.Logger
	opt    *Options
	writer io.Writer
}

// Handler is a wrapper for slog.Handler.
type Handler struct {
	opt      *Options
	internal slog.Handler
}

// NewHandler creates a new handler based on the provided options and writer.
func NewHandler(opt *Options, writer io.Writer) *Handler {
	var handler slog.Handler

	switch opt.Format {
	case Text:
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{
			Level: slog.Level(opt.Level),
		})

	case Json:
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{
			Level: slog.Level(opt.Level),
		})

	default:
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{
			Level: slog.Level(opt.Level),
		})
	}

	return &Handler{
		opt:      opt,
		internal: handler,
	}
}

// Enabled reports whether the handler handles records at the given level.
// The handler ignores records whose level is lower.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return !h.opt.Disable && h.internal.Enabled(ctx, level)
}

// Handle handles the Record.
// It will only be called when Enabled returns true.
func (h *Handler) Handle(ctx context.Context, record slog.Record) error {
	return h.internal.Handle(ctx, record)
}

// WithAttrs returns a new Handler whose attributes consist of
// both the receiver's attributes and the arguments.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.internal.WithAttrs(attrs)
}

// WithGroup returns a new Handler with the given group appended to
// the receiver's existing groups.
// The keys of all subsequent attributes, whether added by With or in a
// Record, should be qualified by the sequence of group names.
func (h *Handler) WithGroup(name string) slog.Handler {
	return h.internal.WithGroup(name)
}
