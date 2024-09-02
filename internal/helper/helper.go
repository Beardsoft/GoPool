package helper

import (
	"encoding/json"
	"fmt"
)

// mapToStruct maps a map[string]interface{} to a struct using JSON marshaling and unmarshaling.
func MapToStruct(m map[string]interface{}, s interface{}) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal map: %w", err)
	}

	err = json.Unmarshal(data, s)
	if err != nil {
		return fmt.Errorf("failed to unmarshal into struct: %w", err)
	}

	return nil
}
