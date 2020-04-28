package internal

type Config struct {
	// Max websocket message length
	MaxSocketMessageLen uint64 `section:"Messages"`

	// Max number of messages per second a user can send
	MaxMessagesPerSec float64 `section:"Messages"`

	// Max number of messages a user can send faster than MaxMessagesPerSec
	// before MaxMessagesPerSec kicks in.
	MaxMessagesBurst uint `section:"Messages"`

	// How many history messages should be send to new users. The maximum
	// effective value is the same as MAX_MESSAGEBUFFER_SIZE, and values larger
	// than that have no effect.
	BacklogLength uint `section:"Messages"`

	// Min length of a username
	MinUsernameLength uint `section:"Users"`

	// Max length of a username
	MaxUsernameLength uint `section:"Users"`
}
