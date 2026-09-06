package app

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"forumdesk/internal/telegram"
)

type fakePoller struct {
	calls   int
	offsets []int
	updates []telegram.Update
	err     error
	cancel  context.CancelFunc
}

func (f *fakePoller) GetUpdates(_ context.Context, offset int) ([]telegram.Update, error) {
	f.calls++
	f.offsets = append(f.offsets, offset)
	if f.err != nil {
		err := f.err
		f.err = nil
		f.cancel()
		return nil, err
	}
	if f.calls == 1 {
		return f.updates, nil
	}
	f.cancel()
	return nil, context.Canceled
}

type fakeHandler struct {
	ids []int
	err error
}

type retryPoller struct {
	calls  int
	cancel context.CancelFunc
}

func (p *retryPoller) GetUpdates(_ context.Context, _ int) ([]telegram.Update, error) {
	p.calls++
	if p.calls == 1 {
		return nil, errors.New("temporary network error")
	}
	p.cancel()
	return nil, context.Canceled
}

func (f *fakeHandler) Handle(_ context.Context, u telegram.Update) error {
	f.ids = append(f.ids, u.UpdateID)
	return f.err
}

func TestRunProcessesBatchAndAdvancesOffset(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &fakePoller{updates: []telegram.Update{{UpdateID: 7}, {UpdateID: 8}}, cancel: cancel}
	h := &fakeHandler{}
	Run(ctx, log.New(&bytes.Buffer{}, "", 0), p, h)
	if len(h.ids) != 2 || h.ids[0] != 7 || h.ids[1] != 8 {
		t.Fatalf("handled=%v", h.ids)
	}
	if len(p.offsets) != 2 || p.offsets[1] != 9 {
		t.Fatalf("offsets=%v", p.offsets)
	}
}

func TestRunLogsHandlerError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &fakePoller{updates: []telegram.Update{{UpdateID: 3}}, cancel: cancel}
	h := &fakeHandler{err: errors.New("copy failed")}
	var output bytes.Buffer
	Run(ctx, log.New(&output, "", 0), p, h)
	if !strings.Contains(output.String(), "update 3: copy failed") {
		t.Fatalf("log=%q", output.String())
	}
}

func TestRunStopsOnCanceledPollingError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &fakePoller{err: errors.New("network"), cancel: cancel}
	Run(ctx, log.New(&bytes.Buffer{}, "", 0), p, &fakeHandler{})
	if p.calls != 0 {
		t.Fatalf("calls=%d", p.calls)
	}
}

func TestRunRetriesAndLogsTemporaryPollingError(t *testing.T) {
	oldDelay := retryDelay
	retryDelay = 0
	t.Cleanup(func() { retryDelay = oldDelay })
	ctx, cancel := context.WithCancel(context.Background())
	p := &retryPoller{cancel: cancel}
	var output bytes.Buffer
	Run(ctx, log.New(&output, "", 0), p, &fakeHandler{})
	if p.calls != 2 || !strings.Contains(output.String(), "temporary network error") {
		t.Fatalf("calls=%d log=%q", p.calls, output.String())
	}
}
