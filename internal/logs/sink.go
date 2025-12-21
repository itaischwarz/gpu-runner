package logs

import (
	"context"
)


type LogSink interface {
	Append(ctx context.Context, jobID string, messsage string) error
	Stream(ctx context.Context, jobID string, from string ) (<-chan string, error)

}