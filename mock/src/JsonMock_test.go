package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Already declade at JsonMock
var DataDir = "data"
var MockRequestResponseFile = "requestResponseMap.json"
var queryStr string
var dataFile string
var checkUp bool
var gzipOn bool
var goroutinesMax uint64

// To process Json input file
type ReqRes struct {
	Qry string           `json:"query,omitempty"`
	Req *json.RawMessage `json:"req"`
	Res *json.RawMessage `json:"res"`
}

// read extra commandline arguments
func init() {

	runtime.GOMAXPROCS(runtime.NumCPU())

	flag.StringVar(&queryStr, "queryStr", "http://0.0.0.0/testingEnd?", "Testing End address, including 'debug' parameter if needed")
	mockRequestResponseFile := filepath.Dir(os.Args[0]) + filepath.FromSlash("/") + DataDir + filepath.FromSlash("/") + MockRequestResponseFile
	flag.StringVar(&dataFile, "dataFile", mockRequestResponseFile, "Data File with Request/Response map. No validation will be carried out.")
	flag.BoolVar(&checkUp, "checkUp", true, "Check it out that FastCGI is up and running through a HEAD request.")
	flag.BoolVar(&gzipOn, "gzipOn", true, "Activate GZIP by adding specific header to the request. That might make all tests fail")
	flag.Uint64Var(&goroutinesMax, "goroutinesMax", uint64(3*runtime.NumCPU()), "Maximum number of goroutines in parallel in order to avoid hoarding too much resources.")
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

func TestRequests(t *testing.T) {

	// depends on your NGINX fastcgi configuration
	t.Log("-queryStr=" + queryStr)
	// depends on your test configuration
	t.Log("-dataFile=" + dataFile)
	// depends if the server under test supports HEAD queries
	t.Logf("-checkUp=%t\n", checkUp)
	// depends if the server under test supports GZIP
	t.Logf("-gzipOn=s%t\n", gzipOn)
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
	dataMap, err := ioutil.ReadFile(dataFile)
	if err != nil {
		t.Error("Unable to read Mock Request Response File. " + err.Error())
		t.FailNow()
	}

	// process json input
	dec := json.NewDecoder(strings.NewReader(string(dataMap)))
	err = ignoreFirstBracket(dec)
	if err != nil {
		t.Error("Unable to process Mock Request Response File. " + err.Error())
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
	for dec.More() {

		var rr ReqRes
		rr.Qry = ""
		err = dec.Decode(&rr)
		if err != nil || rr.Req == nil || rr.Res == nil {
			t.Error("Unable to process Request Response object.")
			t.FailNow()
		}

		// launch an extra goroutine
		wg.Add(1)
		go checkRequest(current, goroutinesRunning, t, &rr, &wg, &failedRequests, &successRequests)
		current++
		goroutinesRunning++

		// Wait a little when too much goroutines are running in order not to hoard too much resources
		if goroutinesRunning >= goroutinesMax {
			wg.Wait()
			goroutinesRunning = 0
		}
	}
	err = ignoreLastBracket(dec)
	if err != nil {
		t.Error("Unable to process Mock Request Response File. " + err.Error())
		t.FailNow()
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
func checkRequest(current uint64, goroutinesRunning uint64, t *testing.T, rr *ReqRes, wg *sync.WaitGroup, failed *uint64, success *uint64) {

	defer wg.Done()

	query := queryStr
	if len(rr.Qry) > 0 {
		query += rr.Qry
	}

	// create the request
	req, err := toString(rr.Req)
	if err != nil {
		t.Error(err)
		atomic.AddUint64(failed, 1)
		return
	}
	request, err := http.NewRequest("POST", query, strings.NewReader(req))
	if err != nil {
		t.Errorf("<%d:%d> ["+query+"]"+req+": "+err.Error()+"\n", current, goroutinesRunning)
		atomic.AddUint64(failed, 1)
		return
	}
	request.Close = true

	// headeres
	if gzipOn {
		request.Header.Add("Accept-Encoding", "gzip")
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Content-Length", strconv.Itoa(len(req)))

	// making the call
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		t.Errorf("<%d:%d> ["+query+"]"+req+": "+err.Error()+"\n", current, goroutinesRunning)
		atomic.AddUint64(failed, 1)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Errorf("<%d:%d> ["+query+"]"+req+": %d\n", current, goroutinesRunning, response.StatusCode)
		atomic.AddUint64(failed, 1)
		return
	}

	// double check the response depending on GZIP usage
	var reader io.ReadCloser
	switch response.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(response.Body)
		defer reader.Close()
	default:
		reader = response.Body
	}
	res, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Errorf("<%d:%d> ["+query+"]"+req+": "+err.Error()+"\n", current, goroutinesRunning)
		atomic.AddUint64(failed, 1)
		return
	}
	responseStr := string(res)

	// what it's read from the file
	expected, err := toString(rr.Res)
	if err != nil {
		t.Errorf("<%d:%d> ["+query+"]"+req+": "+err.Error()+"\n", current, goroutinesRunning)
		atomic.AddUint64(failed, 1)
		return
	}
	if strings.EqualFold(responseStr, expected) {
		// success
		atomic.AddUint64(success, 1)
		return
	}

	// failure
	t.Errorf("<%d:%d> ["+query+"]"+req+": received->"+responseStr+" expected->"+expected+"\n", current, goroutinesRunning)
	atomic.AddUint64(failed, 1)
	return
}

// Already defined at JsonMock
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

// ignore first bracket when json mock Request Response file is decoded
func ignoreFirstBracket(dec *json.Decoder) error {
	_, err := dec.Token()
	if err != nil {
		log.Fatal(err)
		return errors.New("Unable to process first token at Mock Request Response File")
	}
	return nil
}

// ignore last bracket when json mock Request Response file is decoded
func ignoreLastBracket(dec *json.Decoder) error {
	_, err := dec.Token()
	if err != nil {
		log.Fatal(err)
		return errors.New("Unable to process last token at Mock Request Response File")
	}
	return nil
}

// compact json to make it easy to look into the map for equivalent keys
func compactJson(loose []byte) (string, error) {

	compactedBuffer := new(bytes.Buffer)
	err := json.Compact(compactedBuffer, loose)
	if err != nil {
		log.Fatal(err)
		return "", err
	}
	return compactedBuffer.String(), nil
}
