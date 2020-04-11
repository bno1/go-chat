package main

// Init message
type InitMessage struct {
	Username string `json:"username"`
}

// Incoming message object
type IncomingMessage struct {
	Message string `json:"message"`
}

// Outgoing message object
type OutgoingMessage struct {
	Username string `json:"username"`
	Message  string `json:"message"`
}

// Error message
type ErrorMessage struct {
	Error string `json:"error"`
}

// Status mesage
type StatusMessage struct {
	Status    string `json:"status"`
	UserCount uint32 `json:"user_count"`
}
