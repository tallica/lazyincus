package gui

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/jesseduffield/gocui"

	"github.com/tallica/lazyincus/pkg/tasks"
)

// mainTask is one main-panel task as its writes to the view know it.
type mainTask struct {
	generation uint64
	writes     atomic.Uint64
	// Set before the task's first write, read by the main loop after it.
	autoscroll, wrap bool
}

type mainTaskKey struct{}

// mainViewState is the main loop's record of which task owns the main
// view and the last of its writes applied.
type mainViewState struct {
	generation uint64
	write      uint64
}

// QueueTask makes f the main panel's task. Main loop only: the generation
// it hands out is what turns away the writes of the task it replaces.
func (gui *Gui) QueueTask(f func(ctx context.Context)) error {
	gui.mainView.generation++
	gui.mainView.write = 0

	task := &mainTask{generation: gui.mainView.generation}

	return gui.taskManager.NewTask(func(ctx context.Context) {
		f(context.WithValue(ctx, mainTaskKey{}, task))
	})
}

// writeMain changes the main view from a task. gocui applies updates in no
// particular order, so each write is numbered: one from a task that has
// since been replaced, or older than one already applied, is dropped - a
// ticker's clear can't land after the content that follows it, nor the
// last tick of the previous tab over the next one's. A task's first write
// to land sets the view up for it.
func (gui *Gui) writeMain(ctx context.Context, write func(view *gocui.View)) {
	task, ok := ctx.Value(mainTaskKey{}).(*mainTask)
	if !ok {
		return
	}

	number := task.writes.Add(1)

	gui.g.Update(func(*gocui.Gui) error {
		if task.generation != gui.mainView.generation || number < gui.mainView.write {
			return nil
		}

		view := gui.Views.Main

		if gui.mainView.write == 0 {
			view.Autoscroll = task.autoscroll
			view.Wrap = task.wrap
		}

		gui.mainView.write = number
		write(view)

		return nil
	})
}

// renderMain sets the main view's content from the top.
func (gui *Gui) renderMain(ctx context.Context, content string) {
	gui.writeMain(ctx, func(view *gocui.View) {
		_ = view.SetOrigin(0, 0)
		_ = view.SetCursor(0, 0)
		_ = gui.setViewContent(view, content)
	})
}

// reRenderMain sets the main view's content where it's scrolled to, for a
// ticker redrawing the same thing.
func (gui *Gui) reRenderMain(ctx context.Context, content string) {
	gui.writeMain(ctx, func(view *gocui.View) {
		_ = gui.setViewContent(view, content)
	})
}

func (gui *Gui) clearMain(ctx context.Context) {
	gui.writeMain(ctx, func(view *gocui.View) {
		view.Clear()
		_ = view.SetOrigin(0, 0)
		_ = view.SetCursor(0, 0)
	})
}

type RenderStringTaskOpts struct {
	Autoscroll    bool
	Wrap          bool
	GetStrContent func() string
}

type TaskOpts struct {
	Autoscroll bool
	Wrap       bool
	Func       func(ctx context.Context)
}

type TickerTaskOpts struct {
	Duration   time.Duration
	Before     func(ctx context.Context)
	Func       func(ctx context.Context, notifyStopped chan struct{})
	Autoscroll bool
	Wrap       bool
}

func (gui *Gui) NewRenderStringTask(opts RenderStringTaskOpts) tasks.TaskFunc {
	taskOpts := TaskOpts{
		Autoscroll: opts.Autoscroll,
		Wrap:       opts.Wrap,
		Func: func(ctx context.Context) {
			gui.renderMain(ctx, opts.GetStrContent())
		},
	}

	return gui.NewTask(taskOpts)
}

// assumes it's cheap to obtain the content (otherwise we would pass a function that returns the content)
func (gui *Gui) NewSimpleRenderStringTask(getContent func() string) tasks.TaskFunc {
	return gui.NewRenderStringTask(RenderStringTaskOpts{
		GetStrContent: getContent,
		Autoscroll:    false,
		Wrap:          gui.Config.UserConfig.Gui.WrapMainPanel,
	})
}

func (gui *Gui) NewTask(opts TaskOpts) tasks.TaskFunc {
	return func(ctx context.Context) {
		if task, ok := ctx.Value(mainTaskKey{}).(*mainTask); ok {
			task.autoscroll = opts.Autoscroll
			task.wrap = opts.Wrap
		}

		opts.Func(ctx)
	}
}

// NewTickerTask is a convenience function for making a new task that repeats some action once per e.g. second
func (gui *Gui) NewTickerTask(opts TickerTaskOpts) tasks.TaskFunc {
	notifyStopped := make(chan struct{}, 10)

	task := func(ctx context.Context) {
		if opts.Before != nil {
			opts.Before(ctx)
		}
		tickChan := time.NewTicker(opts.Duration)
		defer tickChan.Stop()
		opts.Func(ctx, notifyStopped)
		for {
			select {
			case <-notifyStopped:
				gui.Log.Info("exiting ticker task due to notifyStopped channel")
				return
			case <-ctx.Done():
				gui.Log.Info("exiting ticker task due to stopped channel")
				return
			case <-tickChan.C:
				gui.Log.Info("running ticker task again")
				opts.Func(ctx, notifyStopped)
			}
		}
	}

	taskOpts := TaskOpts{
		Autoscroll: opts.Autoscroll,
		Wrap:       opts.Wrap,
		Func:       task,
	}

	return gui.NewTask(taskOpts)
}
