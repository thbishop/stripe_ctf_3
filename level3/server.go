package main

import (
  "bufio"
  "encoding/json"
  "fmt"
  "log"
  "net/http"
  "os"
  "path/filepath"
  "runtime"
  "time"
  "sort"
  "strconv"
  "strings"
  "sync"
)

type IngestItem struct {
    SubstrIndex int
    Locations []Location
}

type Location struct {
    PathIndex int
    Line  int
}

type QueryResult struct {
	Success bool `json:"success"`
	Results []string `json:"results"`
}

type Status struct {
	Success bool `json:"success"`
}

type LineRecord struct {
	Data map[int]map[int]string
	Lock sync.RWMutex
}

var nCPU = runtime.NumCPU()

var dictionary map[string]int
var indexMap map[int][]Location
var pathList []string
var lineRecord LineRecord
var indexFinished = false
var minSubstrLen = 4
var indexStart int64

func LocationsToStrings(locations []Location) (strs []string) {
  strs = make([]string, len(locations))
  for i, r := range locations {
    strs[i] = fmt.Sprintf("%s:%d", pathList[r.PathIndex], r.Line)
  }
  return
}

func importWorker(workerNum int, pathChan chan int, statusChan chan string, ingestChan chan IngestItem) {
  for i := range pathChan {
    importFile(i, statusChan, ingestChan)
  }
}

func importFile(pathIndex int, statusChan chan string, ingestChan chan IngestItem) error {
  path := pathList[pathIndex]

  file, err := os.Open(path)
  if err != nil {
    return err
  }
  defer file.Close()

  lineScanner := bufio.NewScanner(file)
  var lineNum = 0

  lineRecord.Lock.Lock()
  lineRecord.Data[pathIndex] = make(map[int]string)
  lineRecord.Lock.Unlock()

  for lineScanner.Scan() {
    lineNum += 1

	lineRecord.Lock.Lock()
    lineRecord.Data[pathIndex][lineNum] = lineScanner.Text()
	lineRecord.Lock.Unlock()

    r := strings.NewReader( lineScanner.Text() )

    wordScanner := bufio.NewScanner( r )
    wordScanner.Split(bufio.ScanWords)

    for wordScanner.Scan() {
      str := wordScanner.Text()

      // for all substrings
      for substrLen := minSubstrLen; substrLen <= len(str); substrLen++ {
        for i := 0; i <= len(str) - substrLen; i++ {
          substr := str[i:i+substrLen]

          if idx,ok := dictionary[substr]; ok {

            list, _ := indexMap[idx]
            ingestChan <- IngestItem{idx, append(list, Location{pathIndex, lineNum})}
			list = nil
          }
        }
      }

    }

	r = nil
	wordScanner = nil
  }

  lineScanner = nil

  statusChan <- path

  runtime.GC()

  return nil
}

// handles writes to index
func writeIndexEntries(ingestChan chan IngestItem) {
  for i := range ingestChan {
    indexMap[i.SubstrIndex] = i.Locations
  }
}

func monitorStatus(c chan string) {
  startTime := time.Now().UnixNano()

  for i := 0; i < len(pathList); i++ {
	 _ = <-c
  }

  indexFinished = true

  endTime := time.Now().UnixNano()

  elapsed := float32(endTime-startTime)/1E6

  fmt.Println("Monitor index finished in:", elapsed, " ms" )
}

func dedup(data []string ) []string {
  sort.Strings(data)

  length := len(data) - 1

  for i := 0; i < length; i++ {
    for j := i + 1; j <= length; j++ {
      if (data[i] == data[j]) {
        data[j] = data[length]
        data = data[0:length]
        length--
        j--
      }
    }
  }

  return data
}

func searchManual(q string) []string {
  results := []string{}

  for idx, path := range(pathList) {
    for lineNum, text := range lineRecord.Data[idx] {
      if strings.Contains(text, q) {
        results = append(results, fmt.Sprintf("%s:%d", path, lineNum))
      }
    }
  }

  return results
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
  m := Status{true}
  rep, _ := json.Marshal(m)
  w.Write(rep)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
  path := r.FormValue("path")

  log.Println("Start indexing (" + path + ")")

  os.Chdir(path)

  statusChan := make(chan string)
  pathChan := make(chan int)
  ingestChan := make(chan IngestItem)

  idx := 0
  // generate the path list so we know the number before we start the monitor
  filepath.Walk("./", func(path string, _ os.FileInfo, _ error) error {
	pathList = append(pathList, path)
    idx += 1

    return nil
  })

  log.Println("Found " + strconv.Itoa(len(pathList)) + " items in our path list")

  go monitorStatus(statusChan)
  go writeIndexEntries(ingestChan)

  for numWorkers := 0; numWorkers < 20; numWorkers++ {
    go importWorker(numWorkers, pathChan, statusChan, ingestChan)
  }

  go func () {
    for i, _ := range pathList {
      pathChan <- i
    }

    close(pathChan)

  }()

  m := Status{true}
  rep, _ := json.Marshal(m)
  w.Write(rep)
}

func isIndexedHandler(w http.ResponseWriter, r *http.Request) {
  m := Status{indexFinished}
  rep, _ := json.Marshal(m)
  w.Write(rep)
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
  q := r.FormValue("q")
  var results []string

  if len(q) < minSubstrLen {
    results = searchManual(q)
  }else{
    idx,ok := dictionary[q]

    if ok {
      results = LocationsToStrings(indexMap[idx])
    }
  }

  qr := QueryResult{true, dedup(results)}
  response, _ := json.Marshal(qr)

  w.Write(response)
}

func loadDictionary() {
  file, _ := os.Open("./dictionary.txt")

  defer file.Close()

  lineScanner := bufio.NewScanner(file)

  lineNum := 0

  for lineScanner.Scan() {
    lineNum += 1
      dictionary[lineScanner.Text()] = lineNum
  }

  log.Print("Dictionary Size: ", len(dictionary))
}

func main() {
  runtime.GOMAXPROCS(nCPU)
  log.Print("Set max procs to " + strconv.Itoa(nCPU))

  pathList = []string{}
  indexMap = make(map[int][]Location)
  dictionary = make(map[string]int)
  lineRecord = LineRecord{make(map[int]map[int]string), sync.RWMutex{}}

  loadDictionary()

  http.HandleFunc("/healthcheck", healthCheckHandler)
  http.HandleFunc("/index", indexHandler)
  http.HandleFunc("/isIndexed", isIndexedHandler)
  http.HandleFunc("/", queryHandler)

  http.ListenAndServe(":9090", nil)
}
