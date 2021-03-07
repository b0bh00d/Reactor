package main

import (
	"log"
	"path/filepath"
	"strings"
	"time"

	procList "github.com/mitchellh/go-ps"
)

func processSyncEvent(w watchData, quit <-chan bool) {
	defer w.wg.Done()

	done := false
	for !done {
		select {
		case <-time.After(time.Second * 1):
			w.pointsMutex.Lock()
			if len(*w.points) != 0 {
				now := time.Now().Unix()
				candidates := make([]string, 0) // fs point keys that need synchronizing
				for key := range *w.points {
					l := len((*w.points)[key].events)
					if l != 0 {
						if int(now-(*w.points)[key].lastEvent) > (*w.points)[key].syncDelay {
							candidates = append(candidates, key)
						}
					}
				}

				if len(candidates) > 0 {
					// see if we neeed to regard any process predicates before sending
					// events to be synchronized

					processes := make(map[string]bool)
					p, _ := procList.Processes()
					for i := range p {
						processes[p[i].Executable()] = true
					}

					for i := range candidates {
						preds := (*w.points)[candidates[i]].predicates
						if len(preds) != 0 {
							for j := range preds {
								for event := range (*w.points)[candidates[i]].events {
									if preds[j].exp.MatchString(event) {
										_, ok := processes[preds[j].proc]
										if ok {
											// ignore this event; its predicate is triggered
											log.Printf("debug: Ignoring event \"%s\" due to process predicate \"%s\".\n", event, preds[j].proc)
											delete((*w.points)[candidates[i]].events, event)
										}
									}
								}
							}
						}

						if l := len((*w.points)[candidates[i]].events); l != 0 {
							// boil down the events into the deepest pathways possible.  this is
							// the opposite of the 'common prefix' approach, which can end up causing
							// the entire watchpoint to be processed when only a few small files in
							// unrelated folders been modified.

							syncEvents := make(map[string]int)
							for event := range (*w.points)[candidates[i]].events {
								dir := filepath.Dir(event)
								_, ok := syncEvents[dir]
								if !ok {
									syncEvents[dir] = 1
									syncEventsDel := make(map[string]bool)
									for sP := range syncEvents {
										if strings.HasPrefix(dir, sP) {
											if len(dir) != len(sP) {
												// this entry is a deeper version of an existing task; discard it
												syncEventsDel[dir] = true
												syncEvents[sP]++
											}
										} else if strings.HasPrefix(sP, dir) {
											// this entry is a shallower version of an existing entry; remember it
											syncEvents[dir] += syncEvents[sP]
											syncEventsDel[sP] = true
										}
									}

									if len(syncEventsDel) != 0 {
										for key := range syncEventsDel {
											delete(syncEvents, key)
										}
									}
								} else {
									syncEvents[dir]++
								}
							}

							remotes := make([]*remoteData, 0)
							for j := range (*w.points)[candidates[i]].remotes {
								remotes = append(remotes, &(*w.points)[candidates[i]].remotes[j])
							}

							w.syncMutex.Lock()

							for path := range syncEvents {
								l = syncEvents[path]
								plural := ""
								if l > 1 {
									plural = "s"
								}
								for k := range remotes {
									log.Printf("info: Submitting \"%s:%s\" for %d event%s.", filepath.ToSlash(path), remotes[k].id, l, plural)
								}
								*w.syncTasks = append(*w.syncTasks, syncTask{candidates[i], remotes, filepath.ToSlash(path), (*w.points)[candidates[i]]})
							}

							w.syncMutex.Unlock()

							for key := range (*w.points)[candidates[i]].events {
								delete((*w.points)[candidates[i]].events, key)
							}
						}
					}
				}
			}
			w.pointsMutex.Unlock()
		case <-quit:
			done = true
		}
	}
}
