package main

import (
	"reflect"
	"sync/atomic"
)

type Config struct {
	MaxSocketMessageLen uint64
	MaxMessagesPerSec   float64
	MaxMessagesBurst    uint32
}

type TaggedConfig struct {
	tag    uint64
	config Config
}

type ConfigStore struct {
	tconfig atomic.Value
}

type ConfigHandle struct {
	configTag uint64
	store     *ConfigStore
}

func NewConfigStore(config Config) ConfigStore {
	store := ConfigStore{}
	tconfig := TaggedConfig{
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
	changed := tconfig.tag != handle.configTag

	if changed {
		handle.configTag = tconfig.tag
	}

	return &tconfig.config, changed
}
