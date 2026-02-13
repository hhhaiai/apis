package statepersist

import "errors"

var ErrNotFound = errors.New("persisted state not found")

type Backend interface {
	Load(key string, out any) error
	Save(key string, value any) error
}
