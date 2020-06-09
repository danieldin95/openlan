package storage

import (
	"github.com/danieldin95/openlan-go/libol"
	"github.com/danieldin95/openlan-go/models"
)

type _neighbor struct {
	Neighbors *libol.SafeStrMap
	Listen    Listen
}

var Neighbor = _neighbor{
	Neighbors: libol.NewSafeStrMap(1024),
	Listen: Listen{
		listener: libol.NewSafeStrMap(32),
	},
}

func (p *_neighbor) Init(size int) {
	p.Neighbors = libol.NewSafeStrMap(size)
}

func (p *_neighbor) Add(m *models.Neighbor) {
	if v := p.Neighbors.Get(m.IpAddr.String()); v != nil {
		p.Neighbors.Del(m.IpAddr.String())
	} else {
		_ = p.Listen.AddV(m.IpAddr.String(), m)
	}
	_ = p.Neighbors.Set(m.IpAddr.String(), m)
}

func (p *_neighbor) Update(m *models.Neighbor) *models.Neighbor {
	if v := p.Neighbors.Get(m.IpAddr.String()); v != nil {
		n := v.(*models.Neighbor)
		n.HwAddr = m.HwAddr
		n.HitTime = m.HitTime
	}
	return nil
}

func (p *_neighbor) Get(key string) *models.Neighbor {
	if v := p.Neighbors.Get(key); v != nil {
		return v.(*models.Neighbor)
	}
	return nil
}

func (p *_neighbor) Del(key string) {
	p.Neighbors.Del(key)
	p.Listen.DelV(key)
}

func (p *_neighbor) List() <-chan *models.Neighbor {
	c := make(chan *models.Neighbor, 128)

	go func() {
		p.Neighbors.Iter(func(k string, v interface{}) {
			c <- v.(*models.Neighbor)
		})
		c <- nil //Finish channel by nil.
	}()

	return c
}
