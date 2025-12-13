package entity

import "encoding/json"

type WithMetadata[T any] struct {
	Metadata json.RawMessage `json:"metadata" gorm:"type:jsonb"`
}

func (e WithMetadata[T]) GetMetadata() (*T, error) {
	if e.Metadata == nil {
		return new(T), nil
	}

	var metadata *T
	if err := json.Unmarshal(e.Metadata, &metadata); err != nil {
		return nil, err
	}

	return metadata, nil
}

func (e *WithMetadata[T]) SetMetadata(metadata *T) error {
	if metadata == nil {
		e.Metadata = nil
		return nil
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	e.Metadata = data
	return nil
}
