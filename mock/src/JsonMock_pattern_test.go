package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/xeipuuv/gojsonschema"
)

// Already defined at JsonMock.go
var DataDir = "data"
var DebugParameter = "debug"

// global const
const MockDataFile string = "queries.json"

// global due to lazyness
var queryStr string
var dataFile string
var checkUp bool
var forcedDebug bool
var goroutinesMax uint64
var additionalSchema = make(map[string](*gojsonschema.JSONLoader))

// read extra commandline arguments
func init() {

	runtime.GOMAXPROCS(runtime.NumCPU())

	flag.StringVar(&queryStr, "queryStr", "http://0.0.0.0/testingEnd?", "Testing End address, including 'debug' parameter if needed")
	mockDataFile := filepath.Dir(os.Args[0]) + filepath.FromSlash("/") + DataDir + filepath.FromSlash("/") + MockDataFile
	flag.StringVar(&dataFile, "dataFile", mockDataFile, "Data File with Request/Response map. No validation will be carried out.")
	flag.BoolVar(&checkUp, "checkUp", true, "Check it out that FastCGI is up and running through a HEAD request.")
	flag.Uint64Var(&goroutinesMax, "goroutinesMax", uint64(3*runtime.NumCPU()), "Maximum number of goroutines in parallel in order to avoid hoarding too much resources.")
	flag.BoolVar(&forcedDebug, "debug", false, "Flag to force debug mode.")
	flag.Parse()
	if len(queryStr) < 2 || strings.Index(queryStr, "?") != (len(queryStr)-1) || strings.LastIndex(queryStr, "/") == (len(queryStr)-2) {
		fmt.Printf("Check it out that your -queryStr %v is the correct one expected by NGINX and ends in '?'\n", queryStr)
		os.Exit(1)
	}
	_, err := url.ParseRequestURI(queryStr)
	if err != nil {
		fmt.Printf("Check it out that your -queryStr %v is a correct URL\n", queryStr)
		os.Exit(1)
	}
}

// queries to be requested and validated
type QuerySchema struct {
	query   string
	request string
	schema  *gojsonschema.JSONLoader
}
type Queries []QuerySchema

func ApplyParams(pattern string, params []string) (string, error) {

	if len(params) == 0 {
		return "", errors.New("empty params list")
	}

	// an interface slice is required by sprintf
	args := make([]interface{}, len(params))
	for i, v := range params {
		args[i] = v
	}

	result := fmt.Sprintf(pattern, args...)

	// detect some of the possible errors but NOT ALL
	if strings.Contains(result, "(MISSING)") || strings.Contains(result, "%!(EXTRA") || strings.Contains(result, "%!(BADWITH)") || strings.Contains(result, "%!(BADPREC)") || strings.Contains(result, "(BADINDEX)") {
		return "", errors.New("Sprintf error: " + result)
	}
	return result, nil
}

/*
Example of input json file:
{
  "defaultPattern": "z=%s",
  "defaultSchema": {
    "$schema": "http://json-schema.org/draft-04/schema#",
    "title": "Fake HTTP Response Data",
    "description": "version 0.0.1",
    "type": "object",
    "properties": {
      "url": {
        "type": "string",
        "pattern": "^http://.*$"
      }
    },
    "required": [
      "url"
    ]
  },
  "additionalSchemas": [
    {
      "id": "onlyHTTPS",
      "schema": {
        "$schema": "http://json-schema.org/draft-04/schema#",
        "title": "Fake HTTPS Response Data",
        "description": "version 0.0.1",
        "type": "object",
        "properties": {
          "url": {
            "type": "string",
            "pattern": "^https://.*$"
          }
        },
        "required": [
          "url"
        ]
      }
    }
  ],
  "items": [
    {
      "pattern": "z=%s&ip=%s",
      "params": [
        "150",
        "192.168.0.150"
      ],
      "schema": "onlyHTTPS"
    },
    {
      "params": [
        "160"
      ]
    },
    {
      "params": [
        "170"
      ]
    },
    {
      "params": [
        "180"
      ]
    },
    {
      "params": [
        "190"
      ]
    }
  ]
}
*/
func ReadInfo(filename string, target string, t *testing.T) (Queries, error) {

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	type IdSchema struct {
		Id     string           `json:"id"`
		Schema *json.RawMessage `json:"schema"`
	}
	type Item struct {
		Pattern string           `json:"pattern",omitempty`
		Params  []string         `json:"params"`
		Req     *json.RawMessage `json:"req",omitempty`
		Schema  string           `json:"schema",omitempty`
	}
	type Info struct {
		DefaultPattern    string           `json:"defaultPattern"`
		DefaultSchema     *json.RawMessage `json:"defaultSchema"`
		AdditionalSchemas []IdSchema       `json:"additionalSchemas",omitempty`
		Items             []Item           `json:"items"`
	}

	info := Info{}
	err = json.Unmarshal(bytes, &info)
	if err != nil {
		return nil, err
	}

	t.Logf("Read %d valid items at %s\n", len(info.Items), dataFile)
	queries := make(Queries, 0)

	// regexpr to detect 'debug' params
	var debugRegexp = regexp.MustCompile("^" + DebugParameter + "")

	// default schema is required
	if info.DefaultSchema == nil {
		return nil, errors.New("Missing default Json Schema to validate responses.")
	}

	defaultSchemaStr, err := toString(info.DefaultSchema)
	if err != nil {
		return nil, err
	}
	// default schema cannot be empty
	if len(defaultSchemaStr) == 0 {
		return nil, errors.New("Empty default Json Schema unable to validate responses.")
	}

	var defaultSchema gojsonschema.JSONLoader = gojsonschema.NewStringLoader(defaultSchemaStr)
	if defaultSchema == nil {
		return nil, errors.New("Invalid default Json Schema unable to validate responses.")
	}
	t.Logf("Read valid default schema at %s\n", dataFile)

	// but there can be additional schemas
	if info.AdditionalSchemas != nil {
		for i := 0; i < len(info.AdditionalSchemas); i++ {
			if len(info.AdditionalSchemas[i].Id) > 0 && info.AdditionalSchemas[i].Schema != nil {

				schemaStr, err := toString(info.AdditionalSchemas[i].Schema)
				if err != nil {
					t.Logf("Unable to proccess additional schema %s: %s\n", info.AdditionalSchemas[i].Id, err.Error())
					continue
				}
				// schema cannot be empty
				if len(schemaStr) == 0 {
					t.Logf("Unable to proccess additional an empty schema %s\n", info.AdditionalSchemas[i].Id)
					continue
				}

				var schema gojsonschema.JSONLoader = gojsonschema.NewStringLoader(schemaStr)
				if defaultSchema == nil {
					t.Logf("Unable to proccess additional an nil schema %s\n", info.AdditionalSchemas[i].Id)
					continue
				}

				additionalSchema[info.AdditionalSchemas[i].Id] = &schema
			}
		}
	}
	t.Logf("Read %d additional valid schemas at %s\n", len(additionalSchema), dataFile)

	for i := 0; i < len(info.Items); i++ {
		var args string
		if len(info.Items[i].Pattern) > 0 {
			args, err = ApplyParams(info.Items[i].Pattern, info.Items[i].Params)
		} else {
			args, err = ApplyParams(info.DefaultPattern, info.Items[i].Params)
		}
		if err == nil && len(args) > 0 {
			args = orderQuery(args, debugRegexp)
			// supposed target ended in '?' or something similar to '?debug=true&'
			query := target + args

			// optional request body (json format)
			request := ""
			if info.Items[i].Req != nil {
				request, err = toString(info.Items[i].Req)
				if err != nil {
					t.Logf("Unable to proccess request body: %s\n", err.Error())
					continue
				}
			}

			// Decide what schema to use
			if len(info.Items[i].Schema) == 0 || len(additionalSchema) == 0 {
				queries = append(queries, QuerySchema{query, request, &defaultSchema})
			} else {
				candidate := additionalSchema[info.Items[i].Schema]
				if candidate != nil {
					queries = append(queries, QuerySchema{query, request, candidate})
				}
			}
		}
	}

	t.Logf("Process %d valid queries at %s\n", len(queries), dataFile)
	return queries, nil
}
func TestRequests(t *testing.T) {

	// depends on your NGINX fastcgi configuration
	t.Log("-queryStr=" + queryStr)
	// depends on your test configuration
	t.Log("-dataFile=" + dataFile)
	// depends if the server under test supports HEAD queries
	t.Logf("-checkUp=%t\n", checkUp)
	// depends on the test system resources
	t.Logf("-goroutinesMax=%d\n", goroutinesMax)

	// call that fastcgi to checkout whether it's up or not
	if checkUp {
		ping, err := http.Head(queryStr)
		if err != nil {
			t.Error("Unable to request for HEAD info to the server. " + err.Error())
			t.FailNow()
		}
		if ping.StatusCode != http.StatusOK {
			t.Error("Probably FastCGI down: " + ping.Status)
			t.FailNow()
		}
	}
	// grab the real queries to launch
	queries, err := ReadInfo(dataFile, queryStr, t)
	if err != nil {
		t.Error("Unable to read Mock Request Response File. " + err.Error())
		t.FailNow()
	}

	// resquests stats
	var failedRequests uint64
	var successRequests uint64
	var current uint64
	var goroutinesRunning uint64

	// Get multithread ready
	var wg sync.WaitGroup

	// read object {"req": string, "res": string}
	for i := 0; i < len(queries); i++ {

		// launch an extra goroutine
		wg.Add(1)
		go checkQuery(current, goroutinesRunning, t, &queries[i].query, &queries[i].request, queries[i].schema, &wg, &failedRequests, &successRequests)
		current++
		goroutinesRunning++

		// Wait a little when too much goroutines are running in order not to hoard too much resources
		if goroutinesRunning >= goroutinesMax {
			wg.Wait()
			goroutinesRunning = 0
		}
	}

	// Wait for all gorutines to finish
	if goroutinesRunning > 0 {
		wg.Wait()
	}

	failed := atomic.LoadUint64(&failedRequests)
	success := atomic.LoadUint64(&successRequests)

	if failedRequests > 0 {
		t.Errorf("Failed Requests: %d\n", failed)
		t.FailNow()
	}

	t.Logf("Total requests sent: %d: success %d, failed %d\n", failed+success, success, failed)
}

// process specif request
func checkQuery(current uint64, goroutinesRunning uint64, t *testing.T, queryPtr *string, requestPtr *string, schemaPtr *gojsonschema.JSONLoader, wg *sync.WaitGroup, failed *uint64, success *uint64) {

	defer wg.Done()

	if queryPtr == nil || len(*queryPtr) == 0 {
		t.Errorf("<%d:%d> Empty query\n", current, goroutinesRunning)
		atomic.AddUint64(failed, 1)
		return
	}

	var request *http.Request
	var err error

	if requestPtr == nil || len(*requestPtr) == 0 {
		request, err = http.NewRequest("GET", *queryPtr, nil)
	} else {
		request, err = http.NewRequest("POST", *queryPtr, strings.NewReader(*requestPtr))
		if forcedDebug {
			t.Logf("<%d:%d> Request Body: "+*requestPtr+"\n", current, goroutinesRunning)
		}
	}
	if err != nil {
		t.Errorf("<%d:%d> "+*queryPtr+": "+err.Error()+"\n", current, goroutinesRunning)
		atomic.AddUint64(failed, 1)
		return
	}
	request.Close = true
	if forcedDebug {
		t.Logf("<%d:%d> Requested: "+*queryPtr+"\n", current, goroutinesRunning)
	}

	request.Header.Add("Content-Type", "application/json")

	// making the call
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		t.Errorf("<%d:%d> "+*queryPtr+": "+err.Error()+"\n", current, goroutinesRunning)
		atomic.AddUint64(failed, 1)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Errorf("<%d:%d> Failed with HTTP Code %d ["+*queryPtr+"]\n", current, goroutinesRunning, response.StatusCode)
		atomic.AddUint64(failed, 1)
		return
	}
	res, err := ioutil.ReadAll(response.Body)
	if err != nil {
		t.Errorf("<%d:%d> "+*queryPtr+": "+err.Error()+"\n", current, goroutinesRunning)
		atomic.AddUint64(failed, 1)
		return
	}

	if forcedDebug {
		t.Logf("<%d:%d> Received: "+string(res)+"\n", current, goroutinesRunning)
	}

	result, err := gojsonschema.Validate(*schemaPtr, gojsonschema.NewStringLoader(string(res)))
	if err != nil {
		t.Errorf("<%d:%d> Failed Response validation for query "+*queryPtr+": "+err.Error()+"\n", current, goroutinesRunning)
		atomic.AddUint64(failed, 1)
		return
	}
	if !result.Valid() {
		t.Errorf("<%d:%d> Failed Response validation details for query: %s\n", current, goroutinesRunning, *queryPtr)
		if forcedDebug {
			for _, desc := range result.Errors() {
				t.Errorf("<%d:%d> Failed Response - %s\n", current, goroutinesRunning, desc)
			}
		}
		atomic.AddUint64(failed, 1)
		return
	}

	t.Logf("<%d:%d> "+*queryPtr+": received and validated\n", current, goroutinesRunning)
	atomic.AddUint64(success, 1)
	return
}

// order query string by params in order to match ordered generated r.URL.Query() values later on
func orderQuery(query string, debugRegexp *regexp.Regexp) string {
	if len(query) > 0 {
		list := strings.Split(query, "&")
		sort.Strings(list) // supposed short lists than don't care to be ordered in memory
		var result string
		// remove debug parameters
		for i, v := range list {
			v = debugRegexp.ReplaceAllString(v, "")
			if len(v) == 0 {
				continue
			} else if len(v) > 0 && v[0] == '=' {
				continue
			} else {
				if len(result) > 0 {
					result += "&"
				}
				result += list[i]
			}
		}
		return result
	}
	return ""
}

// convert into an string
func toString(raw *json.RawMessage) (string, error) {
	if raw != nil {
		noSoRaw, err := json.Marshal(raw)
		if err != nil {
			log.Fatal(err)
			return "", err
		}
		return string(noSoRaw), nil
	} else {
		return "", nil
	}
}
