package cacher

import (
	"time"

	"github.com/fresh8/go-cache/engine/common"
	"github.com/fresh8/go-cache/joque"
)

type cacher struct {
	engine   common.Engine
	jobQueue chan joque.Job
}

//go:generate moq -out ../mock/cacher_mock.go . Cacher
//go:generate goimports -w ../mock/cacher_mock.go

// Cacher defines the interface for a caching system so it can be customised.
type Cacher interface {
	Get(string, time.Time, func() ([]byte, error)) func() ([]byte, error)
	Expire(string) error
}

// NewCacher creates a new generic cacher with the given engine.
func NewCacher(engine common.Engine, maxQueueSize int, maxWorkers int) Cacher {
	return cacher{
		engine:   engine,
		jobQueue: joque.Setup(maxQueueSize, maxWorkers),
	}
}

func (c cacher) get(key string, expires time.Time, regenerate func() ([]byte, error)) (data []byte, err error) {
	if c.engine.Exists(key) {
		data, err = c.engine.Get(key)

		// Return, something went wrong
		if err != nil {
			return
		}

		// Return, data is fresh enough
		if !c.engine.IsExpired(key) {
			return
		}

		// Return, as data is being regenerated by another process
		if c.engine.IsLocked(key) {
			return
		}

		// Send the regenerate function to the job queue to be processed
		c.jobQueue <- func() {
			c.engine.Lock(key)
			defer c.engine.Unlock(key)

			data, err = regenerate()
			c.engine.Put(key, data, expires)
		}
		return
	}

	// Return, as data is being regenerated by another process
	if c.engine.IsLocked(key) {
		return
	}

	// Lock on initial generation so that things
	c.engine.Lock(key)
	defer c.engine.Unlock(key)

	// If the key doesn't exist, generate it now and return
	data, err = regenerate()
	if err != nil {
		return
	}

	err = c.engine.Put(key, data, expires)

	return
}

func (c cacher) Get(key string, expires time.Time, regenerate func() ([]byte, error)) func() ([]byte, error) {
	var data []byte
	var err error

	ch := make(chan struct{}, 1)
	go func() {
		defer close(ch)
		data, err = c.get(key, expires, regenerate)
	}()

	return func() ([]byte, error) {
		<-ch
		return data, err
	}
}

// Expire the given key within the cache engine
func (c cacher) Expire(key string) error {
	return c.engine.Expire(key)
}
