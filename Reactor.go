//go:generate goversioninfo -icon=resource/icon.ico -manifest=resource/goversioninfo.exe.manifest

package main

import (
	"log"
	"os"
	"regexp"
	"sync"

	"github.com/b0bh00d/Reactor/icon"
	"github.com/b0bh00d/Reactor/winfsnotify"

	"github.com/getlantern/systray"
)

type remoteData struct {
	id          string
	options     []string
	cleanSlate  bool
	incrUpdates bool
}

type syncTask struct {
	watching  string
	remotes   []*remoteData
	syncPoint string
	watch     *watchPoint
}

type processPredicate struct {
	exp  *regexp.Regexp
	proc string
}

type watchPoint struct {
	watching   string
	remotes    []remoteData
	ignores    []*regexp.Regexp
	lastEvent  int64
	syncDelay  int
	events     map[string]bool
	predicates []processPredicate
}

// global configuration data
type watchData struct {
	watcher         *winfsnotify.Watcher
	eventsMutex     *sync.Mutex
	events          *[]string
	eventsAvailable *chan bool
	wg              *sync.WaitGroup
	pointsMutex     *sync.Mutex
	points          *map[string]*watchPoint
	syncMutex       *sync.Mutex
	syncTasks       *[]syncTask
	ignores         *[]*regexp.Regexp
	suppressIgnores bool
	retainSyncLogs  bool
	dryRun          bool
}

func main() {
	onExit := func() {
		log.Println("info: =[exit]=============================================")
	}

	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTemplateIcon(icon.Reactor, icon.Reactor)
	systray.SetTitle("Reactor")
	systray.SetTooltip("Reactor")
	mQuitOrig := systray.AddMenuItem("Quit", "Quit Reactor")

	go run(mQuitOrig)
}

func run(mQuitOrig *systray.MenuItem) {
	watchPoints := make(map[string]*watchPoint)
	globalIgnores := make([]*regexp.Regexp, 0)

	suppressIgnores := false
	retainSyncLogs := false
	dryRun := false

	var logFile *os.File

	config := configData{&watchPoints, &globalIgnores, &suppressIgnores, &retainSyncLogs, &dryRun, &logFile}
	loadConfig(config)

	defer logFile.Close()

	if len(watchPoints) == 0 {
		log.Fatal("No watchpoints have been defined!")
	}

	watcher, err := winfsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("NewWatcher() failed: %s", err)
	}
	defer watcher.Close()

	var wg sync.WaitGroup
	var eventsMutex sync.Mutex
	var pointsMutex sync.Mutex
	var syncMutex sync.Mutex
	var eventFIFO []string

	syncTasks := make([]syncTask, 0)
	quitWatcherChan := make(chan bool)
	quitEventChan := make(chan bool)
	quitSyncChan := make(chan bool)
	quitExecChan := make(chan bool)

	eventsAvailableChan := make(chan bool)

	w := watchData{watcher, &eventsMutex, &eventFIFO, &eventsAvailableChan, &wg, &pointsMutex, &watchPoints, &syncMutex, &syncTasks, &globalIgnores, suppressIgnores, retainSyncLogs, dryRun}

	// if process predicates are currently triggered, this will block until they reset
	scheduleCleanSlates(&w)

	// the processing pipeline is broken into simple, cascading, threaded activities

	wg.Add(1)
	go watchFilesystem(w, quitWatcherChan)

	wg.Add(1)
	go processFilesystemEvent(w, quitEventChan)

	wg.Add(1)
	go processSyncEvent(w, quitSyncChan)

	wg.Add(1)
	go processRcloneSync(w, quitExecChan)

	const watchMask = winfsnotify.FS_ALL_EVENTS & ^(winfsnotify.FS_ATTRIB|winfsnotify.FS_CLOSE) | winfsnotify.FS_IGNORED
	for key := range watchPoints {
		remotes := make([]*remoteData, 0)
		for i := range (*w.points)[key].remotes {
			remote := &(*w.points)[key].remotes[i]
			if remote.incrUpdates {
				remotes = append(remotes, remote)
			}
		}

		if len(remotes) != 0 {
			log.Printf("info: Starting filesystem watch on \"%s\"\n", key)
			err = watcher.AddDeepWatch(key, watchMask)
			if err != nil {
				log.Fatalf("Watcher.Watch() failed: %s", err)
			}
			defer watcher.RemoveWatch(key)
		} else {
			log.Printf("warn: Watchpoint \"%s\" lacks any remotes for incremental updates.", key)
		}
	}

	// wait for the Quit option to trigger
	<-mQuitOrig.ClickedCh

	log.Println("info: =[quit]=============================================")

	quitWatcherChan <- true // watchFilesystem())
	quitEventChan <- true   // processFilesystem()
	quitSyncChan <- true    // processSync()
	quitExecChan <- true    // processExecution()

	wg.Wait()

	systray.Quit()
}
