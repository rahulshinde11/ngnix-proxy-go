package host

import (
	"strings"
)

// ExtrasValue represents a value that can be stored in extras
type ExtrasValue struct {
	value interface{}
}

// NewExtrasValue creates a new ExtrasValue
func NewExtrasValue(value interface{}) *ExtrasValue {
	return &ExtrasValue{value: value}
}

// Get returns the underlying value
func (e *ExtrasValue) Get() interface{} {
	return e.value
}

// Update updates the value with a new value, handling merging for maps and slices
func (e *ExtrasValue) Update(value interface{}) {
	switch v := e.value.(type) {
	case map[string]interface{}:
		if newMap, ok := value.(map[string]interface{}); ok {
			for k, val := range newMap {
				v[k] = val
			}
		}
	case []string:
		if newSlice, ok := value.([]string); ok {
			e.value = append(v, newSlice...)
		}
	case []interface{}:
		if newSlice, ok := value.([]interface{}); ok {
			e.value = append(v, newSlice...)
		}
	default:
		e.value = value
	}
}

// ExtrasMap represents a map of extras values
type ExtrasMap struct {
	values map[string]*ExtrasValue
}

// NewExtrasMap creates a new ExtrasMap
func NewExtrasMap() *ExtrasMap {
	return &ExtrasMap{
		values: make(map[string]*ExtrasValue),
	}
}

// Get returns the value for a key
func (e *ExtrasMap) Get(key string) interface{} {
	if v, ok := e.values[key]; ok {
		return v.Get()
	}
	return nil
}

// Set sets a value for a key
func (e *ExtrasMap) Set(key string, value interface{}) {
	if v, ok := e.values[key]; ok {
		v.Update(value)
	} else {
		e.values[key] = NewExtrasValue(value)
	}
}

// Update updates the extras map with new values
func (e *ExtrasMap) Update(extras map[string]string) {
	for k, v := range extras {
		switch k {
		case "websocket":
			e.values[k] = NewExtrasValue(v == "true")
		case "http":
			e.values[k] = NewExtrasValue(v == "true")
		case "scheme":
			e.values[k] = NewExtrasValue(v)
		case "container_path":
			e.values[k] = NewExtrasValue(v)
		default:
			// For injected configs, collect them into a deduplicated slice
			if strings.HasPrefix(k, "injected_") {
				// Get existing injected configs as a slice
				var existingInjected []string
				if existing := e.Get("injected"); existing != nil {
					if slice, ok := existing.([]string); ok {
						existingInjected = slice
					}
				}

				// Check if this config already exists (deduplicate like Python set)
				found := false
				for _, existing := range existingInjected {
					if existing == v {
						found = true
						break
					}
				}

				// Only add if not found (mimic Python set behavior)
				if !found {
					existingInjected = append(existingInjected, v)
				}

				e.values["injected"] = NewExtrasValue(existingInjected)
			} else {
				e.values[k] = NewExtrasValue(v)
			}
		}
	}
}

// ToMap converts the ExtrasMap to a regular map
func (e *ExtrasMap) ToMap() map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range e.values {
		result[k] = v.Get()
	}
	return result
}

// Len returns the number of extras in the map
func (e *ExtrasMap) Len() int {
	return len(e.values)
}
