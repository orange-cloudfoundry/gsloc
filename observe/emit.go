package observe

import (
	"fmt"
	"github.com/ArthurHlt/emitter"
	"github.com/orange-cloudfoundry/gsloc-go-sdk/gsloc/api/config/entries/v1"
	"strings"
	"sync"
)

const (
	EventTypeSet EventType = iota
	EventTypeDelete

	TopicKvEntries      topic = "kv_entries"
	TopicCatalogEntries topic = "catalog_entries"
	TopicMembers        topic = "members"
)

type EventType int
type topic string

type MemberFqdn struct {
	Fqdn   string
	Member *entries.Member
}

type ListSigEntries interface {
	Observe(of *emitter.EventOf[*entries.SignedEntry])
}

type ListCatalogEntries interface {
	Observe(of *emitter.EventOf[*entries.Entry])
}

type ListMembers interface {
	Observe(of *emitter.EventOf[*MemberFqdn])
}

type ListenerOf[T any] interface {
	Observe(e *emitter.EventOf[T])
}

type genericListener[T any] struct {
	realListener ListenerOf[T]
}

func (l *genericListener[T]) Observe(e emitter.Event) {
	l.realListener.Observe(e.(*emitter.EventOf[T]))
}

var e = emitter.New(1000)

var listToReal = &sync.Map{}

func OnKvEntries(et EventType, listener ListenerOf[*entries.SignedEntry], middlewares ...func(emitter.Event)) {
	on(TopicKvEntries, et, listener, middlewares...)
}

func OffKvEntries(et EventType, listener ...ListenerOf[*entries.SignedEntry]) {
	off(TopicKvEntries, et, listener...)
}

func EmitKvEntry(et EventType, entry *entries.SignedEntry) chan struct{} {
	return emit[*entries.SignedEntry](TopicKvEntries, et, entry)
}

func OnCatalogEntries(et EventType, listener ListenerOf[*entries.Entry], middlewares ...func(emitter.Event)) {
	on(TopicCatalogEntries, et, listener, middlewares...)
}

func OffCatalogEntries(et EventType, listener ...ListenerOf[*entries.Entry]) {
	off(TopicCatalogEntries, et, listener...)
}

func EmitCatalogEntry(et EventType, entry *entries.Entry) chan struct{} {
	return emit[*entries.Entry](TopicCatalogEntries, et, entry)
}

func OnMembers(et EventType, listener ListenerOf[*MemberFqdn], middlewares ...func(emitter.Event)) {
	on(TopicMembers, et, listener, middlewares...)
}

func OffMembers(et EventType, listener ...ListenerOf[*MemberFqdn]) {
	off(TopicMembers, et, listener...)
}

func EmitMember(et EventType, member *MemberFqdn) chan struct{} {
	return emit[*MemberFqdn](TopicMembers, et, member)
}

func on[T any](t topic, et EventType, listener ListenerOf[T], middlewares ...func(emitter.Event)) {
	gl := &genericListener[T]{realListener: listener}
	listToReal.Store(fmt.Sprintf("%p", listener), gl)
	e.On(makeTopic(t, et), gl, middlewares...)
}

func off[T any](t topic, et EventType, listeners ...ListenerOf[T]) {
	for _, listener := range listeners {
		gl, ok := listToReal.Load(fmt.Sprintf("%p", listener))
		if !ok {
			continue
		}
		e.Off(makeTopic(t, et), gl.(*genericListener[T]))
		listToReal.Delete(fmt.Sprintf("%p", listener))
	}
}

func emit[T any](t topic, et EventType, entry T) chan struct{} {
	return e.Emit(emitter.NewEventOf[T](makeTopic(t, et), entry))
}

func OffAll() {
	e.Off("*")
}

func Topics() []string {
	return e.Topics()
}

func GetEventType(e emitter.Event) EventType {
	splitted := strings.Split(e.Topic(), "/")
	if len(splitted) < 2 {
		return EventTypeSet
	}
	switch splitted[1] {
	case "1":
		return EventTypeDelete
	}
	return EventTypeSet
}

func makeTopic(t topic, et EventType) string {
	return fmt.Sprintf("%s/%d", t, et)
}
