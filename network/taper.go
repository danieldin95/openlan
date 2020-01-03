package network

import (
	"fmt"
	"sync"
)

type Taper interface {
	IsTUN() bool
	IsTAP() bool
	Name() string
	Read(p []byte) (n int, err error)
	InRead(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	OutWrite() ([]byte, error)
	Close() error
	Slave(br Bridger)
	Up()
}

type tapers struct {
	lock    sync.RWMutex
	index   int
	devices map[string]Taper
}

func (t *tapers) GenName() string {
	t.lock.Lock()
	defer t.lock.Unlock()

	t.index++
	return fmt.Sprintf("vir%d", t.index)
}

func (t *tapers) Add(tap Taper) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.devices == nil {
		t.devices = make(map[string]Taper, 1024)
	}
	t.devices[tap.Name()] = tap
}

func (t *tapers) Get(name string) Taper {
	t.lock.RLock()
	defer t.lock.RUnlock()

	if t.devices == nil {
		return nil
	}
	if t, ok := t.devices[name]; ok {
		return t
	}
	return nil
}

func (t *tapers) Del(name string) {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.devices == nil {
		return
	}
	if _, ok := t.devices[name]; ok {
		delete(t.devices, name)
	}
}

var Tapers = &tapers{}