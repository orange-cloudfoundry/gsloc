package regs

import (
	"github.com/ArthurHlt/emitter"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"github.com/orange-cloudfoundry/gsloc/observe"
)

type RegKVHandler interface {
	SetKVEntry(entry *entries.SignedEntry)
	RemoveKvEntry(entry *entries.SignedEntry)
}

var DefaultRegKV = newRegKV()

type RegKV struct {
	handlers []RegKVHandler
}

func newRegKV() *RegKV {
	rk := &RegKV{}
	observe.OnKvEntries(observe.EventTypeSet, rk)
	observe.OnKvEntries(observe.EventTypeDelete, rk)
	return rk
}

func (r *RegKV) Register(handler RegKVHandler) {
	r.handlers = append(r.handlers, handler)
}

func (r *RegKV) Observe(of *emitter.EventOf[*entries.SignedEntry]) {
	et := observe.GetEventType(of)
	entry := of.TypedSubject()
	for _, handler := range r.handlers {
		if et == observe.EventTypeSet {
			handler.SetKVEntry(entry)
		} else {
			handler.RemoveKvEntry(entry)
		}
	}
}
