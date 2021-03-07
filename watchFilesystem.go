package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

func watchFilesystem(w watchData, quit <-chan bool) {
	defer w.wg.Done()

	done := false
	for !done {
		select {
		case fsevent := <-w.watcher.Event:
			if fsevent == nil {
				log.Fatal("nil event received")
			}

			// filter events globally
			if len(*w.ignores) != 0 {
				fileName := filepath.Base(fsevent.Name)
				for i := range *w.ignores {
					if (*w.ignores)[i].MatchString(fileName) {
						wp := strings.ReplaceAll(fsevent.Name, "\\", "/")
						if !w.suppressIgnores {
							log.Printf("debug: Ignoring event \"%s\" due to ignores directive \"%s\".\n", wp, (*w.ignores)[i].String())
						}
						fsevent = nil
						break
					}
				}
			}

			if fsevent != nil {
				if stat, err := os.Stat(fsevent.Name); err == nil && !stat.IsDir() {
					w.eventsMutex.Lock()
					*w.events = append(*w.events, fsevent.Name)
					w.eventsMutex.Unlock()

					// wake up the processing routine
					*w.eventsAvailable <- true

					// log.Println("received:", fsevent)
				}
			}
		case fserr := <-w.watcher.Error:
			log.Fatalf("Error occurred watching file system: %v", fserr)
		case <-quit:
			done = true
		}
	}
}
