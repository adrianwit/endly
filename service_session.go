package endly

import (
	"github.com/viant/toolbox/ssh"
)

//SystemTerminalSession represents a system terminal session
type SystemTerminalSession struct {
	ID string
	*ssh.MultiCommandSession
	Connection      *ssh.Client
	OperatingSystem *OperatingSystem
	envVariables    map[string]string
	path            string
}

//NewSystemTerminalSession create a new client session
func NewSystemTerminalSession(id string, connection *ssh.Client) (*SystemTerminalSession, error) {
	return &SystemTerminalSession{
		ID:           id,
		Connection:   connection,
		envVariables: make(map[string]string),
	}, nil
}

//SystemTerminalSessions represents a map of client sessions keyed by session id
type SystemTerminalSessions map[string]*SystemTerminalSession

//Has checks if client session exists for provided id.
func (s *SystemTerminalSessions) Has(id string) bool {
	_, has := (*s)[id]
	return has
}

var clientSessionKey = (*SystemTerminalSessions)(nil)