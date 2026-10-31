package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// mainTaskFunc runs off the loop, writing main through w until ctx is done.
type mainTaskFunc func(ctx context.Context, w *mainWriter)

// mainTask is which task owns main, its last write applied, and its key.
type mainTask struct {
	generation uint64
	write      uint64
	key        string
}

type mainWriteKind uint8

const (
	writeRender mainWriteKind = iota
	writeRerender
	writeClear
)

// mainWriteMsg: numbered so a stale write is dropped.
type mainWriteMsg struct {
	generation, number uint64
	wrap, autoscroll   bool
	kind               mainWriteKind
	content            string
}

type mainWriter struct {
	generation       uint64
	wrap, autoscroll bool
	writes           uint64
	send             func(tea.Msg)
}

func (w *mainWriter) write(kind mainWriteKind, content string) {
	w.writes++
	w.send(mainWriteMsg{
		generation: w.generation, number: w.writes,
		wrap: w.wrap, autoscroll: w.autoscroll, kind: kind, content: content,
	})
}

func (w *mainWriter) render(content string) { w.write(writeRender, content) }

func (w *mainWriter) rerender(content string) { w.write(writeRerender, content) }

func (w *mainWriter) clear() { w.write(writeClear, "") }

func tickerTask(interval time.Duration, content func() string) mainTaskFunc {
	return tickerTaskFrom("", interval, content)
}

func tickerTaskFrom(first string, interval time.Duration, content func() string) mainTaskFunc {
	return func(ctx context.Context, w *mainWriter) {
		w.clear()

		if first != "" {
			w.rerender(first)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			w.rerender(content())

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}
}

func renderString(content func() string) mainTaskFunc {
	return func(_ context.Context, w *mainWriter) { w.render(content()) }
}

// queueMain makes task main's, unless it already is the one for key.
func (m *Model) queueMain(key string, wrap, autoscroll bool, task mainTaskFunc) {
	if key == m.mainTask.key {
		return
	}

	m.mainTask = mainTask{generation: m.mainTask.generation + 1, key: key}
	w := &mainWriter{generation: m.mainTask.generation, wrap: wrap, autoscroll: autoscroll}
	m.pendingMain = m.startTask(func(ctx context.Context, send func(tea.Msg)) {
		w.send = send
		task(ctx, w)
	})
}

// startTask hands task to pkg/tasks, which stops the last one first.
func (m *Model) startTask(task func(ctx context.Context, send func(tea.Msg))) tea.Cmd {
	if m.runTask != nil {
		return m.runTask(task)
	}

	tasks, inbox := m.tasks, m.inbox

	return func() tea.Msg {
		_ = tasks.NewTask(func(ctx context.Context) {
			task(ctx, func(msg tea.Msg) {
				select {
				case inbox <- msg:
				case <-ctx.Done():
				}
			})
		})

		return nil
	}
}

func (m *Model) onMainWrite(msg mainWriteMsg) {
	if msg.generation != m.mainTask.generation || msg.number < m.mainTask.write {
		return
	}

	if m.mainTask.write == 0 {
		m.main.begin(msg.wrap, msg.autoscroll)
	}
	m.mainTask.write = msg.number

	switch msg.kind {
	case writeRender:
		m.main.render(msg.content)
	case writeRerender:
		m.main.rerender(msg.content)
	case writeClear:
		m.main.clear()
	}
}
