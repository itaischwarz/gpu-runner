package logger

import (
	"context"
	"encoding/json"
	"time"
)

// StreamSink is an interface for appending logs to a stream
type StreamSink interface {
	Append(ctx context.Context, streamKey, data string) error
}

/*
 ─────────────────────────────────────────────
 Core Types
 ─────────────────────────────────────────────
*/

type Field struct {
	Key   string
	Value any
}

type LogEntry struct {
	Level     string
	Message   string
	Timestamp time.Time
	Fields    []Field
}

// Wire format (what actually gets stored)
type wireLogEntry struct {
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	Timestamp time.Time      `json:"timestamp"`
	Fields    map[string]any `json:"fields,omitempty"`
}

/*
 ─────────────────────────────────────────────
 Logger
 ─────────────────────────────────────────────
*/

type JobLogger struct {
	ctx   context.Context
	jobID string
	sink  StreamSink
	base  []Field
}

func NewJobLogger(
	ctx context.Context,
	jobID string,
	sink StreamSink,
) *JobLogger {
	if ctx == nil {
		ctx = context.Background()
	}

	return &JobLogger{
		ctx:   ctx,
		jobID: jobID,
		sink:  sink,
		base: []Field{
			String("job_id", jobID),
		},
	}
}

/*
 ─────────────────────────────────────────────
 Logging API
 ─────────────────────────────────────────────
*/

func (l *JobLogger) Info(msg string, fields ...Field) {
	l.log("info", msg, fields...)
}

func (l *JobLogger) Error(msg string, fields ...Field) {
	l.log("error", msg, fields...)
}

func (l *JobLogger) log(level, msg string, fields ...Field) {
	all := append(l.base, fields...)

	wire := wireLogEntry{
		Level:     level,
		Message:   msg,
		Timestamp: time.Now().UTC(),
		Fields:    fieldsToMap(all),
	}

	data, err := json.Marshal(wire)
	if err != nil {
		return // last-resort: drop
	}

	_ = l.sink.Append(l.ctx, l.jobID, string(data))
}

/*
 ─────────────────────────────────────────────
 Child Logger
 ─────────────────────────────────────────────
*/

func (l *JobLogger) With(fields ...Field) *JobLogger {
	child := *l
	child.base = append(l.base, fields...)
	return &child
}

/*
 ─────────────────────────────────────────────
 Field Helpers
 ─────────────────────────────────────────────
*/

func Item(key string, val any) Field {
	return Field{key, val}
}

func Duration(key string, val time.Duration) Field {
	return Field{key, val.Milliseconds()}
}

func String(key string, val string) Field {
	return Field{key, val}
}

func Time(key string, val time.Time) Field {
	return Field{key, val.UTC()}
}

/*
 ─────────────────────────────────────────────
 Internals
 ─────────────────────────────────────────────
*/

func fieldsToMap(fields []Field) map[string]any {
	m := make(map[string]any, len(fields))
	for _, f := range fields {
		m[f.Key] = f.Value
	}
	return m
}