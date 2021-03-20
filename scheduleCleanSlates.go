package main

import (
	"log"
	"time"

	procList "github.com/mitchellh/go-ps"
)

func scheduleCleanSlates(w *watchData) {
	// do we have watchpoints with remotes that require 'clean-slate' updating
	// AND process predicates defined?

	candidates := make([]*watchPoint, 0)

	for key := range *w.points {
		remotes := make([]*remoteData, 0)
		for i := range (*w.points)[key].remotes {
			remote := &(*w.points)[key].remotes[i]
			if remote.cleanSlate {
				remotes = append(remotes, remote)
			}
		}
		if len(remotes) != 0 {
			candidates = append(candidates, (*w.points)[key])
		}
	}

	// first level of filtering: watchpoints with remotes that require clean-
	// slate updates
	if len(candidates) != 0 {
		// check for process predicates.
		i := 0
		for i < len(candidates) {
			if len(candidates[i].predicates) == 0 {
				// remove it
				candidates = candidates[1:]
				i = 0
			} else {
				i++
			}
		}

		// second level of filtering: watchpoints with clean-slate updates AND
		// process predicates
		if len(candidates) != 0 {
			// if process predicates are currently triggered (i.e., the process
			// is running), we will need to delay scheduling until the process
			// (or processes) terminate.

			done := false
			firstWarning := true

			for !done {
				// get a snapshot of the current process list
				processes := make(map[string]bool)
				p, _ := procList.Processes()
				for i := range p {
					processes[p[i].Executable()] = true
				}

				predicateTriggered := false

				for i := range candidates {
					preds := candidates[i].predicates
					for j := range preds {
						_, ok := processes[preds[j].proc]
						if ok {
							predicateTriggered = true
						}
					}
				}

				if !predicateTriggered {
					done = true
				} else {
					if firstWarning {
						log.Print("warn: One or more process predicates are triggered; Reactor will wait for them to reset before continuing.\n")
						firstWarning = false
					}
					time.Sleep(5 * time.Second)
				}
			}
		}
	}

	// now schedule 'clean slates' unconditionally
	for key := range *w.points {
		remotes := make([]*remoteData, 0)
		for i := range (*w.points)[key].remotes {
			remote := &(*w.points)[key].remotes[i]
			if remote.cleanSlate {
				log.Printf("warn: Scheduling \"%s:%s\" for a clean-slate sync.", key, remote.id)
				remotes = append(remotes, remote)
			}
		}
		if len(remotes) != 0 {
			*w.syncTasks = append(*w.syncTasks, syncTask{key, remotes, key, (*w.points)[key]})
		}
	}
}
