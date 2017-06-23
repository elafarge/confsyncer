package confsyncer

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
)

// DiskStore implements a disk backed config store
type DiskStore struct {
	ConfStore

	Location string

	updateChan  chan ConfEvent
	fwatcher    *fsnotify.Watcher
	dwatcher    *fsnotify.Watcher
	skipMap     map[string]int
	skipMapLock sync.Mutex
}

// NewDiskStore instantiates a new disk ConfigStore
func NewDiskStore(location string) (cs *DiskStore, err error) {
	cs = &DiskStore{
		Location: location,

		updateChan: make(chan ConfEvent),
		skipMap:    map[string]int{},
	}

	if cs.fwatcher, err = fsnotify.NewWatcher(); err != nil {
		return nil, fmt.Errorf("[diskstore] Error creating fsnotify file watcher: %v", err)
	}

	if cs.dwatcher, err = fsnotify.NewWatcher(); err != nil {
		return nil, fmt.Errorf("[diskstore] Error creating fsnotify dir watcher: %v", err)
	}

	// Let's watch files in our config directory and build a set of intermediate dirs
	interms := map[string]struct{}{}
	err = filepath.Walk(cs.Location, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("Error walking %s: %v", path, err)
		}
		if info.IsDir() {
			interms[path] = struct{}{}
		} else {
			log.Debugf("[diskstore] Watching file %s", path)
			cs.fwatcher.Remove(path)
			if err := cs.fwatcher.Add(path); err != nil {
				return fmt.Errorf("Error adding fsnotify watcher on %s: %v", path, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("[diskstore] Error walking %s directory: %v", cs.Location, err)
	}

	// Let's also add intermediate directories to the watchlist
	for interm := range interms {
		log.Debugf("[diskstore] Watching intermediate directory %s", interm)
		cs.fwatcher.Remove(interm)
		if err := cs.dwatcher.Add(interm); err != nil {
			return nil, fmt.Errorf("[diskstore] Error watching intermediate directory: %v", err)
		}
	}

	// Let's watch events happening on our files
	go func() {
		for ev := range cs.fwatcher.Events {
			rel, err := filepath.Rel(cs.Location, ev.Name)
			if err != nil {
				log.Errorf("[diskstore] Error computing rel path from %s to %s: %v", ev.Name, cs.Location, err)
				break
			}

			switch ev.Op {
			case fsnotify.Remove:
				log.Debugf("[diskstore] Got delete fsnotify notification for file %s", ev.Name)

				cs.updateChan <- ConfEvent{
					Type: ConfDelete,
					Key:  rel,
				}
			case fsnotify.Write:
				log.Debugf("[diskstore] Got write notification for file %s", ev.Name)
				content, err := ioutil.ReadFile(ev.Name)
				if err != nil {
					log.Errorf("Error reading changed file %s: %v", ev.Name, err)
				}
				cs.updateChan <- ConfEvent{
					Type:    ConfPut,
					Key:     rel,
					Content: content,
				}
			default:
				log.Debugf("[diskstore] Unsupported fsnotify operation on %s: %s", ev.Name, ev.Op)
			}
		}
	}()

	// Let's watch directories for new file creation
	go func() {
		for ev := range cs.dwatcher.Events {
			switch ev.Op {
			case fsnotify.Create:
				stat, err := os.Stat(ev.Name)
				if err != nil {
					log.Errorf("[diskstore] Error running stat on %s: %s", ev.Name, err)
				}

				if stat.IsDir() {
					log.Debugf("[diskstore] Watching new dir %s", ev.Name)
					cs.dwatcher.Add(ev.Name)
				} else {
					log.Debugf("[diskstore] Watching new file %s", ev.Name)
					content, err := ioutil.ReadFile(ev.Name)
					if err != nil {
						log.Errorf("Error reading new file %s: %v", ev.Name, err)
					}
					rel, err := filepath.Rel(cs.Location, ev.Name)
					if err != nil {
						log.Errorf("[diskstore] Error computing rel path from %s to %s: %v", ev.Name, cs.Location, err)
						break
					}
					cs.updateChan <- ConfEvent{
						Type:    ConfPut,
						Key:     rel,
						Content: content,
					}

					cs.fwatcher.Add(ev.Name)
				}
			}
		}
	}()

	// Let's watch and log fsnotify errors too... just in case :)
	go func() {
		for err := range cs.fwatcher.Errors {
			log.Errorf("[diskstore] Got fsnotify error: %v", err)
		}
	}()

	return
}

// List all conf from local disk
func (cs *DiskStore) List() (data []ConfItem, err error) {
	data = []ConfItem{}
	err = filepath.Walk(cs.Location, func(path string, info os.FileInfo, err error) error {
		rel, err := filepath.Rel(cs.Location, path)
		if err != nil {
			return fmt.Errorf("[diskstore] Error extracting relative path %s to %s: %v", path, cs.Location, err)
		}

		stat, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("[diskstore] Error calling stat on %s: %v", path, err)
		}

		if !stat.IsDir() {
			content, err := ioutil.ReadFile(path)
			if err != nil {
				return fmt.Errorf("Error reading file %s: %v", path, content)
			}

			data = append(data, ConfItem{
				Key:     rel,
				Content: content,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("[diskstore] Error walking %s directory: %v", cs.Location, err)
	}
	return data, err
}

// Updates returns the ConfStore update channel
func (cs *DiskStore) Updates() <-chan ConfEvent {
	return cs.updateChan
}

// Put writes the content of a given config file
func (cs *DiskStore) Put(key string, content []byte) error {
	log.Debugf("[diskstore] Performing put on %s", filepath.Join(cs.Location, key))
	return cs.writeFile(filepath.Join(cs.Location, key), content)
}

// Delete deletes a given file
func (cs *DiskStore) Delete(key string) error {
	log.Debugf("[diskstore] Performing delete on %s", filepath.Join(cs.Location, key))
	if err := cs.fwatcher.Remove(filepath.Join(cs.Location, key)); err != nil {
		return fmt.Errorf("[diskstore] Impossible to unwatch file %s: %v", key, err)
	}

	if err := os.Remove(filepath.Join(cs.Location, key)); err != nil {
		return fmt.Errorf("[diskstore] Impossible to remove file %s: %v", key, err)
	}

	return nil
}

// Close closes the fsnotify watcher
func (cs *DiskStore) Close() error {
	return cs.fwatcher.Close()
}

func (cs *DiskStore) writeFile(path string, content []byte) error {
	cs.fwatcher.Remove(path)
	parentDir := filepath.Dir(path)

	if err := os.MkdirAll(parentDir, os.ModeDir|0755); err != nil {
		return fmt.Errorf("[diskstore] Impossible to create dir %s: %v", parentDir, err)
	}

	if err := ioutil.WriteFile(path, content, 0755); err != nil {
		return fmt.Errorf("[diskstore] Impossible to write file %s: %v", path, err)
	}

	if err := cs.fwatcher.Add(path); err != nil {
		return fmt.Errorf("[diskstore] Impossible to watch file %s: %v", path, err)
	}

	log.Debugf("[diskstore] Written file %s", path)
	return nil
}
