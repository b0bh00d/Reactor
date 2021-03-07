package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/b0bh00d/Reactor/icon"

	"github.com/ddsgok/colog"
	"github.com/getlantern/systray"
	"github.com/google/uuid"
)

var busyIcons []*[]byte = []*[]byte{
	&icon.Reactor_busy_icon1,
	&icon.Reactor_busy_icon2,
	&icon.Reactor_busy_icon3,
	&icon.Reactor_busy_icon4,
	&icon.Reactor_busy_icon5,
	&icon.Reactor_busy_icon6,
	&icon.Reactor_busy_icon7,
	&icon.Reactor_busy_icon8,
	&icon.Reactor_busy_icon9,
	&icon.Reactor_busy_icon10,
	&icon.Reactor_busy_icon11,
	&icon.Reactor_busy_icon12,
	&icon.Reactor_busy_icon13,
	&icon.Reactor_busy_icon14,
	&icon.Reactor_busy_icon15,
	&icon.Reactor_busy_icon16,
	&icon.Reactor_busy_icon17,
	&icon.Reactor_busy_icon18,
	&icon.Reactor_busy_icon19,
	&icon.Reactor_busy_icon20,
	&icon.Reactor_busy_icon21,
	&icon.Reactor_busy_icon22,
	&icon.Reactor_busy_icon23,
	&icon.Reactor_busy_icon24,
	&icon.Reactor_busy_icon25,
	&icon.Reactor_busy_icon26,
	&icon.Reactor_busy_icon27,
	&icon.Reactor_busy_icon28,
	&icon.Reactor_busy_icon29,
	&icon.Reactor_busy_icon30,
	&icon.Reactor_busy_icon31,
	&icon.Reactor_busy_icon32,
	&icon.Reactor_busy_icon33,
	&icon.Reactor_busy_icon34,
	&icon.Reactor_busy_icon35,
}

func busySpinner(quit <-chan bool) {
	spinnerIndex := 0
	done := false
	for !done {
		select {
		case <-time.After(time.Millisecond * 100):
			systray.SetTemplateIcon(*busyIcons[spinnerIndex], *busyIcons[spinnerIndex])
			spinnerIndex++
			if spinnerIndex == 35 {
				spinnerIndex = 0
			}
		case <-quit:
			systray.SetTemplateIcon(icon.Reactor, icon.Reactor)
			done = true
		}
	}
}

func prepRcloneCmd(syncPoint string, remote *remoteData) ([]string, string, string) {
	// make sure the path is in a sane state
	winPath := strings.ReplaceAll(syncPoint, "\\", "/")
	winRel := winPath
	if winPath[1] == ':' {
		winRel = winPath[3:]
	}
	syncPath := filepath.ToSlash(winRel)

	uuid := uuid.New()
	logFilename := fmt.Sprintf("%s/%s_%s.log", os.TempDir(), remote.id, uuid)
	logFilename = strings.ReplaceAll(logFilename, "\\", "/")
	logFileOpt := fmt.Sprintf("--log-file=%s", logFilename)

	cl := []string{"/c", "rclone", "sync", logFileOpt, "--log-level", "INFO"}

	if len(remote.options) != 0 {
		for i := range remote.options {
			cl = append(cl, (*remote).options[i])
		}
	}

	cl = append(cl, syncPath)
	cl = append(cl, fmt.Sprintf("%s:%s", remote.id, syncPath))

	return cl, logFilename, syncPath
}

type localTasks struct {
	mut         sync.Mutex
	tasks       map[*os.Process]int64
	spinning    bool
	spinnerChan chan bool
}

func (l *localTasks) hasTasks() bool {
	l.mut.Lock()
	defer l.mut.Unlock()
	return len(l.tasks) != 0
}

func (l *localTasks) getStart(task *os.Process) int64 {
	l.mut.Lock()
	defer l.mut.Unlock()
	_, ok := l.tasks[task]
	if ok {
		return l.tasks[task]
	}
	return 0
}

func (l *localTasks) addTask(task *os.Process, start int64) {
	l.mut.Lock()
	defer l.mut.Unlock()
	l.tasks[task] = start
}

func (l *localTasks) removeTask(task *os.Process) {
	l.mut.Lock()
	defer l.mut.Unlock()
	_, ok := l.tasks[task]
	if ok {
		delete(l.tasks, task)
		if len(l.tasks) == 0 {
			systray.SetTooltip("Reactor")
			l.spinnerChan <- true
			l.spinning = false
		}
	}
}

func processRcloneSync(w watchData, quit <-chan bool) {
	defer w.wg.Done()

	var currentTasks localTasks
	currentTasks.tasks = make(map[*os.Process]int64)
	currentTasks.spinnerChan = make(chan bool)

	done := false
	for !done {
		select {
		case <-time.After(time.Second * 1):
			if !currentTasks.hasTasks() {
				w.syncMutex.Lock()
				if len((*w.syncTasks)) != 0 {

					syncTask := &(*w.syncTasks)[0]

					// working directory needs to be relative to the watchpoint
					workingDir := syncTask.watching

					index := len(syncTask.watching) - 1
					for index >= 0 && syncTask.watching[index] != '\\' && syncTask.watching[index] != '/' {
						index--
					}
					if index != -1 {
						workingDir = syncTask.watching[:index]
						if strings.HasSuffix(workingDir, ":") {
							workingDir += "/"
						}
					}

					cmdPath, err := exec.LookPath("cmd.exe")
					if err != nil {
						log.Fatal(err)
					}

					firstRun := true

					for i := range syncTask.remotes {
						cl, logFile, syncPath := prepRcloneCmd(syncTask.syncPoint, syncTask.remotes[i])

						if firstRun {
							systray.SetTooltip(fmt.Sprint("Synchronizing ", syncPath))
							currentTasks.spinning = true
							go busySpinner(currentTasks.spinnerChan)
							firstRun = false
						}

						if !w.dryRun {
							go func(remoteId string, cmdPath string, workingDir string, cl []string, syncPoint string, logFile string, retainLog bool) {
								log.Println("info: Executing:", cmdPath, cl, "at", workingDir)

								var procAttr os.ProcAttr
								procAttr.Files = []*os.File{os.Stdin, os.Stdout, os.Stderr}
								procAttr.Dir = workingDir
								procAttr.Sys = &syscall.SysProcAttr{HideWindow: true}

								start := time.Now().Unix()
								// https://gist.github.com/lee8oi/ec404fa99ea0f6efd9d1
								task, err := os.StartProcess(cmdPath, cl, &procAttr)
								if err != nil {
									log.Fatal(err)
								}
								currentTasks.addTask(task, start)

								state, err := task.Wait()
								if err != nil {
									log.Fatal(err)
								}

								currentTasks.removeTask(task)

								prefix := "info: "
								if state.ExitCode() != 0 {
									prefix = "error: "
								}
								logMsg := fmt.Sprintf("%sSync task completed. watchpoint=%s:%s result=%d duration=%ds", prefix, filepath.ToSlash(syncPoint), remoteId, state.ExitCode(), int(time.Now().Unix()-start))
								if state.ExitCode() != 0 {
									file, err := os.Open(logFile)
									if err != nil {
										log.Fatal(err)
									}

									scanner := bufio.NewScanner(file)

									logMsg += ":\n---------------------------\n"
									for scanner.Scan() {
										logMsg += fmt.Sprintf("%s\n", scanner.Text())
									}
									for strings.HasSuffix(logMsg, "\n") {
										logMsg = logMsg[:len(logMsg)-1]
									}
									if err = file.Close(); err != nil {
										log.Fatal(err)
									}
									logMsg += "\n---------------------------"
								}
								colog.SetParseFields(true)
								log.Println(logMsg)
								colog.SetParseFields(false)

								if !retainLog {
									os.Remove(logFile)
								}
							}(syncTask.remotes[i].id, cmdPath, workingDir, cl, syncTask.syncPoint, logFile, w.retainSyncLogs)
						} else {
							log.Println("info: Dry-run:", cmdPath, cl, "at", workingDir)
						}
					}

					// this one has processed; dequeue it
					*w.syncTasks = (*w.syncTasks)[1:]
				}
				w.syncMutex.Unlock()
			}
		case <-quit:
			done = true
		}
	}

	// is our busy indicator running?
	if currentTasks.spinning {
		systray.SetTooltip("Reactor")
		// tell it to stop
		currentTasks.spinnerChan <- true
		currentTasks.spinning = false
	}
}
