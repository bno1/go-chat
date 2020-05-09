package internal

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	ANY_MESSAGE         = ""
	INIT_MESSAGE        = "init"
	SEND_MESSAGE        = "send"
	RECV_MESSAGE        = "recv"
	ERROR_MESSAGE       = "error"
	HELLO_MESSAGE       = "hello"
	USER_CHANGE_MESSAGE = "user_change"
)

type Frame struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

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
	// Unix Timestmap
	Timestmap uint64 `json:"timestamp"`
	Username  string `json:"username"`
	Message   string `json:"message"`
}

// Error message
type ErrorMessage struct {
	Error string `json:"error"`
}

// Hello message
type HelloMessage struct {
	UserCount uint32 `json:"user_count"`
}

// User change
type UserChangeMessage struct {
	// Unix Timestmap
	Timestmap uint64 `json:"timestamp"`
	Username  string `json:"username"`
	Action    string `json:"action"`

	UserCount uint32   `json:"user_count"`
	UserList  []string `json:"user_list"`
}

type EncodedMessage struct {
	originalMsg interface{}
	msg         []byte
}

func ParseMessage(
	data []byte,
	expected string,
) (msgType string, msg interface{}, err error) {
	var frame Frame

	msg = nil

	err = json.Unmarshal(data, &frame)
	if err != nil {
		return
	}

	msgType = frame.Type

	if expected != ANY_MESSAGE && frame.Type != expected {
		err = fmt.Errorf(
			"Expected message of type \"%s\" but got a message of type \"%s\"",
			expected, frame.Type)
		return
	}

	var m interface{}

	switch frame.Type {
	case INIT_MESSAGE:
		m = new(InitMessage)

	case SEND_MESSAGE:
		m = new(IncomingMessage)

	case RECV_MESSAGE:
		m = new(OutgoingMessage)

	case ERROR_MESSAGE:
		m = new(ErrorMessage)

	case HELLO_MESSAGE:
		m = new(HelloMessage)

	case USER_CHANGE_MESSAGE:
		m = new(UserChangeMessage)

	default:
		err = fmt.Errorf("Unknown message type \"%s\"", frame.Type)
		return
	}

	err = json.Unmarshal(frame.Message, m)
	if err != nil {
		return
	}

	msg = m

	return
}

func NewOutgoingMessage(
	username string,
	message string,
) (EncodedMessage, error) {
	var frame Frame
	var emsg EncodedMessage
	var err error

	msg := OutgoingMessage{
		Timestmap: uint64(time.Now().Unix()),
		Username:  username,
		Message:   message,
	}

	frame.Type = RECV_MESSAGE
	frame.Message, err = json.Marshal(&msg)
	if err != nil {
		return emsg, err
	}

	emsg.originalMsg = &msg
	emsg.msg, err = json.Marshal(&frame)

	return emsg, err
}

func NewHelloMessage(
	userCount uint32,
) (EncodedMessage, error) {
	var frame Frame
	var emsg EncodedMessage
	var err error

	msg := HelloMessage{
		UserCount: userCount,
	}

	frame.Type = HELLO_MESSAGE
	frame.Message, err = json.Marshal(&msg)
	if err != nil {
		return emsg, err
	}

	emsg.originalMsg = &msg
	emsg.msg, err = json.Marshal(&frame)

	return emsg, err
}

func NewUserChangeMessage(
	username string,
	action string,
	userCount uint32,
	userList []string,
) (EncodedMessage, error) {
	var frame Frame
	var emsg EncodedMessage
	var err error

	msg := UserChangeMessage{
		Timestmap: uint64(time.Now().Unix()),
		Username:  username,
		Action:    action,
		UserCount: userCount,
		UserList:  userList,
	}

	frame.Type = USER_CHANGE_MESSAGE
	frame.Message, err = json.Marshal(&msg)
	if err != nil {
		return emsg, err
	}

	emsg.originalMsg = &msg
	emsg.msg, err = json.Marshal(&frame)

	return emsg, err
}

func NewErrorMessage(
	message string,
) (EncodedMessage, error) {
	var frame Frame
	var emsg EncodedMessage
	var err error

	msg := ErrorMessage{
		Error: message,
	}

	frame.Type = ERROR_MESSAGE
	frame.Message, err = json.Marshal(&msg)
	if err != nil {
		return emsg, err
	}

	emsg.originalMsg = &msg
	emsg.msg, err = json.Marshal(&frame)

	return emsg, err
}
