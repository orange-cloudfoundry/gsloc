package regs

import (
	"github.com/ArthurHlt/emitter"
	"github.com/orange-cloudfoundry/gsloc/observe"
)

type RegMemberHandler interface {
	DisableEntryIp(fqdn, ip string)
	EnableEntryIp(fqdn, ip string)
}

var DefaultRegMember = newRegMember()

type RegMember struct {
	handlers []RegMemberHandler
}

func newRegMember() *RegMember {
	rm := &RegMember{}
	observe.OnMembers(observe.EventTypeSet, rm)
	return rm
}

func (r *RegMember) Register(handler RegMemberHandler) {
	r.handlers = append(r.handlers, handler)
}

func (r *RegMember) Observe(of *emitter.EventOf[*observe.MemberFqdn]) {
	memberFqdn := of.TypedSubject()
	for _, handler := range r.handlers {
		if memberFqdn.Member.GetDisabled() {
			handler.DisableEntryIp(memberFqdn.Fqdn, memberFqdn.Member.Ip)
		} else {
			handler.EnableEntryIp(memberFqdn.Fqdn, memberFqdn.Member.Ip)
		}
	}
}
