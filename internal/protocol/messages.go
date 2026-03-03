package protocol

// Protocol message type constants
const (
	TypeMatched          = "matched"
	TypeMessage          = "message"
	TypeStrangerLeft     = "stranger_left"
	TypeWaiting          = "waiting"
	TypeError            = "error"
	TypeTyping           = "typing"
	TypeSkip             = "skip"
	TypeConnectionClosed = "connection_closed"
)

// Client → Server

type MessageOut struct {
	Type string `json:"type"` // "message"
	Text string `json:"text"`
}

type SkipOut struct {
	Type string `json:"type"` // "skip"
}

type TypingOut struct {
	Type  string `json:"type"`  // "typing"
	State string `json:"state"` // "typing" | "stopped"
}

// Server → Client

type ServerMsg struct {
	Type      string           `json:"type"`
	RoomID    string           `json:"room_id,omitempty"`
	Stranger  *StrangerProfile `json:"stranger,omitempty"`
	From      string           `json:"from,omitempty"`
	Text      string           `json:"text,omitempty"`
	Timestamp string           `json:"timestamp,omitempty"`
	Message   string           `json:"message,omitempty"`
	State     string           `json:"state,omitempty"`
}

type StrangerProfile struct {
	Username        string `json:"username"`
	AvatarURL       string `json:"avatar_url"`
	Bio             string `json:"bio"`
	PublicRepos     int    `json:"public_repos"`
	GithubCreatedAt string `json:"github_created_at"`
	TopLanguages    string `json:"top_languages"`
	TopRepo         string `json:"top_repo"`
	TopRepoStars    int    `json:"top_repo_stars"`
	Contributions     int    `json:"contributions"`
	ContributionGraph string `json:"contribution_graph"`
}
