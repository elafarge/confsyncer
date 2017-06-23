package confsyncer

import (
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"
)

// Syncer keeps a local config store in sync with a remote config stores
type Syncer struct {
	LocalStore  ConfStore
	RemoteStore ConfStore

	wg sync.WaitGroup
}

// PullFromRemote pulls the config from the remote store without triggering any
// event. It's useful to initialize the config from the remote single source of
// truth.
func (s *Syncer) PullFromRemote() error {
	data, err := s.RemoteStore.List()
	if err != nil {
		return fmt.Errorf("[syncer] Error pulling data from remote: %v", err)
	}

	for _, item := range data {
		log.Debugf("[syncer] Pulling file %s from remote store", item.Key)
		s.LocalStore.Put(item.Key, item.Content)
	}

	return nil
}

// AddLocalConfToRemoteStore puts on the remote all the local config files that
// aren't already in there
func (s *Syncer) AddLocalConfToRemoteStore() error {
	remoteData, err := s.RemoteStore.List()
	if err != nil {
		return fmt.Errorf("[syncer] Error pulling data from remote: %v", err)
	}

	localData, err := s.LocalStore.List()
	if err != nil {
		return fmt.Errorf("[syncer] Error creating local config map: %v", err)
	}

	// Let's remove items present in the remoteList from the local one
	for _, remoteItem := range remoteData {
		for i, localItem := range localData {
			if localItem.Key == remoteItem.Key {
				localData = append(localData[:i], localData[i+1:]...)
			}
		}
	}

	// And update the remote with missing local files
	for _, item := range localData {
		log.Debugf("[syncer] Pushing missing file %s to remote store", item.Key)
		s.RemoteStore.Put(item.Key, item.Content)
	}

	return nil
}

// KeepInSync makes sure all config stores are kept in sync
func (s *Syncer) KeepInSync() {
	s.wg.Add(2)

	// Propagates events from local store to remote store
	go func() {
		defer s.wg.Done()
		for ev := range s.LocalStore.Updates() {
			// Let's stop watching while the modification takes place to stop
			// stores from playing ping-pong.
			if ev.Type == ConfPut {
				log.Infof("[syncer] Propagating Put event for key %s to remote store", ev.Key)
				if err := s.RemoteStore.Put(ev.Key, ev.Content); err != nil {
					log.Errorf("[syncer] Error calling Put for key %s to remote store: %v", ev.Key, err)
				}
			} else if ev.Type == ConfDelete {
				log.Infof("[syncer] Propagating Delete event for key %s to remote store", ev.Key)
				if err := s.RemoteStore.Delete(ev.Key); err != nil {
					log.Errorf("[syncer] Error calling Delete for key %s to remote store: %v", ev.Key, err)
				}
			} else {
				log.Errorf("[syncer] Unknown event type %d", ev.Type)
			}
		}
	}()

	// Propagates events from remote store to local store
	go func() {
		defer s.wg.Done()
		for ev := range s.RemoteStore.Updates() {
			// Let's stop watching while the modification takes place to stop
			// stores from playing ping-pong.
			if ev.Type == ConfPut {
				log.Infof("[syncer] Propagating Put event for key %s to local store", ev.Key)
				if err := s.LocalStore.Put(ev.Key, ev.Content); err != nil {
					log.Errorf("[syncer] Error calling Put for key %s to local store: %v", ev.Key, err)
				}
			} else if ev.Type == ConfDelete {
				log.Infof("[syncer] Propagating Delete event for key %s to local store", ev.Key)
				if err := s.LocalStore.Delete(ev.Key); err != nil {
					log.Errorf("[syncer] Error calling Delete for key %s to local store: %v", ev.Key, err)
				}
			} else {
				log.Errorf("[syncer] Unknown event type %d", ev.Type)
			}
		}
	}()

	// Let's hold on until our goroutines finish... which souldn't happen :)
	s.wg.Wait()
}
