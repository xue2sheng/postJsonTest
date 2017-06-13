# Json Mock

Simple FastCGI golang implemented Json Mock in order to 'fake' validated json requests/responses.

![Mock Server](/images/mockServer.jpeg)

Although the main development environment was **Linux** (debian/opensuse), some tips will be provided for your **Windows** and **Apple** boxes. Being a testing tool, we'd better try to be able to work on different operating systems.

## Target

This simple **Json Mock** aims at **validating** not only the *real* server against *fake* or *real* clients but also let us change our **code under testing** without modifing already validated **data** or **testing structure**:

![Component Diagram](/images/component_diagram.png)

Pay special attention to the fact that **Fake Queries** and all **Json Schemas** are just **JSON** files that can be externally validated with your favourite tools and be reused in your REAL code or in CLIENT test cases.

## Example

At [data](/data/) folder, a very simple example is included with its [Request](/data/requestJsonSchema.json) and [Response](/data/responseJsonSchema.json) *Json Schemas*. That way **fake data** at [Request/Response Map](/data/requestResponseMap.json) is **validated** even before launching the mock server; invalid entries will be ignored.

Once the mock server got its **MAP** between validated requests and responses, requests works as **keys** and responses as **values** to be matched and answered back. Besides, those *entries* can include **query** information on GET query parameters that will take part into those **keys** after being ordered and filtered out. For example, **debug** parameters are filtered out:

    { "query": "ip=10.0.0.5&country=us", "req": { "test": 1, "id": "5" }, "res": { "id": "5" } }

As you can see **OPTIONAL** *"query"* elements are just bare strings so try to avoid superfluous blanks or exotic characters because there isn't too much validation on them. Regarding to *"req"* and *"res"* elementes, they must be json objects on their own and comply with their **Json Schemas**:

    ./JsonMock -debug=true

Launched in *debug* mode or including *debug* flag in *query* elements, it's possible to keep an eye on possible Json Schema validation issues.

### Automatic Multithreaded check of all request/response pairs

Don't hesitate to check them out with the command:

    ./JsonMock.test

See *JsonMock.test -help* for further details, paying special attention to its "-gzipOn" and "-queryStr" arguments in order to **test your REAL server when you consider proper** or just avoid typical *macOS* or *Windows* issues with **NGINX** running at port *80*:

    ./JsonMock.test -queryStr="http://0.0.0.0:8080/testingEnd?" -test.v

That test will try to use all your *CPU's* and to take advantage from *atomic* instructions so don't run it on a critical system where other processes shouldn't be impacted on their performance. 

Usually tuning its "-goroutinesMax" argument lets obtain better results with large resquest/response maps: no more goroutines running means necessarily a boost in performace and you might hoard too much resources, as file descriptors, and make your requests fail.

As well, if the **production server** doesn't implement **HEAD** queries, it's possible to disable those kind of checks with "-checkUp=false":

   ./JsonMock.test -queryStr="http://gsn.XXXX?" -checkUp=false

That way **the very same tester for the mock can be used on the production server** without having to change too much you're testing environment.

## Dependencies

Some *golang 3rd party libraries* have been used:

    go get github.com/gorilla/mux
    go get github.com/xeipuuv/gojsonschema
    
[gorilla/mux](http://www.gorillatoolkit.org/pkg/mux) by [Diego Siqueira](https://github.com/DiSiqueira) makes it easier to serve *FastCGI* requests and [xeipuuv/gojsonschema](https://github.com/xeipuuv/gojsonschema) by [xeipuuv](https://github.com/xeipuuv/gojsonschema) simpilfies *json schema* validations.

## CMake-based build

Although usual **golang** commands like *go build* or *go test* can be directly used, a **CMake** project is provided in order to avoid typical differencies among **Linux**, **macOS** and **Windows**.

For example, to build & install everything, server and its tests, take advantage that all the different *make targets* are related. So if you ask for installing its tests, all the binaries will be generated:

    mkdir build && cd build && cmake .. -DJsonMock_TEST=1 && make installJsonMock.test 

But if you're only interested in the bare server in order to deploy it, then just execute the usual command:

    mkdir build && cd build && cmake .. && make

See **make help** at that *build* folder to get all the possibilities (all, JsonMock, JsonMock.test, installJsonMock, ...).

## Getting a simple mock server to simulate client's behaviour

### Curl queries

The query can be simulated using **curl**. For example, a typical call might be:

    

### NGINX configuration

Being a FastCGI that processes request body and probably responses with a **gzipped** json, don't forget:

#### GZIP

Nginx configuration for GZIP WITH THE CORRECT ERROR CODE (200) in the response:

     gzip on;
     gzip_vary on;
     gzip_proxied any;
     gzip_comp_level 6;
     gzip_buffers 16 8k;
     gzip_http_version 1.1;

     gzip_types text/plain text/css application/json application/javascript application/x-javascript text/javascript text/xml application/xml application/rss+xml application/atom+xml application/rdf+xml;

 
#### POST request body

Nginx configuration for passing the POST body:

     fastcgi_param  REQUEST_BODY       $request_body;

#### Using different Linux distros

If you happen to use **Debian**, its default *Nginx* configuration for *location* should be defined at:

     /etc/nginx/sites-available/default

But the usual *gzip* configuration should be defined at:

     /etc/nginx/nginx.conf

In case of **openSUSE**, only the previous configuration file should be updated with both *gzip* and *location* info.

#### Using different Operating Systems 

On **macOS** systems, it's highly likely that your *nginx* runs on a different port due to security reasons. For example, you might use **8080** port instead. On that case, you could redirect that port to the usual **80** in oder to avoid having to invoke your test cases with *:8080/testingEnd*.

On **Windows 10** systems, the situation is similar because there might be a system process using that **80** port already (check it out with *netstat* and *taskmanager*). So your *NGINX* configuration can look like:

     server {
        listen       8080;
        server_name  localhost;

        #charset koi8-r;

        #access_log  logs/host.access.log  main;

        location / {
            root   html;
            index  index.html index.htm;
        }

	location /testingEnd {
		fastcgi_pass   127.0.0.1:9797;
		fastcgi_param  GATEWAY_INTERFACE  CGI/1.1;
		fastcgi_param  SERVER_SOFTWARE    nginx;
		fastcgi_param  QUERY_STRING       $query_string;
		fastcgi_param  REQUEST_METHOD     $request_method;
		fastcgi_param  REQUEST_BODY       $request_body;
		fastcgi_param  CONTENT_TYPE       $content_type;
		fastcgi_param  CONTENT_LENGTH     $content_length;
		fastcgi_param  SCRIPT_FILENAME    $document_root$fastcgi_script_name;
		fastcgi_param  SCRIPT_NAME        $fastcgi_script_name;
		fastcgi_param  REQUEST_URI        $request_uri;
		fastcgi_param  DOCUMENT_URI       $document_uri;
		fastcgi_param  DOCUMENT_ROOT      $document_root;
		fastcgi_param  SERVER_PROTOCOL    $server_protocol;
		fastcgi_param  REMOTE_ADDR        $remote_addr;
		fastcgi_param  REMOTE_PORT        $remote_port;
		fastcgi_param  SERVER_ADDR        $server_addr;
		fastcgi_param  SERVER_PORT        $server_port;
		fastcgi_param  SERVER_NAME        $server_name;
	}

## More Windows 10 specific tricks

Typical **make** command on Windows could get confused (there can be serveral versions) and crash with the following error message:

     make: *** No targets specified and no makefile found.  Stop.

By installing **ninja build** command and add it to the *path environment varible* at your *Powershell console*, you might spare yourself all that pain in the neck, provided you got already installed *go 1.8*:

     mkdir build
     cd build
     cmake .. -G "Ninja" -DJsonMock_TEST=1
     ninja
     ninja JsonMock.test

If you *NGINX* configuration expects to get the JsonMock server running at **127.0.0.1:9797**, don't forget to launch it that way:

     .\JsonMock.exe 127.0.0.1 9797 C:\Users\user\Documents\Code\jsonMock\build\data\requestResponseMap.json c:\Users\user\Documents\Code\jsonMock\build\data\requestJsonSchema.json C:\Users\user\Documents\Code\jsonMock\build\data\responseJsonSchema.json true

Regarding to commandline **curl.exe** invocation and avoiding Powershell *curl* alias, take into account to escape properly all *quotation* marks in the body message at your **Powershell** console:

     curl.exe -vvv -H 'Content-Type: application/json' -H 'Accept-Encoding: gzip' "http://localhost:8080/testingEnd" -d '{\"test\": 1, \"id\": \"1\"}'     

