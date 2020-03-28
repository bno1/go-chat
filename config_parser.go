package main

import (
	"fmt"
	"gopkg.in/ini.v1"
	"reflect"
)

// Parse an INI file and construct a config. Initialize the config with values
// from defaultConfig.
func ReadConfig(iniPath string, defaultConfig *Config) (Config, error) {
	config := *defaultConfig

	// Open file
	configIni, err := ini.Load(iniPath)
	if err != nil {
		return config, fmt.Errorf("Failed to open ini file: %v, %w", iniPath,
			err)
	}

	configType := reflect.TypeOf(config)
	configValue := reflect.ValueOf(&config).Elem()

	for i := 0; i < configType.NumField(); i++ {
		field := configType.Field(i)
		value := configValue.Field(i)

		sectionName := field.Tag.Get("section")
		if len(sectionName) == 0 {
			continue
		}

		section, err := configIni.GetSection(sectionName)
		if err != nil {
			continue
		}

		key, err := section.GetKey(field.Name)
		if err != nil {
			continue
		}

		new_value := reflect.ValueOf(struct{}{})
		switch field.Type.Kind() {
		case reflect.Uint:
			new_value = reflect.ValueOf(key.MustUint(uint(value.Uint())))
		case reflect.Uint64:
			new_value = reflect.ValueOf(key.MustUint64(value.Uint()))
		case reflect.Int:
			new_value = reflect.ValueOf(key.MustInt(int(value.Int())))
		case reflect.Int64:
			new_value = reflect.ValueOf(key.MustInt64(value.Int()))
		case reflect.Bool:
			new_value = reflect.ValueOf(key.MustBool(value.Bool()))
		case reflect.Float32:
			new_value = reflect.ValueOf(float32(key.MustFloat64(value.Float())))
		case reflect.Float64:
			new_value = reflect.ValueOf(key.MustFloat64(value.Float()))
		case reflect.String:
			new_value = reflect.ValueOf(key.String())
		default:
			return config, fmt.Errorf("Parser for type %s not implemented",
				field.Type.Kind().String())
		}

		value.Set(new_value)
	}

	return config, nil
}
