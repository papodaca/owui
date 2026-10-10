package tui

import "time"

// genJob is one reply the server is still writing. Keyed by the assistant
// message id, which exists before a new chat has a server id. Several chats
// can have a job at once. The server tracks tasks per chat, not per client.
type genJob struct {
	chatID    string
	assistant string
	taskIDs   []string
	started   time.Time
	seq       int
	token     int
	chime     bool
	// adopted is a reply started elsewhere and picked up when its chat opened.
	adopted bool
	// lastEvent is when the socket last sent anything for this reply.
	lastEvent time.Time
}

func (m *App) ensureJobs() {
	if m.jobs == nil {
		m.jobs = map[string]*genJob{}
	}
}

func (m App) jobByChat(id string) *genJob {
	if id == "" {
		return nil
	}
	for _, j := range m.jobs {
		if j.chatID == id {
			return j
		}
	}
	return nil
}

// openJob is the reply for the chat on screen. A new chat has no server id
// yet, so it matches the draft started from this composer.
func (m App) openJob() *genJob {
	if m.chatID != "" {
		return m.jobByChat(m.chatID)
	}
	for _, j := range m.jobs {
		if j.chatID == "" && j.token == m.viewToken {
			return j
		}
	}
	return nil
}

func (m *App) dropJob(assistant string) {
	if j := m.jobs[assistant]; j != nil {
		j.seq++
	}
	delete(m.jobs, assistant)
}
