package tbot

type Router interface {
	Handle(m *Message)
}

type TypedRouter struct {
	onNewChatMembers handlerFunc
}

func (s *TypedRouter) Handle(m *Message) {
	if len(m.NewChatMembers) != 0 && s.onNewChatMembers != nil {
		s.onNewChatMembers(m)
		return
	}
}
