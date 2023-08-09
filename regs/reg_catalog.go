package regs

import (
	"github.com/ArthurHlt/emitter"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"github.com/orange-cloudfoundry/gsloc/observe"
)

type RegCatalogHandler interface {
	SetCatalogEntry(entry *entries.Entry)
	RemoveCatalogEntry(entry *entries.Entry)
}

var DefaultRegCatalog = newRegCatalog()

type RegCatalog struct {
	handlers []RegCatalogHandler
}

func newRegCatalog() *RegCatalog {
	rc := &RegCatalog{}
	observe.OnCatalogEntries(observe.EventTypeSet, rc)
	observe.OnCatalogEntries(observe.EventTypeDelete, rc)
	return rc
}

func (r *RegCatalog) Register(handler RegCatalogHandler) {
	r.handlers = append(r.handlers, handler)
}

func (r *RegCatalog) Observe(of *emitter.EventOf[*entries.Entry]) {
	et := observe.GetEventType(of)
	entry := of.TypedSubject()
	for _, handler := range r.handlers {
		if et == observe.EventTypeSet {
			handler.SetCatalogEntry(entry)
		} else {
			handler.RemoveCatalogEntry(entry)
		}
	}
}
