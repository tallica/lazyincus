package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/sasha-s/go-deadlock"
	"github.com/sirupsen/logrus"
	"github.com/tallica/lazyincus/pkg/i18n"
)

type TaskManager struct {
	currentTask  *Task
	waitingMutex deadlock.Mutex
	taskIDMutex  deadlock.Mutex
	Log          *logrus.Entry
	Tr           *i18n.TranslationSet
	newTaskId    int
}

type Task struct {
	ctx           context.Context
	cancel        context.CancelFunc
	stopped       bool
	stopMutex     deadlock.Mutex
	notifyStopped chan struct{}
	Log           *logrus.Entry
	f             func(ctx context.Context)
}

type TaskFunc func(ctx context.Context)

func NewTaskManager(log *logrus.Entry, translationSet *i18n.TranslationSet) *TaskManager {
	return &TaskManager{Log: log, Tr: translationSet}
}

// Close closes the task manager, killing whatever task may currently be running
func (t *TaskManager) Close() {
	if t.currentTask == nil {
		return
	}

	c := make(chan struct{}, 1)

	go func() {
		t.currentTask.Stop()
		c <- struct{}{}
	}()

	select {
	case <-c:
		return
	case <-time.After(3 * time.Second):
		fmt.Println(t.Tr.CannotKillChildError)
	}
}

func (t *TaskManager) NewTask(f func(ctx context.Context)) error {
	go func() {
		t.taskIDMutex.Lock()
		t.newTaskId++
		taskID := t.newTaskId
		t.taskIDMutex.Unlock()

		t.waitingMutex.Lock()
		defer t.waitingMutex.Unlock()
		t.taskIDMutex.Lock()
		if taskID < t.newTaskId {
			t.taskIDMutex.Unlock()
			return
		}
		t.taskIDMutex.Unlock()

		ctx, cancel := context.WithCancel(context.Background())
		notifyStopped := make(chan struct{})

		if t.currentTask != nil {
			t.Log.Info("asking task to stop")
			t.currentTask.Stop()
			t.Log.Info("task stopped")
		}

		t.currentTask = &Task{
			ctx:           ctx,
			cancel:        cancel,
			notifyStopped: notifyStopped,
			Log:           t.Log,
			f:             f,
		}

		go func() {
			f(ctx)
			t.Log.Info("returned from function, closing notifyStopped")
			close(notifyStopped)
		}()
	}()

	return nil
}

func (t *Task) Stop() {
	t.stopMutex.Lock()
	defer t.stopMutex.Unlock()
	if t.stopped {
		return
	}

	t.cancel()
	t.Log.Info("closed stop channel, waiting for notifyStopped message")
	<-t.notifyStopped
	t.Log.Info("received notifystopped message")
	t.stopped = true
}
