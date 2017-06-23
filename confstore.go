package confsyncer

// ConfEventType discriminates config update event types (PUT and DELETE)
type ConfEventType int

// Conf update event types
const (
	ConfPut ConfEventType = iota
	ConfDelete
)

// ConfEvent describes a conf change event
type ConfEvent struct {
	Key     string
	Content []byte
	Type    ConfEventType
}

// ConfItem abstracts a config store item
type ConfItem struct {
	Key     string
	Content []byte
}

// ConfStore abstracts a config store (etcd, local disk...)
type ConfStore interface {
	List() (data []ConfItem, err error)
	Close() error
	Updates() <-chan ConfEvent
	Put(key string, value []byte) error
	Delete(key string) error
	SkipNext(key string)
}
