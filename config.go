package main

import (
	"reflect"
	"sync/atomic"
)

type Config struct {
	// Max websocket message length
	MaxSocketMessageLen uint64 `section:"Messages"`

	// Max number of messages per second a user can send
	MaxMessagesPerSec float64 `section:"Messages"`

	// Max number of messages a user can send faster than MaxMessagesPerSec
	// before MaxMessagesPerSec kicks in.
	MaxMessagesBurst uint `section:"Messages"`

	// Min length of a username
	MinUsernameLength uint `section:"Users"`

	// Max length of a username
	MaxUsernameLength uint `section:"Users"`
}

// Each config has a tag (opaque numerical value). ConfigHandle stores the tag
// of the last loaded config and it uses it to detect if the main config
// changed.
type TaggedConfig struct {
	tag    uint64
	config Config
}

type ConfigStore struct {
	tconfig atomic.Value // stores a TaggedConfig
}

type ConfigHandle struct {
	configTag uint64 // tag of the last loaded config
	store     *ConfigStore
}

func NewConfigStore(config Config) ConfigStore {
	store := ConfigStore{}
	tconfig := TaggedConfig{
		// Initialize tag with 1 so ConfigHandle (initialized with tag 0) are
		// notified of the initial configuration.
		tag:    1,
		config: config,
	}

	store.tconfig.Store(&tconfig)

	return store
}

func (store *ConfigStore) getTaggedConfig() *TaggedConfig {
	return store.tconfig.Load().(*TaggedConfig)
}

func (store *ConfigStore) UpdateConfig(config Config) {
	tconfig := store.getTaggedConfig()

	// If configuration is different then save it and update the tag.
	// Otherwise ignore it.
	if !reflect.DeepEqual(tconfig.config, config) {
		new_tconfig := TaggedConfig{
			tag:    tconfig.tag + 1,
			config: config,
		}

		store.tconfig.Store(&new_tconfig)
	}
}

func (store *ConfigStore) GetHandle() ConfigHandle {
	handle := ConfigHandle{
		configTag: 0,
		store:     store,
	}

	return handle
}

func (handle *ConfigHandle) GetConfig() (*Config, bool) {
	tconfig := handle.store.getTaggedConfig()

	// Config is changed if tags differ
	changed := tconfig.tag != handle.configTag

	if changed {
		// Save the new tag
		handle.configTag = tconfig.tag
	}

	return &tconfig.config, changed
}
