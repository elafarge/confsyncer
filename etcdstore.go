package confsyncer

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	etcd3client "github.com/coreos/etcd/clientv3"
	log "github.com/sirupsen/logrus"
)

// Etcd3Store implements an etcd3-backed config store
type Etcd3Store struct {
	ConfStore

	Prefix string

	updateChan  chan ConfEvent
	client      *etcd3client.Client
	skipMap     map[string]int
	skipMapLock sync.Mutex
}

// NewEtcd3Store creates a new etcd3 config store
func NewEtcd3Store(endpoint, prefix string) (cs *Etcd3Store, err error) {
	cs = &Etcd3Store{
		Prefix: prefix,

		updateChan: make(chan ConfEvent),
		skipMap:    map[string]int{},
	}

	// Let's create the etcd client
	cs.client, err = etcd3client.New(etcd3client.Config{
		Endpoints:   []string{endpoint},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("[etcdstore] Error creating etcd client: %v", err)
	}

	// And let's watch our prefix for updates
	watchChan := cs.client.Watch(context.Background(), cs.Prefix, etcd3client.WithPrefix())
	go func() {
		for watchResp := range watchChan {
			for _, ev := range watchResp.Events {
				rel, _ := filepath.Rel(cs.Prefix, string(ev.Kv.Key))

				if ev.Type == etcd3client.EventTypeDelete {
					if cs.shouldSkip(rel) {
						log.Debugf("[etcdstore] Skipping event %q (%d skips left)", ev.Kv.Key, cs.skipMap[rel])
						continue
					}
					log.Debugf("[etcdstore] Got delete event for %s", ev.Kv.Key)
					cs.updateChan <- ConfEvent{
						Type: ConfDelete,
						Key:  rel,
					}
				} else if ev.Type == etcd3client.EventTypePut {
					if cs.shouldSkip(rel) {
						log.Debugf("[etcdstore] Skipping event %q (%d skips left)", ev.Kv.Key, cs.skipMap[rel])
						continue
					}

					log.Debugf("[etcdstore] Got update event for %q", ev.Kv.Key)
					cs.updateChan <- ConfEvent{
						Type:    ConfPut,
						Key:     rel,
						Content: ev.Kv.Value,
					}
				}
			}
		}
	}()
	return
}

// List all conf from Etcd3
func (cs *Etcd3Store) List() (data []ConfItem, err error) {
	// Pull files if key prefix exists, create key otherwise
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	resp, err := cs.client.Get(ctx, cs.Prefix, etcd3client.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("[etcdstore] Error performing prefix get on %s: %v", cs.Prefix, err)
	}

	data = []ConfItem{}
	for _, item := range resp.Kvs {
		rel, err := filepath.Rel(cs.Prefix, string(item.Key))
		if err != nil {
			return nil, fmt.Errorf("[etcdstore] Error extracting relative path %s to %s: %v", item.Key, cs.Prefix, err)
		}

		data = append(data, ConfItem{
			Key:     rel,
			Content: item.Value,
		})
	}
	return
}

// Updates returns the ConfStore update channel
func (cs *Etcd3Store) Updates() <-chan ConfEvent {
	return cs.updateChan
}

// Put puts a given value under a given key
func (cs *Etcd3Store) Put(key string, value []byte) error {
	log.Debugf("[etcdstore] Performing put on %s", filepath.Join(cs.Prefix, key))
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	cs.skipNext(key)
	_, err := cs.client.Put(ctx, filepath.Join(cs.Prefix, key), string(value))
	if err != nil {
		return fmt.Errorf("[etcdstore] Error performing put on %s: %v", filepath.Join(cs.Prefix, key), err)
	}
	return nil
}

// Delete deletes a given key
func (cs *Etcd3Store) Delete(key string) error {
	log.Debugf("[etcdstore] Performing delete on %s", filepath.Join(cs.Prefix, key))
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	cs.skipNext(key)
	_, err := cs.client.Delete(ctx, filepath.Join(cs.Prefix, key))
	if err != nil {
		return fmt.Errorf("[etcdstore] Error performing delete on %s: %v", filepath.Join(cs.Prefix, key), err)
	}
	return nil
}

// Close closes the etcd3 client
func (cs *Etcd3Store) Close() error {
	return cs.client.Close()
}

func (cs *Etcd3Store) skipNext(key string) {
	cs.skipMapLock.Lock()
	defer cs.skipMapLock.Unlock()
	cs.skipMap[key] = cs.skipMap[key] + 1
}

func (cs *Etcd3Store) shouldSkip(key string) bool {
	if cs.skipMap[key] <= 0 {
		return false
	}
	cs.skipMapLock.Lock()
	defer cs.skipMapLock.Unlock()
	cs.skipMap[key] = cs.skipMap[key] - 1
	return true
}
