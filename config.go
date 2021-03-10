package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/comail/wincolog"
	"github.com/ddsgok/colog"
)

type configData struct {
	watchPoints     *map[string]*watchPoint
	globalIgnores   *[]*regexp.Regexp
	suppressIgnores *bool
	retainSyncLogs  *bool
	dryRun          *bool
	logFile         **os.File
}

func loadConfig(config configData) {
	appdata := os.Getenv("APPDATA")
	configFile := fmt.Sprintf("%s/Reactor.xml", appdata)

	if _, err := os.Stat(configFile); err == nil {
		doc := etree.NewDocument()
		if err := doc.ReadFromFile(configFile); err != nil {
			log.Panic(err)
		}

		getAttrs := func(a *etree.Element) map[string]string {
			attrMap := make(map[string]string)
			for _, attr := range a.Attr {
				attrMap[attr.Key] = attr.Value
			}
			return attrMap
		}

		root := doc.SelectElement("Reactor")

		colog.Register()
		colog.SetMinLevel(colog.LInfo)
		colog.SetDefaultLevel(colog.LDebug)
		colog.SetParseFields(false)
		// colog.SetFlags(log.LstdFlags | log.Lshortfile)

		logCaptured := false
		if element := root.SelectElement("log"); element != nil {
			attrMap := getAttrs(element)
			fileName, ok := attrMap["file"]
			if ok {
				*config.logFile, err = os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0666)
				if err != nil {
					log.Fatalf("Error opening file: %v", err)
				}

				colog.SetOutput(*config.logFile)
				logCaptured = true
			}

			levelStr, ok := attrMap["level"]
			if ok {
				var level colog.Level = 0
				switch levelStr {
				case "trace":
					level = colog.LTrace
				case "debug":
					level = colog.LDebug
				case "info":
					level = colog.LInfo
				case "message":
					level = colog.LMessage
				case "warn":
					level = colog.LWarning
				case "error":
					level = colog.LError
				case "alert":
					level = colog.LAlert
				}
				colog.SetMinLevel(level)
			}
		}

		if !logCaptured {
			colog.SetOutput(wincolog.Stdout())
		} else {
			log.Println("info: =[start]============================================")
		}

		if element := root.SelectElement("globalIgnores"); element != nil {
			for _, ignore := range element.SelectElements("ignore") {
				exp := regexp.MustCompile(ignore.Text())
				*config.globalIgnores = append(*config.globalIgnores, exp)
			}
		}

		*config.dryRun = false
		if element := root.SelectElement("dryRun"); element != nil {
			attrMap := getAttrs(element)
			value, ok := attrMap["value"]
			if ok {
				if value == "true" {
					*config.dryRun = true
					log.Println("warn: Dry-run mode enabled.")
				}
			}
		}

		*config.suppressIgnores = false
		if element := root.SelectElement("suppressIgnoreEvents"); element != nil {
			attrMap := getAttrs(element)
			value, ok := attrMap["value"]
			if ok {
				if value == "true" {
					*config.suppressIgnores = true
				}
			}
		}

		*config.retainSyncLogs = false
		if element := root.SelectElement("retainSyncLogs"); element != nil {
			attrMap := getAttrs(element)
			value, ok := attrMap["value"]
			if ok {
				if value == "true" {
					*config.retainSyncLogs = true
				}
			}
		}

		if element := root.SelectElement("watchpoints"); element != nil {
			for _, wp := range element.SelectElements("watchpoint") {
				attrMap := getAttrs(wp)
				wpPath, ok := attrMap["path"]
				if ok {
					// sanity check: must be a folder!
					if stat, err := os.Stat(wpPath); err != nil || !stat.IsDir() {
						log.Fatalf("Watchpoint \"%s\" is not a directory!", wpPath)
					}

					ignores := make([]*regexp.Regexp, 0)
					if ig := wp.SelectElement("ignores"); ig != nil {
						for _, ignore := range ig.SelectElements("ignore") {
							exp := regexp.MustCompile(ignore.Text())
							ignores = append(ignores, exp)
						}
					}

					syncDelay := 0
					if sD := wp.SelectElement("syncDelay"); sD != nil {
						attrMap := getAttrs(sD)
						value, ok := attrMap["value"]
						if ok {
							syncDelay, _ = strconv.Atoi(value)
						}
					}

					predicates := make([]processPredicate, 0)
					if pP := wp.SelectElement("processPredicates"); pP != nil {
						for _, pred := range pP.SelectElements("predicate") {
							attrMap := getAttrs(pred)
							key, gotKey := attrMap["key"]
							value, gotValue := attrMap["value"]
							if gotKey && gotValue {
								exp := regexp.MustCompile(key)
								predicates = append(predicates, processPredicate{exp, value})
							}
						}
					}

					remotes := make([]remoteData, 0)
					if rmts := wp.SelectElement("remotes"); rmts != nil {
						for _, rmt := range rmts.SelectElements("remote") {
							var r remoteData

							if id := rmt.SelectElement("id"); id != nil {
								r.id = id.Text()
							} else {
								log.Fatal("Remote defined without specifying an id!")
							}

							r.options = make([]string, 0)
							if opts := rmt.SelectElement("options"); opts != nil {
								r.options = strings.Split(opts.Text(), " ")
							}

							r.cleanSlate = false
							if cS := rmt.SelectElement("cleanSlate"); cS != nil {
								attrMap := getAttrs(cS)
								value, ok := attrMap["value"]
								if ok {
									if value == "true" {
										r.cleanSlate = true
									}
								}
							}

							r.incrUpdates = false
							if iU := rmt.SelectElement("incrementalUpdates"); iU != nil {
								attrMap := getAttrs(iU)
								value, ok := attrMap["value"]
								if ok {
									if value == "true" {
										r.incrUpdates = true
									}
								}
							}

							remotes = append(remotes, r)
						}
					}

					if len(remotes) == 0 {
						log.Fatalf("Watchpoint \"%s\" contains no defined remotes!", wpPath)
					}

					(*config.watchPoints)[wpPath] = &watchPoint{wpPath, remotes, ignores, 0, syncDelay, make(map[string]bool), predicates}
				}
			}
		}
	} else {
		log.Fatal("Config file", configFile, "could not be found!")
	}
}
