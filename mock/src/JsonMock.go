/**

@startuml component_diagram.png

package "Json Mock" {
FastCGI - [Mock Server]
File - [Json Schema Validator]
[Json Schema Validator] --> [Mock Server] : Req/Res Schemas\nValidated fake data
}

node "Json Schemas" {
[Request] --> File
[Response] --> File
[Data] --> File
}

note bottom of [Response] : JSON files that can\nbe externally validated

node "Logs" {
[Mock Server] --> [Debug Mode]
[Json Schema Validator] --> [Debug Mode]
}

note right of [Debug Mode] : Provides insights on\nREAL or FAKE client requests

database "Fake Queries" {
[Map: Query+Request -> Response] --> File
}
note bottom of [Map: Query+Request -> Response] : JSON files that can\nbe externally validated


package "HTTP Server" {
HTTP - [NGINX]
[NGINX] --> FastCGI : Qry/Req
FastCGI --> [NGINX] : Validated Res
}

note right of [NGINX]: Can be reconfigured\nto point to the REAL FastCGI\nto validate REAL deployments

cloud "Clients" {
[Client apps] --> HTTP
[Curl commandline] --> HTTP
[Multithreaded\ntesting\nprograms] --> HTTP
}

note bottom of [Client apps] : Could be REAL or FAKE apps

@enduml

**/

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/xeipuuv/gojsonschema"
)

type QueryResponse struct {
	query    string
	response string
}

// Request Response map
type RequestResponseMap map[string]QueryResponse

// helper for HTTP handler queries
type customHandler struct {
	cmux        http.Handler
	rrmap       *RequestResponseMap
	reqJS       *gojsonschema.JSONLoader
	forcedDebug bool
}

// Expected data Dir
var DataDir = "data"

// RequestJsonSchema to validate requests
var RequestJsonSchemaFile = "requestJsonSchema.json"

// ResponseJsonSchema to validate responses
var ResponseJsonSchemaFile = "responseJsonSchema.json"

// MockRequestResponseFile global var due to lazyness
var MockRequestResponseFile = "requestResponseMap.json"

// DebugParameter global var due to lazyness
var DebugParameter = "debug"
var ForcedDebug = false

func main() {

	host, port, mockRequestResponseFile, requestJsonSchemaFile, responseJsonSchemaFile, forcedDebug := cmdLine()
	log.Printf("Launched "+os.Args[0]+" -host="+host+" -port="+port+" -map="+mockRequestResponseFile+
		" -req="+requestJsonSchemaFile+" -res="+responseJsonSchemaFile+" -debug=%t", forcedDebug)

	reqresmap, reqJS, err := validateMockRequestResponseFile(mockRequestResponseFile, requestJsonSchemaFile, responseJsonSchemaFile, forcedDebug)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Number of fake request/response: %d", len(reqresmap))

	mux := mux.NewRouter()
	// bind cmux to mx(route) and rrmap to reqresmap
	fcgiHandler := &customHandler{cmux: mux, rrmap: &reqresmap, reqJS: &reqJS, forcedDebug: forcedDebug}
	mux.Path("/").Handler(fcgiHandler)

	listener, _ := net.Listen("tcp", host+":"+port) // see nginx.conf
	if err := fcgi.Serve(listener, fcgiHandler); err != nil {
		log.Fatal(err)
	}
}

// get command line parameters
func cmdLine() (string, string, string, string, string, bool) {

	hostArg := "0.0.0.0"
	portArg := "9797"
	mockRequestResponseFile := filepath.Dir(os.Args[0]) + filepath.FromSlash("/") + DataDir + filepath.FromSlash("/") + MockRequestResponseFile
	requestJsonSchemaFile := filepath.Dir(os.Args[0]) + filepath.FromSlash("/") + DataDir + filepath.FromSlash("/") + RequestJsonSchemaFile
	responseJsonSchemaFile := filepath.Dir(os.Args[0]) + filepath.FromSlash("/") + DataDir + filepath.FromSlash("/") + ResponseJsonSchemaFile
	forcedDebug := ForcedDebug

	cmd := strings.Join(os.Args, " ")
	if strings.Contains(cmd, " help") || strings.Contains(cmd, " -help") || strings.Contains(cmd, " --help") ||
		strings.Contains(cmd, " -h") || strings.Contains(cmd, " /?") {
		fmt.Println()
		fmt.Println("Usage: " + os.Args[0] + " -host=<host> -port=<port> -map=<MockRequestResponseFile> -req=<RequestJsonSchema> -res=<ResponseJsonSchema> -debug=<ForcedDebug>")
		fmt.Println()
		fmt.Println("host:  Host name for this FastCGI process.   By default " + hostArg)
		fmt.Println("port:  Port number for this FastCGI process. By default " + portArg)
		fmt.Println()
		fmt.Println("map: Fake mapped request/response file. By default " + mockRequestResponseFile)
		fmt.Println("req: Json Schema to validate requests.  By default " + requestJsonSchemaFile)
		fmt.Println("res: Json Schema to validate responses. By default " + responseJsonSchemaFile)
		fmt.Println()
		fmt.Printf("debug:  Flag to force debug mode. By default %b\n", forcedDebug)
		fmt.Println()
		fmt.Println("Being a FastCGI, don't forget to properly configure NGINX. For example, something similar to:")
		fmt.Println()
		fmt.Println(" location /testingEnd  { ")
		fmt.Println("    fastcgi_pass   0.0.0.0:9797; ")
		fmt.Println("    fastcgi_connect_timeout 5h; ")
		fmt.Println("    fastcgi_read_timeout 5h; ")
		fmt.Println("")
		fmt.Println("    fastcgi_param  QUERY_STRING       $query_string; ")
		fmt.Println("    fastcgi_param  REQUEST_METHOD     $request_method; ")
		fmt.Println("    fastcgi_param  CONTENT_TYPE       $content_type; ")
		fmt.Println("    fastcgi_param  CONTENT_LENGTH     $content_length; ")
		fmt.Println("    fastcgi_param  REQUEST            $request; ")
		fmt.Println("    fastcgi_param  REQUEST_BODY       $request_body; ")
		fmt.Println("    fastcgi_param  REQUEST_URI        $request_uri; ")
		fmt.Println("    fastcgi_param  DOCUMENT_URI       $document_uri; ")
		fmt.Println("    fastcgi_param  DOCUMENT_ROOT      $document_root; ")
		fmt.Println("    fastcgi_param  SERVER_PROTOCOL    $server_protocol; ")
		fmt.Println("    fastcgi_param  REMOTE_ADDR        $remote_addr; ")
		fmt.Println("    fastcgi_param  REMOTE_PORT        $remote_port; ")
		fmt.Println("    fastcgi_param  SERVER_ADDR        $server_addr; ")
		fmt.Println("    fastcgi_param  SERVER_PORT        $server_port; ")
		fmt.Println("    fastcgi_param HTTP_REFERER        $http_referer; ")
		fmt.Println("    fastcgi_param SCHEME              $scheme; ")
		fmt.Println(" } ")
		fmt.Println("")

		os.Exit(0)
	}

	flag.StringVar(&hostArg, "host", hostArg, "Host name for this FastCGI process.")
	flag.StringVar(&portArg, "port", portArg, "Port name for this FastCGI process.")
	flag.StringVar(&mockRequestResponseFile, "map", mockRequestResponseFile, "Fake mapped request/response file.")
	flag.StringVar(&requestJsonSchemaFile, "req", requestJsonSchemaFile, "Json Schema to validate requests.")
	flag.StringVar(&responseJsonSchemaFile, "res", responseJsonSchemaFile, "Json Schema to validate responses.")
	flag.BoolVar(&forcedDebug, "debug", forcedDebug, "Flag to force debug mode.")
	flag.Parse()

	return hostArg, portArg, mockRequestResponseFile, requestJsonSchemaFile, responseJsonSchemaFile, forcedDebug
}

// validate fake request response map against their json schemas
func validateMockRequestResponseFile(mockRequestResponseFile string, requestJsonSchemaFile string, responseJsonSchemaFile string, debug bool) (RequestResponseMap, gojsonschema.JSONLoader, error) {

	// regexpr to detect 'debug' params
	var debugRegexp = regexp.MustCompile("^" + DebugParameter + "")
	var err error
	var reqresmap RequestResponseMap = make(map[string]QueryResponse)
	var reqJsonSchema gojsonschema.JSONLoader

	mock, err := validateMockInput(mockRequestResponseFile)
	if err != nil {
		return reqresmap, reqJsonSchema, err
	}

	req, err := ioutil.ReadFile(requestJsonSchemaFile)
	if err != nil {
		log.Fatal(err)
		return reqresmap, reqJsonSchema, errors.New("Unable to read Request Json Schema File.")
	}

	res, err := ioutil.ReadFile(responseJsonSchemaFile)
	if err != nil {
		log.Fatal(err)
		return reqresmap, reqJsonSchema, errors.New("Unable to read Response Json Schema File.")
	}

	reqJsonSchema = gojsonschema.NewStringLoader(string(req))
	resJsonSchema := gojsonschema.NewStringLoader(string(res))

	type ReqRes struct {
		Qry      string           `json:"query,omitempty"`
		Req      *json.RawMessage `json:"req,omitempty"`
		Res      *json.RawMessage `json:"res"`
		request  string
		response string
	}
	dec := json.NewDecoder(strings.NewReader(string(mock)))

	err = ignoreFirstBracket(dec)
	if err != nil {
		return reqresmap, reqJsonSchema, err
	}

	// read object {"req": string, "res": string}
	for dec.More() {
		var rr ReqRes
		err = dec.Decode(&rr)
		if err != nil {
			log.Fatal(err)
			return reqresmap, reqJsonSchema, errors.New("Unable to process object at Mock Request Response File")
		}

		rr.request, err = toString(rr.Req)
		if err != nil {
			log.Println("Unable to process request object at Mock Request Response File")
			continue
		}

		rr.response, err = toString(rr.Res)
		if err != nil {
			log.Println("Unable to process response object at Mock Request Response File")
			continue
		}

		rr.Qry = orderQueryByParams(rr.Qry, debugRegexp)
		if debug {
			if len(rr.Qry) > 0 {
				log.Printf("%v %v -> %v\n", rr.Qry, rr.request, rr.response)
			} else {
				log.Printf(" %v -> %v\n", rr.request, rr.response)
			}
		}

		// request could be empty because it's an optative field
		if len(rr.request) > 0 {
			if !validateRequest(reqJsonSchema, rr.request) {
				continue
			}
		}

		if !validateResponse(resJsonSchema, rr.response) {
			continue
		}

		var key string

		// request could be empty because it's an optative field
		if len(rr.request) > 0 {

			// add pair to the map but after compacting those json
			key, err := compactJson([]byte(rr.request))
			if err != nil {
				log.Println("This request will be ignored")
				continue
			}
			if len(rr.Qry) > 0 {
				// key must take into account as well the provided query
				key = "[" + rr.Qry + "]" + key
			}

		} else { // request empty

			if len(rr.Qry) > 0 {
				// key must take only the provided query
				key = "[" + rr.Qry + "]"
			}

		}

		response, err := compactJson([]byte(rr.response))
		if err != nil {
			log.Println("That response will be ignored")
			continue
		}
		var value QueryResponse
		value.response = response
		reqresmap[key] = value
	}

	err = ignoreLastBracket(dec)
	if err != nil {
		return reqresmap, reqJsonSchema, err
	}

	// return result
	if len(reqresmap) == 0 {
		err = errors.New("Unable to validate any entry at Mock Request Response File")
	}
	return reqresmap, reqJsonSchema, err
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

// order query string by params in order to match ordered generated r.URL.Query() values later on
func orderQueryByParams(query string, debugRegexp *regexp.Regexp) string {
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

// validation request
func validateRequest(reqJsonSchema gojsonschema.JSONLoader, rrReq string) bool {

	result, err := gojsonschema.Validate(reqJsonSchema, gojsonschema.NewStringLoader(rrReq))
	if err != nil {
		log.Fatal(err)
		log.Println("This request will be ignored")
		return false
	}
	if !result.Valid() {
		log.Println("Request is not valid. See errors: ")
		for _, desc := range result.Errors() {
			log.Printf("- %s\n", desc)
		}
		log.Println("That request will be ignored")
		return false
	}
	return true
}

// validation response
func validateResponse(resJsonSchema gojsonschema.JSONLoader, rrRes string) bool {

	result, err := gojsonschema.Validate(resJsonSchema, gojsonschema.NewStringLoader(rrRes))
	if err != nil {
		log.Fatal(err)
		log.Println("This response will be ignored")
		return false
	}
	if !result.Valid() {
		log.Println("Response is not valid. See errors: ")
		for _, desc := range result.Errors() {
			log.Printf("- %s\n", desc)
		}
		log.Println("That response will be ignored")
		return false
	}

	return true
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

// validate just mock input
func validateMockInput(mockRequestResponseFile string) ([]byte, error) {

	mock, err := ioutil.ReadFile(mockRequestResponseFile)
	if err != nil {
		log.Fatal(err)
		return mock, errors.New("Unable to read Mock Request Response File.")
	}

	// validate the own mock input
	mockJsonSchema := gojsonschema.NewStringLoader(`{ 
		"$schema": "http://json-schema.org/draft-04/schema#",
  		"title": "Mock Request Response Json Schema",
  		"description": "version 0.0.1",
    	"type": "array",
    	"items": {
    		"type": "object",
    		"properties": {
      			"req": {
        			"type": "object"
      			},
      			"res": {
        			"type": "object"
      		   },
               "query": {
                    "type": "string"
               }
             },
    		"required": [
      			"res"
    		]
  		}
	}`)

	result, err := gojsonschema.Validate(mockJsonSchema, gojsonschema.NewStringLoader(string(mock)))
	if err != nil {
		log.Fatal(err)
		return mock, errors.New("Unable to process mock Json Schema")
	}

	if !result.Valid() {
		log.Println("Mock Request Response File is not valid. See errors: ")
		for _, desc := range result.Errors() {
			log.Printf("- %s\n", desc)
		}
		return mock, errors.New("Invalid Mock Request Response File")
	}

	// success
	return mock, nil
}

// must have at least ServeHTTP(), otherwise you will get this error
// *customHandler does not implement http.Handler (missing ServeHTTP method)
func (c *customHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	debug := (r.URL.Query()[DebugParameter] != nil) || c.forcedDebug
	var debugRegexp = regexp.MustCompile("^" + DebugParameter + "")

	if r.Method == http.MethodHead {
		if debug {
			log.Println("Requested Method HEAD. Probably a kind of ping")
		}
		http.NoBody.WriteTo(w)
		r.Body.Close()
		return
	}

	// GET params as a string
	query := QueryAsString(r)
	if debug {
		log.Println(query)
	}

	if r.ContentLength > 0 {

		// get body request to process
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			if debug {
				log.Println(err)
			}
		}

		if debug {
			log.Println("Body received: " + string(body))
		}

		// avoid processing before having booted up completely
		if c.rrmap != nil && len(*c.rrmap) > 0 {

			// really not needed, no invalid request in our map, but it's good to provide some feedback to our logs
			if validateRequest(*c.reqJS, string(body)) {

				key, err := compactJson(body)
				if err != nil {
					if debug {
						log.Print(err)
					}
				}
				if len(query) > 0 {
					key = "[" + orderQueryByParams(query, debugRegexp) + "]" + key
				}
				value := (*c.rrmap)[key]
				if len(value.response) > 0 {
					w.Header().Set("Content-Lenghth", strconv.Itoa(len(value.response)))
					w.Header().Set("Content-Type", "application/json")
					if _, err := w.Write([]byte(value.response)); err != nil {
						http.Error(w, err.Error(), http.StatusUnprocessableEntity)
						if debug {
							log.Println(err)
						}
					}
					if debug {
						log.Println("Sent back: " + value.response)
					}
				} else {
					http.Error(w, "key not found at internal cache", http.StatusNoContent)
					if debug {
						log.Println("key not found at internal cache")
					}
				}

			} else {
				http.Error(w, "Body Json Request doesn't comply with its expected Json Schema", http.StatusUnprocessableEntity)
			}
		}

	} else {
		if debug {
			log.Println("empty request body received")
		}

		if len(query) > 0 {
			key := "[" + orderQueryByParams(query, debugRegexp) + "]"
			value := (*c.rrmap)[key]
			if len(value.response) > 0 {
				w.Header().Set("Content-Lenghth", strconv.Itoa(len(value.response)))
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(value.response)); err != nil {
					http.Error(w, err.Error(), http.StatusUnprocessableEntity)
					if debug {
						log.Println(err)
					}
				}
				if debug {
					log.Println("Sent back: " + value.response)
				}
			} else {
				http.Error(w, "key not found at internal cache", http.StatusNoContent)
				if debug {
					log.Println("key not found at internal cache")
				}
			}
		} else {
			http.Error(w, "empty query with empty request body", http.StatusNoContent)
			if debug {
				log.Println("empty query with empty request body")
			}
		}

	}

	if debug {
		log.Printf("Processed request of %d bytes", r.ContentLength)
	}
}

// convert query parameter into a string to be used as index in the map
func QueryAsString(r *http.Request) string {

	// try to get IN ORDER all the parameters
	keys := []string{}
	for k, _ := range r.URL.Query() {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	query := ""
	for _, k := range keys {
		if k == DebugParameter {
			continue
		}
		if len(query) > 0 {
			query += "&"
		}
		v := r.URL.Query()[k]
		if len(v) > 0 { // there might be repeated params
			query += k + "="
			for i, w := range v {
				if i > 0 {
					query += ","
				}
				query += w
			}
		} else {
			// is a Flag
			query += k
		}
	}
	return query
}
