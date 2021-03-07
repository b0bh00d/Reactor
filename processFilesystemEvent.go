package main

import (
	"log"
	"path/filepath"
	"strings"
	"time"
)

func processFilesystemEvent(w watchData, quit <-chan bool) {
	defer w.wg.Done()

	done := false
	for !done {
		select {
		case token := <-*w.eventsAvailable:
			if !token {
				log.Fatal("nil event received")
			}

			w.eventsMutex.Lock()
			if len(*w.events) > 0 {
				w.pointsMutex.Lock()
				for len(*w.events) != 0 {
					event := strings.ReplaceAll((*w.events)[0], "\\", "/")
					*w.events = (*w.events)[1:] // dequeue event
					for key, point := range *w.points {
						if strings.HasPrefix(event, key) {

							fileName := filepath.Base(event)

							if len(point.ignores) != 0 {
								for i := range point.ignores {
									if point.ignores[i].MatchString(fileName) {
										if !w.suppressIgnores {
											log.Printf("debug: Ignoring event \"%s\" due to ignores directive \"%s\".\n", event, point.ignores[i].String())
										}
										event = ""
										break
									}
								}
							}

							if len(event) != 0 {
								// log.Println("Event occurred on watchpoint:", key)

								_, ok := point.events[event]
								if !ok {
									point.events[event] = true
									log.Printf("debug: Queuing new event: \"%s\"\n", event)
								}
								// reset the timer
								point.lastEvent = time.Now().Unix()
							}
						}
					}
				}
				w.pointsMutex.Unlock()
			}
			w.eventsMutex.Unlock()
		case <-quit:
			done = true
		}
	}
}
