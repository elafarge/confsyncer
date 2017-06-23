package main

import (
	"flag"

	confsyncer "github.com/elafarge/confsyncer"
	log "github.com/sirupsen/logrus"
)

func main() {
	var (
		etcdEndpoint string
		kvPrefix     string
		location     string
		justPull     bool

		logLevel string
	)

	flag.StringVar(&etcdEndpoint, "etcd-endpoint", "", "etcd endpoint to reach")
	flag.StringVar(&kvPrefix, "kv-prefix", "", "etcd prefix containing files to synchronize")
	flag.StringVar(&location, "location", "", "folder to sync.")
	flag.BoolVar(&justPull, "just-pull", false, "when true, pull files from etcd and exit")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (panic, fatal, error, warn, info (default), debug")

	flag.Parse()

	// Flag checking
	if etcdEndpoint == "" || kvPrefix == "" || location == "" {
		log.Fatalf("ALL parameters -etcd-endpoint -kv-prefix and -location must be set")
	}

	logrusLogLevel, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Panicf("Impossible to parse log level %s: %v", logLevel, err)
	}

	log.SetLevel(logrusLogLevel)

	// Connecting to remote etcd client
	log.Infof("Connecting to etcd endpoint: %s (prefix: %s)", etcdEndpoint, kvPrefix)
	kvStore, err := confsyncer.NewEtcd3Store(etcdEndpoint, kvPrefix)
	if err != nil {
		log.Fatalf("Impossible to create etcd client: %v", err)
	}
	defer func() {
		if err := kvStore.Close(); err != nil {
			log.Fatalf("Error closing connection to etcd: %v", err)
		}
	}()

	// Creating fsnotify watcher on local config directory
	log.Infof("Creating local directory watcher over directory: %s", location)
	diskStore, err := confsyncer.NewDiskStore(location)
	if err != nil {
		log.Fatalf("Impossible to create local directory watcher: %v", err)
	}
	defer func() {
		if err := diskStore.Close(); err != nil {
			log.Fatalf("Error creating local directory watcher: %v", err)
		}
	}()

	// Let's create our syncer
	syncer := confsyncer.Syncer{LocalStore: diskStore, RemoteStore: kvStore}

	// Pull all config from etcd
	if err = syncer.PullFromRemote(); err != nil {
		log.Fatalf("Impossible to pull files from remote store: %v", err)
	}
	log.Infof("Remote config successfully pulled and applied")

	if !justPull {
		// Let's upload files present locally but missing from the remote store
		if err = syncer.AddLocalConfToRemoteStore(); err != nil {
			log.Fatalf("Impossible to add local conf to remote store: %v", err)
		}
		log.Infof("Missing local files successfully added into remote store")
		// Let's watch over our files for changes with FSNotify in a separate goroutine
		log.Infof("Keeping local and remote config stores in sync...")
		// Let's watch over our files for changes with FSNotify in a separate goroutine
		syncer.KeepInSync()
	}
}
